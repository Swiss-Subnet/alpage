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
    let
      # The IC release the dry-run runs against. One attrset rather than seven
      # bindings because they all move together on a bump: a tag changed without
      # its hashes fails with a bare mismatch and no hint which of them is
      # stale. Overridable as a unit, see legacyPackages.withIcRelease.
      defaultIcRelease = {
        tag = "release-2026-10-16_03-29-base";
        commit = "661a9806c9eef7106475c396e6348fba63e74066";

        canistersTarHash = "sha256-ZIAz2hQXj+oRkEY4O3gCxtCpb1PD8i0GlQJCxLn1gkw=";

        # Keyed by nix system, but the release names the arm64 assets
        # "arm64-*" (see pocketIcPlatform). Every entry moves on an IC release
        # bump, not just the one this machine builds.
        pocketIcHashes = {
          "x86_64-linux" = "sha256-59mkz5wCEayrVfcGYBhectgIfswDZYDGO9F4mgSDQcs=";
          "aarch64-linux" = "sha256-J8zt1T/EgjoYBgNuMifqycLjCIOYhiLMKTWZM4JUCxc=";
          "x86_64-darwin" = "sha256-PqizlHOrHNit2bDjOfdMHBrBGy7H+NZ1br6TtZKn3Hk=";
          "aarch64-darwin" = "sha256-MzKQNyU98vw++kBLE2uv2TeKdZqqk2/bjLLH0Je/4As=";
        };

        # Fixed-output hashes for the codegen derivations, which read the
        # release's .did and .proto files. Set to fakeHash and run `nix build`
        # to learn the real values, then paste them back.
        genBindingsHash = "sha256-xlHOhNnv7lYWnHMyI1ucU2GiGQ849EQi/uFX7ZPfiqc=";
        genProtoHash = "sha256-RN8rWv91ujHc3P2ZJaxlRWS7j8XuxWLeoiFJzXnpcmY=";
      };
    in
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs { inherit system; };

        # alp version: read from nns/version.go, the same string the release
        # workflow asserts against the tag. Not from self.ref, which a
        # tag-pinned flake input does not expose. A tree with no rev is not the
        # release commit, so it says so.
        declaredVersion =
          let
            m = builtins.match ''.*const Version = "([^"]+)".*'' (builtins.readFile ./nns/version.go);
          in
          if m == null then throw "flake.nix: cannot parse Version from nns/version.go" else builtins.head m;
        alpVersion = if self ? rev then declaredVersion else "${declaredVersion}-dirty";

        # These two move when the Go deps or the npm lockfile move, not when the
        # IC release does, so they stay out of icRelease.
        alpVendorHash = "sha256-Gf6nXUxR5rp9EAHHuq2iIax5UJyMF7gfmr4I1aYshlU=";
        siteNpmDepsHash = "sha256-qXcEt1n1jtucPf8YGAMWJ7+tkCMKVpcDzgLAumLVsEQ=";

        pocketIcPlatform =
          {
            "x86_64-linux" = "x86_64-linux";
            "aarch64-linux" = "arm64-linux";
            "x86_64-darwin" = "x86_64-darwin";
            "aarch64-darwin" = "arm64-darwin";
          }
          .${system};

        # Everything downstream of the IC release, as a function of it.
        # makeOverridable gives the result an .override that merges into the
        # defaults, so a caller states only what it changes.
        forRelease = pkgs.lib.makeOverridable (
          icRelease:
          let
            icCanisters = pkgs.stdenv.mkDerivation {
              pname = "ic-canisters";
              version = icRelease.tag;
              src = pkgs.fetchurl {
                url = "https://github.com/dfinity/ic/releases/download/${icRelease.tag}/canisters.tar";
                hash = icRelease.canistersTarHash;
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
              version = icRelease.tag;
              src = pkgs.fetchurl {
                url = "https://github.com/dfinity/ic/releases/download/${icRelease.tag}/pocket-ic-${pocketIcPlatform}.gz";
                hash = icRelease.pocketIcHashes.${system};
              };
              dontUnpack = true;
              nativeBuildInputs = [
                pkgs.gzip
              ]
              ++ pkgs.lib.optionals pkgs.stdenv.hostPlatform.isLinux [ pkgs.autoPatchelfHook ];
              buildInputs = pkgs.lib.optionals pkgs.stdenv.hostPlatform.isLinux [
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
              version = icRelease.tag;
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
                # Run via bash, not the shebang: the sandbox has no /usr/bin/env.
                bash ./generate.sh
              '';
              installPhase = "cp -r gen $out";
              outputHashMode = "recursive";
              outputHashAlgo = "sha256";
              outputHash = icRelease.genBindingsHash;
            };

            # IC protobuf types (nns/pb/) are generated from .proto files fetched from
            # the pinned IC commit over the network, so this is also a fixed-output
            # derivation. protoc/protoc-gen-go come from nixpkgs.
            genProto = pkgs.stdenv.mkDerivation {
              pname = "alpage-gen-proto";
              version = icRelease.tag;
              src = ./.;
              nativeBuildInputs = [
                pkgs.protobuf
                pkgs.protoc-gen-go
                pkgs.curl
                pkgs.cacert
                pkgs.go
              ];
              IC_RELEASE_COMMIT = icRelease.commit;
              buildPhase = ''
                export HOME="$TMPDIR"
                bash ./generate-proto.sh
              '';
              installPhase = "cp -r nns/pb $out";
              outputHashMode = "recursive";
              outputHashAlgo = "sha256";
              outputHash = icRelease.genProtoHash;
            };

            alp = pkgs.buildGoModule {
              pname = "alp";
              version = alpVersion;
              src = ./.;
              vendorHash = alpVendorHash;
              # Drop in the generated code the offline build cannot produce itself.
              preBuild = ''
                cp -r ${genBindings} gen
                cp -r ${genProto} nns/pb
                chmod -R u+w gen nns/pb
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

            devShell = pkgs.mkShell {
              packages = [
                pkgs.go
                pkgs.gopls
                pkgs.nixfmt
                pkgs.protobuf
                pkgs.protoc-gen-go
                pkgs.nodejs_24
                pocketIc
              ];

              # Fail on a Go older than go.mod wants, instead of downloading one.
              GOTOOLCHAIN = "local";

              POCKET_IC_BIN = "${pocketIc}/bin/pocket-ic";
              IC_CANISTERS_DIR = "${icCanisters}";
              IC_RELEASE_COMMIT = icRelease.commit;
              GOVERNANCE_WASM = "${icCanisters}/governance-canister_test.wasm.gz";
              REGISTRY_WASM = "${icCanisters}/registry-canister.wasm.gz";
              ROOT_WASM = "${icCanisters}/root-canister.wasm.gz";

              shellHook = ''
                if [ -t 1 ]; then
                  echo "alpage dev shell"
                  echo "  go         $(go version | awk '{print $3}')"
                  echo "  pocket-ic  ${icRelease.tag} (ic @ ${builtins.substring 0 12 icRelease.commit})"
                  echo "  canisters  ${icRelease.tag} (ic @ ${builtins.substring 0 12 icRelease.commit})"
                fi
              '';
            };
          in
          {
            inherit
              icCanisters
              pocketIc
              genBindings
              genProto
              alp
              devShell
              ;
          }
        ) defaultIcRelease;

        # The landing page (web/). src is the repo root, not web/, because the
        # docs pages render ../docs/*.md at build time; the filter keeps the
        # closure to just those two trees so a Go edit does not rebuild the site.
        # Independent of the IC release, so it stays outside forRelease.
        site = pkgs.buildNpmPackage {
          pname = "alpage-site";
          version = alpVersion;
          src = pkgs.lib.cleanSourceWith {
            name = "source";
            src = ./.;
            filter =
              path: type:
              let
                rel = pkgs.lib.removePrefix (toString ./. + "/") (toString path);
              in
              rel == "web" || pkgs.lib.hasPrefix "web/" rel || rel == "docs" || pkgs.lib.hasPrefix "docs/" rel;
          };
          sourceRoot = "source/web";
          npmDepsHash = siteNpmDepsHash;
          nodejs = pkgs.nodejs_24;
          # sharp ships prebuilt binaries it wants to fetch; astro only needs it
          # for image optimisation, which this site does not use.
          npmFlags = [ "--ignore-scripts" ];
          installPhase = ''
            runHook preInstall
            cp -r dist $out
            runHook postInstall
          '';
        };
      in
      {
        packages = {
          inherit (forRelease)
            icCanisters
            pocketIc
            genBindings
            genProto
            alp
            ;
          inherit site;
          default = forRelease.alp;
        };

        formatter = pkgs.nixfmt;

        devShells.default = forRelease.devShell;

        # legacyPackages is the escape hatch for things that are not plain
        # derivations. withIcRelease builds the same outputs against another IC
        # release without forking this flake:
        #
        #   alpage.legacyPackages.${system}.withIcRelease {
        #     tag = "release-...-base"; commit = "..."; canistersTarHash = "...";
        #     pocketIcHashes = { ... }; genBindingsHash = "..."; genProtoHash = "...";
        #   }
        #
        # Whatever it leaves out keeps the default, which is why a partial
        # override fails on a hash rather than silently building the old release.
        legacyPackages = {
          inherit defaultIcRelease;
          withIcRelease = forRelease.override;
        };
      }
    );
}
