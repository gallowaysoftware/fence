"""Shared by train.py and evaluate.py: reading var/distill rows and rendering them with the
base model's chat template exactly as the bridge's inference request will be rendered."""

import json
from pathlib import Path


def read_rows(path):
    with open(path) as f:
        return [json.loads(line) for line in f if line.strip()]


def prompt_text(tok, messages):
    """The generation prompt for messages without the answer. Thinking is off: the bridge
    asks the strategist with thinking disabled, and the targets carry no reasoning trace."""
    return tok.apply_chat_template(
        messages, tokenize=False, add_generation_prompt=True, enable_thinking=False
    )


def encode(tok, row):
    """(input_ids, n_target): the prompt then the answer and its end-of-turn, with the
    answer's tokens last. The answer suffix is cut from the full rendered conversation when
    the template renders it as a continuation of the generation prompt (Qwen3 does);
    otherwise it falls back to answer + eos."""
    msgs = row["messages"]
    prompt = prompt_text(tok, msgs[:-1])
    full = tok.apply_chat_template(msgs, tokenize=False, enable_thinking=False)
    if full.startswith(prompt):
        answer = full[len(prompt):]
    else:
        answer = msgs[-1]["content"] + (tok.eos_token or "")
    p = tok(prompt, add_special_tokens=False)["input_ids"]
    a = tok(answer, add_special_tokens=False)["input_ids"]
    return p + a, len(a)


def write_json(path, obj):
    Path(path).parent.mkdir(parents=True, exist_ok=True)
    with open(path, "w") as f:
        json.dump(obj, f, indent=2)
        f.write("\n")
