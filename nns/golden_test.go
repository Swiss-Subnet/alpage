package nns

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/aviate-labs/agent-go/principal"
)

var update = flag.Bool("update", false, "update golden files")

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
			n := startNNS(t, principal.Principal{})
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
