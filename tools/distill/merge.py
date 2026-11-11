"""Merge a LoRA adapter into its bf16 base on the CPU, for GGUF conversion. The merge is
done against the unquantised base even when training was QLoRA."""

import argparse

import torch
from peft import PeftModel
from transformers import AutoModelForCausalLM, AutoTokenizer


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", required=True)
    ap.add_argument("--adapter", required=True)
    ap.add_argument("--out", required=True)
    a = ap.parse_args()
    model = AutoModelForCausalLM.from_pretrained(a.base, dtype=torch.bfloat16, device_map={"": "cpu"})
    model = PeftModel.from_pretrained(model, a.adapter).merge_and_unload()
    model.save_pretrained(a.out, safe_serialization=True)
    AutoTokenizer.from_pretrained(a.adapter).save_pretrained(a.out)


if __name__ == "__main__":
    main()
