package nns

import (
	"strings"
	"testing"
)

// Declared form is hex; the registry serves base64. Each pair is the same 64
// bytes in both encodings.
const (
	testChipA   = "299910b915f5bca7368fcc90c8a2dec744e9437794a5ed68641c35e25ca2570de8c7f27391d4b2ff8c5bd202d15e13eba2262951084d0ce99978de8c27e2a3e8"
	testChipA64 = "KZkQuRX1vKc2j8yQyKLex0TpQ3eUpe1oZBw14lyiVw3ox/JzkdSy/4xb0gLRXhProiYpUQhNDOmZeN6MJ+Kj6A=="
	testChipB   = "b49a397d2fc75c497851812386a01e1d6a5b8fead5551dfe8c06130d62dea0e794ce5ec9a7a5ceb50d37130517e82e103c4783992246de1a87b1064e94e819ee"
	testChipB64 = "tJo5fS/HXEl4UYEjhqAeHWpbj+rVVR3+jAYTDWLeoOeUzl7Jp6XOtQ03EwUX6C4QPEeDmSJG3hqHsQZOlOgZ7g=="
)

func sevRowFor(sr NodeSevReconcile, id string) *NodeSevRow {
	for i := range sr.Nodes {
		if sr.Nodes[i].NodeID == id {
			return &sr.Nodes[i]
		}
	}
	return nil
}

func TestReconcileNodeSev(t *testing.T) {
	r := &Resources{
		Nodes: []NodeRes{
			{Name: "n_match", ID: "n-match", ChipID: testChipA},
			{Name: "n_none", ID: "n-none"},
			{Name: "n_lost", ID: "n-lost", ChipID: testChipA},
			{Name: "n_gained", ID: "n-gained"},
			{Name: "n_swapped", ID: "n-swapped", ChipID: testChipA},
		},
		labels: map[string]string{},
	}
	status := map[string]NodeStatus{
		"n-match":   {Registered: true, ChipID: testChipA64},
		"n-none":    {Registered: true},
		"n-lost":    {Registered: true},
		"n-gained":  {Registered: true, ChipID: testChipA64},
		"n-swapped": {Registered: true, ChipID: testChipB64},
	}

	sr := ReconcileNodeSev(r, status)

	if got := sevRowFor(sr, "n-match"); got == nil || got.Status != NodeSevInSync {
		t.Errorf("n-match: got %+v, want in-sync", got)
	}
	if got := sevRowFor(sr, "n-none"); got == nil || got.Status != NodeSevInSync {
		t.Errorf("n-none: got %+v, want in-sync", got)
	}
	if got := sevRowFor(sr, "n-lost"); got == nil || got.Status != NodeSevMissing {
		t.Errorf("n-lost: got %+v, want missing", got)
	}
	if got := sevRowFor(sr, "n-gained"); got == nil || got.Status != NodeSevUndeclared {
		t.Errorf("n-gained: got %+v, want undeclared", got)
	}
	if got := sevRowFor(sr, "n-swapped"); got == nil || got.Status != NodeSevMismatch {
		t.Errorf("n-swapped: got %+v, want mismatch", got)
	}
	if !sr.HasDrift() {
		t.Error("expected drift")
	}
}

// The whole point of declaring the value rather than a bool: the chip changing
// under a node id is drift even though the node stays attested throughout.
func TestReconcileNodeSevDetectsChipSwap(t *testing.T) {
	r := &Resources{
		Nodes:  []NodeRes{{Name: "n", ID: "n-1", ChipID: testChipA}},
		labels: map[string]string{},
	}
	sr := ReconcileNodeSev(r, map[string]NodeStatus{"n-1": {Registered: true, ChipID: testChipB64}})
	got := sevRowFor(sr, "n-1")
	if got == nil || got.Status != NodeSevMismatch {
		t.Fatalf("n-1: got %+v, want mismatch", got)
	}
	// Both sides render as hex, whatever the registry served.
	if got.Declared != testChipA || got.Live != testChipB {
		t.Errorf("row should carry both chips, got declared=%q live=%q", got.Declared, got.Live)
	}
	if !sr.HasDrift() {
		t.Error("a swapped chip is drift")
	}
}

// A decommissioned node is expected to be absent from the registry, so there is
// no chip_id to compare.
func TestReconcileNodeSevSkipsDecommissioned(t *testing.T) {
	r := &Resources{
		Nodes:  []NodeRes{{Name: "n_old", ID: "n-old", ChipID: testChipA, Decommissioned: true}},
		labels: map[string]string{},
	}
	sr := ReconcileNodeSev(r, map[string]NodeStatus{})
	if len(sr.Nodes) != 0 {
		t.Errorf("expected no rows, got %+v", sr.Nodes)
	}
	if sr.HasDrift() {
		t.Error("a decommissioned node is not sev drift")
	}
}

// Without a fetched record there is nothing to compare: unknown, not drift.
func TestReconcileNodeSevUnknownWhenUnfetched(t *testing.T) {
	r := &Resources{
		Nodes:  []NodeRes{{Name: "n", ID: "n-1", ChipID: testChipA}},
		labels: map[string]string{},
	}
	sr := ReconcileNodeSev(r, map[string]NodeStatus{})
	if got := sevRowFor(sr, "n-1"); got == nil || got.Status != NodeSevUnknown {
		t.Errorf("n-1: got %+v, want unknown", got)
	}
	if sr.HasDrift() {
		t.Error("an unread record is not drift")
	}
}

// A deregistered node has no record to carry a chip_id; that is a registration
// problem the ownership check reports, not sev drift.
func TestReconcileNodeSevSkipsDeregistered(t *testing.T) {
	r := &Resources{
		Nodes:  []NodeRes{{Name: "n", ID: "n-1", ChipID: testChipA}},
		labels: map[string]string{},
	}
	sr := ReconcileNodeSev(r, map[string]NodeStatus{"n-1": {Registered: false}})
	if got := sevRowFor(sr, "n-1"); got == nil || got.Status != NodeSevUnknown {
		t.Errorf("n-1: got %+v, want unknown", got)
	}
	if sr.HasDrift() {
		t.Error("a deregistered node is not sev drift")
	}
}

// The registry serves base64; resources.hcl declares hex, the form AMD's KDS
// takes. Comparison is on the decoded bytes so the two agree.
func TestNormalizeChipID(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		want  string
		valid bool
	}{
		{"hex passes through lowercased", testChipA, testChipA, true},
		{"uppercase hex normalizes", strings.ToUpper(testChipA), testChipA, true},
		{"base64 decodes to the same hex", testChipA64, testChipA, true},
		{"empty stays empty", "", "", true},
		{"wrong length hex is invalid", "299910b9", "", false},
		{"not an encoding at all", "not-a-chip-id", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeChipID(tc.in)
			if (err == nil) != tc.valid {
				t.Fatalf("err = %v, want valid=%v", err, tc.valid)
			}
			if tc.valid && got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// A chip declared in the registry's base64 form still matches: reconcile
// compares decoded bytes, not the literal string.
func TestReconcileNodeSevAcceptsBase64Declaration(t *testing.T) {
	r := &Resources{
		Nodes:  []NodeRes{{Name: "n", ID: "n-1", ChipID: testChipA64}},
		labels: map[string]string{},
	}
	sr := ReconcileNodeSev(r, map[string]NodeStatus{"n-1": {Registered: true, ChipID: testChipA64}})
	if got := sevRowFor(sr, "n-1"); got == nil || got.Status != NodeSevInSync {
		t.Errorf("n-1: got %+v, want in-sync", got)
	}
}

// A chip AMD refuses to vouch for is drift even when the declaration matches
// the registry: config and registry can agree on a chip that is not genuine.
func TestReconcileNodeSevAMDUnverified(t *testing.T) {
	r := &Resources{
		Nodes:  []NodeRes{{Name: "n", ID: "n-1", ChipID: testChipA}},
		labels: map[string]string{},
	}
	sr := ReconcileNodeSev(r, map[string]NodeStatus{"n-1": {Registered: true, ChipID: testChipA64}})
	sr.ApplyChipVerification(map[string]ChipVerification{
		testChipA: {ChipID: testChipA, Err: "AMD KDS does not know this chip"},
	})
	if got := sevRowFor(sr, "n-1"); got == nil || got.Status != NodeSevUnverified {
		t.Errorf("n-1: got %+v, want unverified", got)
	}
	if !sr.HasDrift() {
		t.Error("a chip AMD will not vouch for is drift")
	}
}

// A rate-limited lookup must leave the in-sync verdict alone: inconclusive is
// not evidence against the chip.
func TestReconcileNodeSevAMDInconclusive(t *testing.T) {
	r := &Resources{
		Nodes:  []NodeRes{{Name: "n", ID: "n-1", ChipID: testChipA}},
		labels: map[string]string{},
	}
	sr := ReconcileNodeSev(r, map[string]NodeStatus{"n-1": {Registered: true, ChipID: testChipA64}})
	sr.ApplyChipVerification(map[string]ChipVerification{
		testChipA: {ChipID: testChipA, Inconclusive: true, Err: "AMD KDS rate-limited the lookup"},
	})
	if got := sevRowFor(sr, "n-1"); got == nil || got.Status != NodeSevInSync {
		t.Errorf("n-1: got %+v, want in-sync", got)
	}
	if sr.HasDrift() {
		t.Error("an inconclusive lookup is not drift")
	}
}

func TestReconcileNodeSevAMDVerified(t *testing.T) {
	r := &Resources{
		Nodes:  []NodeRes{{Name: "n", ID: "n-1", ChipID: testChipA}},
		labels: map[string]string{},
	}
	sr := ReconcileNodeSev(r, map[string]NodeStatus{"n-1": {Registered: true, ChipID: testChipA64}})
	sr.ApplyChipVerification(map[string]ChipVerification{
		testChipA: {ChipID: testChipA, Verified: true, Product: "Milan-B0"},
	})
	got := sevRowFor(sr, "n-1")
	if got == nil || got.Status != NodeSevInSync {
		t.Fatalf("n-1: got %+v, want in-sync", got)
	}
	if got.AMD == nil || got.AMD.Product != "Milan-B0" {
		t.Errorf("row should carry the AMD verdict, got %+v", got.AMD)
	}
	if sr.HasDrift() {
		t.Error("a verified chip is not drift")
	}
}

// A chip_id that is not valid hex or base64 is a config error, reported rather
// than silently comparing unequal.
func TestReconcileNodeSevRejectsMalformedDeclaration(t *testing.T) {
	r := &Resources{
		Nodes:  []NodeRes{{Name: "n", ID: "n-1", ChipID: "nonsense"}},
		labels: map[string]string{},
	}
	sr := ReconcileNodeSev(r, map[string]NodeStatus{"n-1": {Registered: true, ChipID: testChipA64}})
	if got := sevRowFor(sr, "n-1"); got == nil || got.Status != NodeSevMalformed {
		t.Errorf("n-1: got %+v, want malformed", got)
	}
	if !sr.HasDrift() {
		t.Error("a malformed chip_id is drift")
	}
}
