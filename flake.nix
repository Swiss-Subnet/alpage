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
      in
      {
        packages = {
          inherit icCanisters pocketIc;
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
