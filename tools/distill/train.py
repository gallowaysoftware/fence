"""LoRA / QLoRA fine-tune of a base model on var/distill/{train,val}.jsonl.

Loss is on the answer only (WHY line + directives JSON), weighted per row by the builder's
outcome weight. Batch size is one sequence (prompts are ~6-9k tokens); --accum sets the
effective batch. Only the answer's logits are materialised (logits_to_keep), which keeps a
150k-vocab head from costing gigabytes per step.

  uv run train.py --base ~/models/hf/Qwen3-8B --out ~/models/distill/qwen3-8b-r1 --qlora
"""

import argparse
import math
import random
import time
from pathlib import Path

import torch
import torch.nn.functional as F
from peft import LoraConfig, get_peft_model, prepare_model_for_kbit_training
from transformers import AutoModelForCausalLM, AutoTokenizer, BitsAndBytesConfig

from common import encode, read_rows, write_json


def load_model(base, qlora):
    kw = dict(dtype=torch.bfloat16, attn_implementation="sdpa")
    if qlora:
        kw["quantization_config"] = BitsAndBytesConfig(
            load_in_4bit=True,
            bnb_4bit_quant_type="nf4",
            bnb_4bit_use_double_quant=True,
            bnb_4bit_compute_dtype=torch.bfloat16,
        )
        kw["device_map"] = {"": 0}
    else:
        kw["device_map"] = "auto"
    try:
        model = AutoModelForCausalLM.from_pretrained(base, **kw)
    except ValueError:
        # Multimodal checkpoints (Qwen3.8's qwen3_5) may only map to the image-text class.
        from transformers import AutoModelForImageTextToText

        model = AutoModelForImageTextToText.from_pretrained(base, **kw)
    if qlora:
        model = prepare_model_for_kbit_training(model, use_gradient_checkpointing=True)
    else:
        model.gradient_checkpointing_enable(gradient_checkpointing_kwargs={"use_reentrant": False})
        model.enable_input_require_grads()
    model.config.use_cache = False
    return model


def encode_all(tok, rows, max_len):
    out, dropped = [], 0
    for r in rows:
        ids, n = encode(tok, r)
        if len(ids) > max_len:
            dropped += 1
            continue
        out.append((ids, n, float(r.get("weight", 1.0)), r["meta"]))
    return out, dropped


def answer_loss(model, ids, n, device):
    x = torch.tensor([ids], device=device)
    logits = model(input_ids=x, logits_to_keep=n + 1).logits[0, :n].float()
    return F.cross_entropy(logits, x[0, -n:])


@torch.no_grad()
def val_loss(model, data, device):
    model.eval()
    tot = 0.0
    for ids, n, _, _ in data:
        tot += answer_loss(model, ids, n, device).item()
    model.train()
    return tot / max(len(data), 1)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--data", default="../../var/distill")
    ap.add_argument("--base", required=True, help="HF model id or local HF-format checkpoint")
    ap.add_argument("--out", required=True, help="adapter output dir (outside the repo)")
    ap.add_argument("--qlora", action="store_true", help="4-bit base (5090); default bf16 LoRA (Spark)")
    ap.add_argument("--rank", type=int, default=32)
    ap.add_argument("--alpha", type=int, default=64)
    ap.add_argument("--dropout", type=float, default=0.05)
    ap.add_argument("--lr", type=float, default=1e-4)
    ap.add_argument("--epochs", type=float, default=2)
    ap.add_argument("--accum", type=int, default=16)
    ap.add_argument("--warmup", type=float, default=0.05)
    ap.add_argument("--max-len", type=int, default=12288)
    ap.add_argument("--seed", type=int, default=1)
    ap.add_argument("--limit", type=int, default=0, help="train rows to use (smoke test)")
    ap.add_argument("--epoch-rows", type=int, default=0, help="expected rows per epoch (0: all rows' worth of weight)")
    ap.add_argument("--val-rows", type=int, default=96, help="val loss on the decision-like rows (at most 2/3) plus a seeded routine sample, this many in all (0: all)")
    a = ap.parse_args()

    random.seed(a.seed)
    torch.manual_seed(a.seed)
    out = Path(a.out)
    out.mkdir(parents=True, exist_ok=True)

    tok = AutoTokenizer.from_pretrained(a.base)
    train, dropped_t = encode_all(tok, read_rows(Path(a.data) / "train.jsonl"), a.max_len)
    val, dropped_v = encode_all(tok, read_rows(Path(a.data) / "val.jsonl"), a.max_len)
    if a.limit:
        train = train[: a.limit]
        val = val[: max(a.limit // 8, 1)]
    if a.val_rows and len(val) > a.val_rows:
        # Val loss runs 7 times; on all ~1,100 rows that is over 2 h of forward passes.
        routine = {"routine", "empty", "keep", "repeat"}
        dec = [v for v in val if v[3]["kind"] not in routine]
        rest = [v for v in val if v[3]["kind"] in routine]
        rng = random.Random(a.seed)
        if len(dec) > a.val_rows * 2 // 3:
            dec = rng.sample(dec, a.val_rows * 2 // 3)
        val = dec + rng.sample(rest, min(len(rest), a.val_rows - len(dec)))
    mean_w = sum(w for _, _, w, _ in train) / len(train)
    lens = sorted(len(t[0]) for t in train)
    print(f"train {len(train)} (dropped {dropped_t} over {a.max_len} tokens), val {len(val)} (dropped {dropped_v}); "
          f"tokens p50 {lens[len(lens) // 2]} max {lens[-1]}; mean weight {mean_w:.3f}", flush=True)

    model = load_model(a.base, a.qlora)
    model = get_peft_model(model, LoraConfig(
        r=a.rank, lora_alpha=a.alpha, lora_dropout=a.dropout, target_modules="all-linear",
        exclude_modules=r".*(visual|vision|mtp).*", task_type="CAUSAL_LM",
    ))
    model.print_trainable_parameters()
    device = next(model.parameters()).device
    params = [p for p in model.parameters() if p.requires_grad]
    opt = torch.optim.AdamW(params, lr=a.lr, weight_decay=0.0, betas=(0.9, 0.99))
    # Each epoch keeps row i with probability p_i = min(1, R * w_i / sum(w)) and scales its
    # loss by (w_i / p_i) * (sum(p) / sum(w)): the expected gradient is the weighted one,
    # and R (--epoch-rows, default all rows) bounds the compute an epoch costs.
    ws = [w for _, _, w, _ in train]
    rows = a.epoch_rows or len(train)
    keep = [min(1.0, rows * w / sum(ws)) for w in ws]
    norm = sum(keep) / sum(ws)
    scale = [w / p * norm for w, p in zip(ws, keep)]
    per_epoch = sum(keep)
    steps = max(1, math.ceil(per_epoch * a.epochs / a.accum))
    warm = max(1, int(steps * a.warmup))
    sched = torch.optim.lr_scheduler.LambdaLR(opt, lambda s: min(1.0, (s + 1) / warm) * 0.5 * (1 + math.cos(math.pi * min(s, steps) / steps)))
    print(f"~{per_epoch:.0f} rows per epoch, {steps} optimizer steps", flush=True)

    log = {"args": vars(a), "train_rows": len(train), "val_rows": len(val), "rows_per_epoch": per_epoch, "steps": steps, "history": []}
    log["history"].append({"step": 0, "val_loss": val_loss(model, val, device)})
    print(f"step 0 val_loss {log['history'][-1]['val_loss']:.4f}", flush=True)
    order, step, micro, run_loss, t0 = [], 0, 0, 0.0, time.time()
    model.train()
    while step < steps:
        if not order:
            order = [i for i in range(len(train)) if random.random() < keep[i]]
            random.shuffle(order)
            continue
        i = order.pop()
        ids, n, _, _ = train[i]
        loss = answer_loss(model, ids, n, device) * scale[i]
        (loss / a.accum).backward()
        run_loss += loss.item()
        micro += 1
        if micro % a.accum == 0:
            torch.nn.utils.clip_grad_norm_(params, 1.0)
            opt.step()
            sched.step()
            opt.zero_grad(set_to_none=True)
            step += 1
            rec = {"step": step, "train_loss": run_loss / a.accum, "lr": sched.get_last_lr()[0],
                   "sec": round(time.time() - t0), "max_mem_gb": round(torch.cuda.max_memory_allocated() / 2**30, 1) if torch.cuda.is_available() else None}
            run_loss = 0.0
            if step % max(1, steps // 6) == 0 or step == steps:
                rec["val_loss"] = val_loss(model, val, device)
            log["history"].append(rec)
            print(rec, flush=True)
    model.save_pretrained(out / "adapter")
    tok.save_pretrained(out / "adapter")
    log["seconds"] = round(time.time() - t0)
    write_json(out / "train_log.json", log)
    print(f"adapter -> {out / 'adapter'}", flush=True)


if __name__ == "__main__":
    main()
