package nns

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/aviate-labs/agent-go/principal"
)

var update = flag.Bool("update", false, "update golden files")

// The provider the remove-node-provider fixture targets. Governance validates
// AddOrRemoveNodeProvider against its own list, so the fixture only reaches
// submission if the provider is seeded there.
var goldenSubnetSeeds = []SubnetSeed{{
	SubnetID:       subnetX,
	NumNodes:       1,
	ReplicaVersion: "0000000000000000000000000000000000000001",
}}

var goldenProviderSeeds = []ProviderSeed{{
	ProviderID: "dchi6-uidam-bqgay-dambq-gayda-mbqga-ydamb-qgayd-ambqg-aydam-bqg",
	OperatorID: "hsi7b-rl4wt-lum3m-ophfi-oxgx5-u2q7r-ak7ag-nyiik-c4gam-epo2r-3qe",
	DcID:       "dc1",
	DcRegion:   "eu/ch/zurich",
}}

// TestDryRunGolden brings up a local NNS, submits each proposal declared in the
// repo config, and compares the rendered result against a committed golden file.
// Run with -update to regenerate the goldens after an intentional change.
func TestDryRunGolden(t *testing.T) {
	cfg, err := LoadConfig("testdata/golden_src/proposals.hcl")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	for _, spec := range cfg.Proposals {
		t.Run(spec.Name, func(t *testing.T) {
			action, err := spec.Action()
			if err != nil {
				t.Fatalf("action: %v", err)
			}
			// remove_nodes needs a subnet list to exist at all, and the
			// provider fixture additionally needs governance's own
			// node-provider list seeded.
			var providers []ProviderSeed
			var seeds []SubnetSeed
			switch spec.Kind {
			case "remove_node_provider":
				providers, seeds = goldenProviderSeeds, goldenSubnetSeeds
			case "remove_nodes":
				seeds = goldenSubnetSeeds
			}
			n, _, _ := startNNSSeededWithProviders(t, principal.Principal{}, seeds, providers, false)
			pid, err := n.SubmitAs(n.Proposer, n.ProposerNeuron(), action)
			if err != nil {
				t.Fatalf("submit: %v", err)
			}
			pi, err := n.GetProposalInfo(pid)
			if err != nil {
				t.Fatalf("get proposal info: %v", err)
			}
			got := Render(pi)

			golden := filepath.Join("testdata", "golden", spec.Name+".txt")
			if *update {
				if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				t.Logf("wrote %s", golden)
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden (run with -update to create): %v", err)
			}
			if got != string(want) {
				t.Errorf("rendered output differs from %s:\n--- got ---\n%s\n--- want ---\n%s", golden, got, want)
			}
		})
	}
}
