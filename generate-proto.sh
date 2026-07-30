#!/usr/bin/env bash
# Regenerate the IC protobuf types: the registry types used to seed a subnet
# into a local PocketIC registry for tests, and the governance NnsFunction enum
# the proposal actions dispatch on.
#
# The .proto sources are NOT committed: they are fetched from the same pinned IC
# release commit the flake uses for the WASMs (IC_RELEASE_COMMIT), so the
# generated types match the registry canister we install. Bump the release in
# flake.nix and rerun this; the types move in lockstep.
#
# Run inside `nix develop` (needs IC_RELEASE_COMMIT, protoc, protoc-gen-go):
#   ./generate-proto.sh
set -euo pipefail

: "${IC_RELEASE_COMMIT:?run inside 'nix develop' so IC_RELEASE_COMMIT is set}"
command -v protoc >/dev/null || { echo "protoc not found; run inside 'nix develop'"; exit 1; }
command -v protoc-gen-go >/dev/null || { echo "protoc-gen-go not found; run inside 'nix develop'"; exit 1; }

root="$(cd "$(dirname "$0")" && pwd)"
raw="https://raw.githubusercontent.com/dfinity/ic/${IC_RELEASE_COMMIT}"

# subnet.proto (SubnetRecord, SubnetListRecord) and governance.proto (the
# NnsFunction enum), plus their transitive imports. Each entry is
# "<repo proto root> <path under it>": the protos live under several crates'
# own proto/ dirs, and the path under each root is what imports resolve
# against, so it doubles as the proto_path-relative name.
protos=(
  "rs/protobuf/def registry/subnet/v1/subnet.proto"
  "rs/protobuf/def registry/crypto/v1/crypto.proto"
  "rs/protobuf/def registry/replica_version/v1/replica_version.proto"
  "rs/protobuf/def registry/node/v1/node.proto"
  "rs/protobuf/def registry/node_operator/v1/node_operator.proto"
  "rs/protobuf/def registry/dc/v1/dc.proto"
  "rs/protobuf/def types/v1/types.proto"
  "rs/nns/governance/proto ic_nns_governance/pb/v1/governance.proto"
  "rs/types/base_types/proto ic_base_types/pb/v1/types.proto"
  "rs/ledger_suite/icp/proto ic_ledger/pb/v1/types.proto"
  "rs/nervous_system/proto/proto ic_nervous_system/pb/v1/nervous_system.proto"
  "rs/nns/common/proto ic_nns_common/pb/v1/types.proto"
  "rs/sns/swap/proto ic_sns_swap/pb/v1/swap.proto"
)

src="$(mktemp -d)"
trap 'rm -rf "$src"' EXIT
names=()
for entry in "${protos[@]}"; do
  read -r prefix p <<<"$entry"
  names+=("$p")
  mkdir -p "$src/$(dirname "$p")"
  curl -sfL -o "$src/$p" "$raw/$prefix/$p" || { echo "fetch failed: $prefix/$p"; exit 1; }
done

# One Go package per proto package. The registry protos can share a package
# (no colliding type names), but governance's imports cannot join it or each
# other: several declare their own Tokens/Account/GovernanceError, and two are
# both named types.proto. protoc's module= mode writes each file to the
# directory its M-mapping names, so all packages come out of one invocation.
mod="github.com/swiss-subnet/alpage/nns/pb"
maps=(
  "--go_opt=Mregistry/subnet/v1/subnet.proto=$mod/registry"
  "--go_opt=Mregistry/crypto/v1/crypto.proto=$mod/registry"
  "--go_opt=Mregistry/replica_version/v1/replica_version.proto=$mod/registry"
  "--go_opt=Mregistry/node/v1/node.proto=$mod/registry"
  "--go_opt=Mregistry/node_operator/v1/node_operator.proto=$mod/registry"
  "--go_opt=Mregistry/dc/v1/dc.proto=$mod/registry"
  "--go_opt=Mtypes/v1/types.proto=$mod/registry"
  "--go_opt=Mic_nns_governance/pb/v1/governance.proto=$mod/governance"
  "--go_opt=Mic_base_types/pb/v1/types.proto=$mod/basetypes"
  "--go_opt=Mic_ledger/pb/v1/types.proto=$mod/ledger"
  "--go_opt=Mic_nervous_system/pb/v1/nervous_system.proto=$mod/nervoussystem"
  "--go_opt=Mic_nns_common/pb/v1/types.proto=$mod/nnscommon"
  "--go_opt=Mic_sns_swap/pb/v1/swap.proto=$mod/snsswap"
)

rm -rf "$root/nns/pb"
mkdir -p "$root/nns/pb"

protoc \
  --proto_path="$src" \
  --go_out="$root/nns/pb" \
  --go_opt=module="$mod" \
  "${maps[@]}" \
  "${names[@]/#/$src/}"

gofmt -w "$root/nns/pb"
echo "generated nns/pb from ic @ ${IC_RELEASE_COMMIT:0:12}"
