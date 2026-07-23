#!/usr/bin/env bash
# Regenerate the registry protobuf types used to seed a subnet into a local
# PocketIC registry for tests.
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
outdir="$root/nns/registrypb"
base="https://raw.githubusercontent.com/dfinity/ic/${IC_RELEASE_COMMIT}/rs/protobuf/def"

# subnet.proto (SubnetRecord, SubnetListRecord) and its transitive imports.
# Paths mirror the upstream proto_path layout so imports resolve unchanged.
protos=(
  "registry/subnet/v1/subnet.proto"
  "registry/crypto/v1/crypto.proto"
  "registry/replica_version/v1/replica_version.proto"
  "registry/node/v1/node.proto"
  "types/v1/types.proto"
)

src="$(mktemp -d)"
trap 'rm -rf "$src"' EXIT
for p in "${protos[@]}"; do
  mkdir -p "$src/$(dirname "$p")"
  curl -sfL -o "$src/$p" "$base/$p" || { echo "fetch failed: $p"; exit 1; }
done

rm -rf "$outdir"
mkdir -p "$outdir"

# All three .proto files land in one Go package (registrypb) via M-mappings;
# their distinct proto packages (registry.subnet.v1, ...) do not force separate
# Go packages once mapped to the same import path.
protoc \
  --proto_path="$src" \
  --go_out="$outdir" \
  --go_opt=paths=source_relative \
  --go_opt=Mregistry/subnet/v1/subnet.proto=./registrypb \
  --go_opt=Mregistry/crypto/v1/crypto.proto=./registrypb \
  --go_opt=Mregistry/replica_version/v1/replica_version.proto=./registrypb \
  --go_opt=Mregistry/node/v1/node.proto=./registrypb \
  --go_opt=Mtypes/v1/types.proto=./registrypb \
  "${protos[@]/#/$src/}"

# protoc lays files out under the source-relative tree; flatten to the package dir.
find "$outdir" -name '*.pb.go' -exec mv -f {} "$outdir/" \;
find "$outdir" -type d -empty -delete
gofmt -w "$outdir"
echo "generated nns/registrypb from ic @ ${IC_RELEASE_COMMIT:0:12}"
