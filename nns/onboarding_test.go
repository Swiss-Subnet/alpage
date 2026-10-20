package nns

import (
	"strings"
	"testing"

	"github.com/aviate-labs/agent-go/principal"
)

// TestAddDataCenterWithoutGPS pins that the registry accepts a data center
// record with no coordinates. DataCenterRecord::validate only checks the
// lengths of id, region, and owner, so gps really is optional, and this is the
// test that keeps that true if validate grows a requirement.
func TestAddDataCenterWithoutGPS(t *testing.T) {
	action := AddDataCentersAction{
		Meta: Meta{Title: "no gps", Summary: "data center without coordinates"},
		DataCenters: []DataCenterSpec{{
			ID:     "nogps1",
			Region: "Europe,CH,Zurich",
			Owner:  "Test Owner AG",
		}},
	}
	n, _, _ := startNNSWith(t, principal.Principal{}, action.DryRunSeeds(nil), false)
	pid, err := n.SubmitAs(n.Proposer, n.ProposerNeuron(), action)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	pi, err := n.GetProposalInfo(pid)
	if err != nil {
		t.Fatalf("get proposal info: %v", err)
	}
	if got := Render(pi); !strings.Contains(got, "Status: Executed") {
		t.Errorf("a data center with no gps did not execute, so gps is not optional:\n%s", got)
	}
}
