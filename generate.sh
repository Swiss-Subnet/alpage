#!/usr/bin/env bash
# Regenerate Go canister bindings from the pinned IC release's candid files.
#
# did files are NOT committed: they are a byproduct of the same pinned
# canisters.tar (IC_CANISTERS_DIR) that the flake fetches for the WASMs, so
# they always match the code we install. Bump the release in flake.nix, run
# this, and bindings move in lockstep.
#
# Run inside `nix develop` (needs IC_CANISTERS_DIR) with goic on PATH:
#   go install github.com/aviate-labs/agent-go/cmd/goic@v0.9.2
set -euo pipefail

: "${IC_CANISTERS_DIR:?run inside 'nix develop' so IC_CANISTERS_DIR is set}"
command -v goic >/dev/null || { echo "goic not found; go install github.com/aviate-labs/agent-go/cmd/goic@v0.9.2"; exit 1; }

root="$(cd "$(dirname "$0")" && pwd)"
outroot="$root/gen"
rm -rf "$outroot"

# Each canister gets its own package: governance/registry/root define
# overlapping type names (CanisterSettings, GuestLaunchMeasurements, ...) that
# collide in a single package.
#
# name  did-basename  canister-id
gen() {
  local name="$1" did="$2" id="$3"
  local src="$IC_CANISTERS_DIR/$did"
  [ -f "$src" ] || { echo "missing did: $src"; exit 1; }
  mkdir -p "$outroot/$name"
  # goic's candid parser chokes on // line comments; strip them first.
  local tmp; tmp="$(mktemp)"
  sed -E 's://.*$::' "$src" > "$tmp"
  goic generate did "$tmp" "$name" \
    --packageName="$name" \
    --agentName="${name^}" \
    --canisterID="$id" \
    --output="$outroot/$name/$name.go"
  rm -f "$tmp"
  echo "  gen/$name/$name.go  <- $did"
}

echo "generating bindings from $IC_CANISTERS_DIR"
gen governance governance-canister_test.wasm.gz.did rrkah-fqaaa-aaaaa-aaaaq-cai
gen registry   registry-canister.wasm.gz.did        rwlgt-iiaaa-aaaaa-aaaaa-cai
gen root       root-canister.wasm.gz.did             r7inp-6aaaa-aaaaa-aaabq-cai
# Engine controller: NNS subnet index 18 (ENGINE_CONTROLLER_CANISTER_INDEX_IN_NNS_SUBNET).
# Its did comes from the CDN, not canisters.tar (see cdnCanisters in flake.nix).
gen engine     engine-controller-canister.wasm.gz.did si2b5-pyaaa-aaaaa-aaaja-cai

gofmt -w "$outroot"
echo "done"
