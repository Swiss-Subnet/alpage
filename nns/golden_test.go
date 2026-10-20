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

// goldenMainnetProviders stands in for the live provider list DryRunSeeds is
// handed. It names the provider the remove fixture targets (so that one seeds
// and reaches submission) and not the one the add fixture registers (so that
// one is not rejected as a duplicate).
var goldenMainnetProviders = []string{
	"dchi6-uidam-bqgay-dambq-gayda-mbqga-ydamb-qgayd-ambqg-aydam-bqg",
}

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
			// Each kind asks for the state its own dry run needs, which is
			// what apply does. remove_nodes is the exception: it declares a
			// mirror scope rather than seeds, and still needs a subnet list to
			// exist at all.
			state := DryRunState{}
			if seeder, ok := action.(DryRunSeeder); ok {
				state = seeder.DryRunSeeds(goldenMainnetProviders)
			}
			if spec.Kind == "remove_nodes" {
				state.Subnets = goldenSubnetSeeds
			}
			n, _, _ := startNNSWith(t, principal.Principal{}, state, false)
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
