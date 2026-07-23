{
  description = "alpage: declarative node-fleet management for the Internet Computer, applied via NNS proposals";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs { inherit system; };

        icReleaseTag = "release-2026-07-09_04-35-base";
        icReleaseCommit = "992628c4f26cc7320396b6b8be37133a666a4386";

        icCanistersTarHash = "sha256-MrUXCUV08zIf/mHkA84fizuCp0BlU81AogEAxdFuapM=";

        # alp version: a git tag when building a tagged rev, else the short rev,
        # else "dev" for a dirty tree. Injected into main.version by the alp
        # package's ldflags.
        alpVersion =
          self.ref or (if self ? shortRev then "g${self.shortRev}" else "dev");

        # Fixed-output hashes for the codegen derivations and the Go vendor tree.
        # Set to fakeHash and run `nix build` to learn the real values, then
        # paste them back. Rehash when the IC release (flake pins) or deps move.
        genBindingsHash = "sha256-axdmDqj9SYBeSXoGPKJL5p76gWEAEBHjjOGmuQvF88g=";
        genProtoHash = "sha256-u0RMTWaPFBQd/a7D/pGS8Rxr3wjDfO9wELagbZp8QDM=";
        alpVendorHash = "sha256-/u2ndj9qCkzR1RYOWJWhxOCmRR64VsPVUl/a3OPzP7U=";

        pocketIcPlatform =
          {
            "x86_64-linux" = "x86_64-linux";
            "aarch64-linux" = "arm64-linux";
            "x86_64-darwin" = "x86_64-darwin";
            "aarch64-darwin" = "arm64-darwin";
          }
          .${system};

        pocketIcHashes = {
          "x86_64-linux" = "sha256-0bY9iGMoHzBR2wT7LaMYXA1qvAvHTWgQQLijTFv34z8=";
          "aarch64-linux" = "sha256-46ro8duQHXxjvUbHtbusIRr8ZJUicCS14Zim9vwHTAM=";
          "x86_64-darwin" = "sha256-onS7d1aOTcbfD34HPmYqjhUf0wEOQfqF8eL5l9TjSmw=";
          "aarch64-darwin" = "sha256-qRWTYxGxMRxAi2nsMBJVmQk62WTSbJhnUn5hz7NPmh8=";
        };

        icCanisters = pkgs.stdenv.mkDerivation {
          pname = "ic-canisters";
          version = icReleaseTag;
          src = pkgs.fetchurl {
            url = "https://github.com/dfinity/ic/releases/download/${icReleaseTag}/canisters.tar";
            hash = icCanistersTarHash;
          };
          sourceRoot = ".";
          dontBuild = true;
          installPhase = ''
            mkdir -p $out
            cp *.wasm.gz *.did $out/
          '';
        };

        pocketIc = pkgs.stdenv.mkDerivation {
          pname = "pocket-ic";
          version = icReleaseTag;
          src = pkgs.fetchurl {
            url = "https://github.com/dfinity/ic/releases/download/${icReleaseTag}/pocket-ic-${pocketIcPlatform}.gz";
            hash = pocketIcHashes.${system};
          };
          dontUnpack = true;
          nativeBuildInputs = [
            pkgs.gzip
          ]
          ++ pkgs.lib.optionals pkgs.stdenv.isLinux [ pkgs.autoPatchelfHook ];
          buildInputs = pkgs.lib.optionals pkgs.stdenv.isLinux [
            pkgs.stdenv.cc.cc.lib
          ];
          installPhase = ''
            mkdir -p $out/bin
            gunzip -c $src > $out/bin/pocket-ic
            chmod +x $out/bin/pocket-ic
          '';
        };
        # Candid bindings (gen/) are generated, not committed. goic is fetched
        # via `go install`, so this needs network: it is a fixed-output
        # derivation pinned by genBindingsHash. The .did inputs come from the
        # already-pinned icCanisters, so output changes only when goic or the IC
        # release move; rehash then (nix build prints the expected hash).
        genBindings = pkgs.stdenv.mkDerivation {
          pname = "alpage-gen-bindings";
          version = icReleaseTag;
          src = ./.;
          nativeBuildInputs = [
            pkgs.go
            pkgs.cacert
          ];
          IC_CANISTERS_DIR = "${icCanisters}";
          GOFLAGS = "-mod=mod";
          buildPhase = ''
            export HOME="$TMPDIR"
            export GOPATH="$TMPDIR/go"
            export GOBIN="$TMPDIR/gobin"
            export PATH="$GOBIN:$PATH"
            go install github.com/aviate-labs/agent-go/cmd/goic@v0.9.2
            ./generate.sh
          '';
          installPhase = "cp -r gen $out";
          outputHashMode = "recursive";
          outputHashAlgo = "sha256";
          outputHash = genBindingsHash;
        };

        # Registry protobuf types (nns/registrypb/) are generated from .proto
        # files fetched from the pinned IC commit over the network, so this is
        # also a fixed-output derivation. protoc/protoc-gen-go come from nixpkgs.
        genProto = pkgs.stdenv.mkDerivation {
          pname = "alpage-gen-proto";
          version = icReleaseTag;
          src = ./.;
          nativeBuildInputs = [
            pkgs.protobuf
            pkgs.protoc-gen-go
            pkgs.curl
            pkgs.cacert
            pkgs.go
          ];
          IC_RELEASE_COMMIT = icReleaseCommit;
          buildPhase = ''
            export HOME="$TMPDIR"
            ./generate-proto.sh
          '';
          installPhase = "cp -r nns/registrypb $out";
          outputHashMode = "recursive";
          outputHashAlgo = "sha256";
          outputHash = genProtoHash;
        };

        alp = pkgs.buildGoModule {
          pname = "alp";
          version = alpVersion;
          src = ./.;
          vendorHash = alpVendorHash;
          # Drop in the generated code the offline build cannot produce itself.
          preBuild = ''
            cp -r ${genBindings} gen
            cp -r ${genProto} nns/registrypb
            chmod -R u+w gen nns/registrypb
          '';
          subPackages = [ "cmd/alp" ];
          # gnark-crypto (via agent-go) ships arm64/amd64 asm that #includes
          # generated files from its own GOROOT layout, which buildGoModule's
          # vendored tree does not reproduce; purego selects its pure-Go paths.
          tags = [ "purego" ];
          ldflags = [
            "-s"
            "-w"
            "-X main.version=${alpVersion}"
          ];
          doCheck = false;
        };
      in
      {
        packages = {
          inherit
            icCanisters
            pocketIc
            genBindings
            genProto
            ;
          alp = alp;
          default = alp;
        };

        formatter = pkgs.nixfmt;

        devShells.default = pkgs.mkShell {
          packages = [
            pkgs.go_1_25
            pkgs.gopls
            pkgs.nixfmt
            pkgs.protobuf
            pkgs.protoc-gen-go
            pocketIc
          ];

          POCKET_IC_BIN = "${pocketIc}/bin/pocket-ic";
          IC_CANISTERS_DIR = "${icCanisters}";
          IC_RELEASE_COMMIT = icReleaseCommit;
          GOVERNANCE_WASM = "${icCanisters}/governance-canister_test.wasm.gz";
          REGISTRY_WASM = "${icCanisters}/registry-canister.wasm.gz";
          ROOT_WASM = "${icCanisters}/root-canister.wasm.gz";

          shellHook = ''
            if [ -t 1 ]; then
              echo "alpage dev shell"
              echo "  go         $(go version | awk '{print $3}')"
              echo "  pocket-ic  ${icReleaseTag} (ic @ ${builtins.substring 0 12 icReleaseCommit})"
              echo "  canisters  ${icReleaseTag} (ic @ ${builtins.substring 0 12 icReleaseCommit})"
            fi
          '';
        };
      }
    );
}
