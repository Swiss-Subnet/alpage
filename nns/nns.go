// Package nns brings up the core NNS canisters on a PocketIC instance and
// submits governance proposals against them.
package nns

import (
	"fmt"
	"os"

	"github.com/aviate-labs/agent-go/principal"
)

// Well-known mainnet NNS canister ids. Installing at these exact ids lets
// governance's inter-canister calls to the registry/root resolve as they do
// on mainnet.
var (
	GovernanceID = principal.MustDecode("rrkah-fqaaa-aaaaa-aaaaq-cai")
	RegistryID   = principal.MustDecode("rwlgt-iiaaa-aaaaa-aaaaa-cai")
	RootID       = principal.MustDecode("r7inp-6aaaa-aaaaa-aaabq-cai")
	LifelineID   = principal.MustDecode("rno2w-sqaaa-aaaaa-aaacq-cai")
)

// wasmPaths are read from env (set by the nix devShell) so the bytes always
// come from the pinned release.
type wasmPaths struct {
	governance string
	registry   string
	root       string
}

func wasmPathsFromEnv() (wasmPaths, error) {
	w := wasmPaths{
		governance: os.Getenv("GOVERNANCE_WASM"),
		registry:   os.Getenv("REGISTRY_WASM"),
		root:       os.Getenv("ROOT_WASM"),
	}
	for name, p := range map[string]string{"GOVERNANCE_WASM": w.governance, "REGISTRY_WASM": w.registry, "ROOT_WASM": w.root} {
		if p == "" {
			return w, fmt.Errorf("%s not set; run inside nix develop", name)
		}
		if _, err := os.Stat(p); err != nil {
			return w, fmt.Errorf("%s=%s: %w", name, p, err)
		}
	}
	return w, nil
}

// readWasm returns the raw file bytes. The replica accepts gzipped modules
// directly, so .wasm.gz files are passed through unchanged.
func readWasm(path string) ([]byte, error) {
	return os.ReadFile(path)
}
