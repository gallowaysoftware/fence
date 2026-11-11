#!/usr/bin/env bash
# Export a trained adapter to GGUF for llama-swap (docs/research/DISTILL.md, "Export").
#
#   export.sh merged BASE_HF ADAPTER OUT_DIR NAME [QUANT]   merge, convert, quantize (default Q8_0)
#   export.sh lora   BASE_HF ADAPTER OUT_DIR NAME           a GGUF LoRA for llama-server --lora
#                                                           on the base model's existing GGUF
set -euo pipefail

# llama.cpp at the release tag of the local llama-server (llama-server --version: build 10797).
LLAMA_CPP_COMMIT=${LLAMA_CPP_COMMIT:-b10797}
LLAMA_CPP_DIR=${LLAMA_CPP_DIR:-$HOME/models/distill/llama.cpp}
QUANTIZE=${QUANTIZE:-llama-quantize}

mode=$1 base=$2 adapter=$3 out=$4 name=$5 quant=${6:-Q8_0}
here=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$out"

if [[ ! -d $LLAMA_CPP_DIR/.git ]]; then
	git clone --quiet --filter=blob:none https://github.com/ggml-org/llama.cpp "$LLAMA_CPP_DIR"
fi
git -C "$LLAMA_CPP_DIR" fetch --quiet --tags origin
git -C "$LLAMA_CPP_DIR" checkout --quiet "$LLAMA_CPP_COMMIT"

case $mode in
merged)
	uv run --project "$here" python "$here/merge.py" --base "$base" --adapter "$adapter" --out "$out/merged"
	uv run --project "$here" python "$LLAMA_CPP_DIR/convert_hf_to_gguf.py" "$out/merged" \
		--outtype bf16 --outfile "$out/$name-bf16.gguf"
	"$QUANTIZE" "$out/$name-bf16.gguf" "$out/$name-$quant.gguf" "$quant"
	rm -rf "$out/merged"
	echo "$out/$name-$quant.gguf"
	;;
lora)
	uv run --project "$here" python "$LLAMA_CPP_DIR/convert_lora_to_gguf.py" "$adapter" \
		--base "$base" --outtype bf16 --outfile "$out/$name-lora.gguf"
	echo "$out/$name-lora.gguf"
	;;
*)
	echo "usage: $0 merged|lora BASE_HF ADAPTER OUT_DIR NAME [QUANT]" >&2
	exit 2
	;;
esac
