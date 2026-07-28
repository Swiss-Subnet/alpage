package nns

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

const (
	verA = "b6c0f508b9028382b68786cafe654ec8b32e319e"
	verB = "992628c4f26cc7320396b6b8be37133a666a4386"
)

func TestReconcileNodeVersionsClassifies(t *testing.T) {
	r := &Resources{
		Nodes: []NodeRes{
			{Name: "ok", ID: "n-ok", GuestosVersion: verA},
			{Name: "drifted", ID: "n-drift", GuestosVersion: verA},
			{Name: "undeclared", ID: "n-undeclared"},
			{Name: "unreachable", ID: "n-down", GuestosVersion: verA},
			{Name: "gone", ID: "n-gone", GuestosVersion: verA, Decommissioned: true},
		},
		labels: map[string]string{},
	}
	live := map[string]NodeVersion{
		"n-ok":    {Version: verA},
		"n-drift": {Version: verB},
		"n-down":  {Err: "dial tcp: no route to host"},
	}
	vr := ReconcileNodeVersions(r, live)

	got := map[string]NodeVersionStatus{}
	for _, row := range vr.Nodes {
		got[row.NodeID] = row.Status
	}
	want := map[string]NodeVersionStatus{
		"n-ok":    NodeVersionOK,
		"n-drift": NodeVersionMismatch,
		"n-down":  NodeVersionUnreachable,
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("node %s: status = %q, want %q", id, got[id], w)
		}
	}
	// A node with no declared version is not checked, and a decommissioned node
	// is not expected to answer.
	for _, skipped := range []string{"n-undeclared", "n-gone"} {
		if _, ok := got[skipped]; ok {
			t.Errorf("node %s should not be reported, got %q", skipped, got[skipped])
		}
	}
}

// An unreachable node is not version drift: it is a different failure, and
// treating it as drift would make reconcile fail on any node that is merely down.
func TestUnreachableNodeIsNotDrift(t *testing.T) {
	r := &Resources{
		Nodes:  []NodeRes{{Name: "n", ID: "n-down", GuestosVersion: verA}},
		labels: map[string]string{},
	}
	vr := ReconcileNodeVersions(r, map[string]NodeVersion{"n-down": {Err: "timeout"}})
	if vr.HasDrift() {
		t.Error("unreachable node reported as drift")
	}
	var b strings.Builder
	vr.Render(&b)
	if !strings.Contains(b.String(), "unreachable") {
		t.Errorf("unreachable node not surfaced in output:\n%s", b.String())
	}
}

func TestVersionMismatchIsDrift(t *testing.T) {
	r := &Resources{
		Nodes:  []NodeRes{{Name: "n", ID: "n1", GuestosVersion: verA}},
		labels: map[string]string{},
	}
	vr := ReconcileNodeVersions(r, map[string]NodeVersion{"n1": {Version: verB}})
	if !vr.HasDrift() {
		t.Error("version mismatch not reported as drift")
	}
	var b strings.Builder
	vr.Render(&b)
	// Both versions appear so the operator can see what to deploy.
	for _, want := range []string{verA[:12], verB[:12]} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("output missing %s:\n%s", want, b.String())
		}
	}
}

func TestNoDeclaredVersionsRendersNothing(t *testing.T) {
	r := &Resources{Nodes: []NodeRes{{Name: "n", ID: "n1"}}, labels: map[string]string{}}
	vr := ReconcileNodeVersions(r, nil)
	var b strings.Builder
	vr.Render(&b)
	if b.String() != "" {
		t.Errorf("rendered output with nothing declared:\n%s", b.String())
	}
	if vr.HasDrift() {
		t.Error("drift reported with nothing declared")
	}
}

func TestNodeRegistrationReadsHttpEndpoint(t *testing.T) {
	st, ok := nodeRegistration([]nodeRecord{
		{Version: 2, Value: &nodeRecordValue{
			NodeOperatorID: "op", Http: &nodeRecordAddr{IPAddr: "2403:f000::1", Port: 8080},
		}},
		{Version: 1, Value: &nodeRecordValue{NodeOperatorID: "op"}},
	})
	if !ok {
		t.Fatal("registered node reported as absent")
	}
	if st.HttpIP != "2403:f000::1" || st.HttpPort != 8080 {
		t.Errorf("endpoint = %s/%d", st.HttpIP, st.HttpPort)
	}
}

// A bare IPv6 address must be bracketed or the URL is unparseable.
func TestNodeStatusURLBracketsIPv6(t *testing.T) {
	if got := NodeStatusURL("2403:f000::1", 8080); got != "https://[2403:f000::1]:8080" {
		t.Errorf("url = %s", got)
	}
	if got := NodeStatusURL("192.0.2.1", 8080); got != "https://192.0.2.1:8080" {
		t.Errorf("url = %s", got)
	}
}

// FetchNodeVersion reads impl_version from a replica's /api/v2/status CBOR, the
// same field rs/http_endpoints/public/src/status.rs serves.
func TestFetchNodeVersionReadsImplVersion(t *testing.T) {
	rootKey, _ := hex.DecodeString("deadbeef")
	body, err := cbor.Marshal(map[string]any{
		"impl_version":          verA,
		"replica_health_status": "healthy",
		"root_key":              rootKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/api/v2/status" {
			t.Errorf("unexpected path %s", req.URL.Path)
		}
		w.Header().Set("Content-Type", "application/cbor")
		w.Write(body)
	}))
	defer srv.Close()

	got, err := FetchNodeVersion(srv.URL)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if got != verA {
		t.Errorf("version = %q, want %q", got, verA)
	}
}

func TestFetchNodeVersionFromDashboard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !strings.HasSuffix(req.URL.Path, "/api/v3/nodes/n1") {
			t.Errorf("unexpected path %s", req.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"node_id":"n1","guestos_version":%q,"status":"UP"}`, verA)
	}))
	defer srv.Close()

	got, err := FetchNodeVersionFromDashboard(srv.URL, "n1")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if got != verA {
		t.Errorf("version = %q, want %q", got, verA)
	}
}

func TestFetchNodeVersionFromDashboardMissingVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"node_id":"n1","status":"UP"}`)
	}))
	defer srv.Close()

	if _, err := FetchNodeVersionFromDashboard(srv.URL, "n1"); err == nil {
		t.Fatal("missing guestos_version accepted")
	}
}

// A row sourced from the dashboard is marked, so the rendered output can warn
// that the value may lag the node's real state.
func TestDashboardSourcedRowIsMarked(t *testing.T) {
	r := &Resources{
		Nodes: []NodeRes{
			{Name: "direct", ID: "n-direct", GuestosVersion: verA},
			{Name: "fellback", ID: "n-fell", GuestosVersion: verA},
			{Name: "drifted", ID: "n-drift", GuestosVersion: verA},
		},
		labels: map[string]string{},
	}
	vr := ReconcileNodeVersions(r, map[string]NodeVersion{
		"n-direct": {Version: verA},
		"n-fell":   {Version: verA, Indirect: true},
		"n-drift":  {Version: verB, Indirect: true},
	})
	src := map[string]bool{}
	for _, row := range vr.Nodes {
		src[row.NodeID] = row.Indirect
	}
	if src["n-direct"] {
		t.Error("directly-read row marked indirect")
	}
	if !src["n-fell"] {
		t.Error("dashboard-sourced row not marked indirect")
	}

	var b strings.Builder
	vr.Render(&b)
	out := b.String()
	// The caveat is stated once, not per row.
	if n := strings.Count(out, "note:"); n != 1 {
		t.Errorf("dashboard caveat appears %d times, want 1:\n%s", n, out)
	}
	if !strings.Contains(out, "may lag") {
		t.Errorf("output does not warn about staleness:\n%s", out)
	}
	// Indirect rows are individually identifiable.
	if !strings.Contains(out, "via dashboard") {
		t.Errorf("indirect rows not marked in output:\n%s", out)
	}
}

// Drift confirmed only via the dashboard is still drift: the version is real,
// just possibly stale.
func TestIndirectMismatchIsStillDrift(t *testing.T) {
	r := &Resources{
		Nodes:  []NodeRes{{Name: "n", ID: "n1", GuestosVersion: verA}},
		labels: map[string]string{},
	}
	vr := ReconcileNodeVersions(r, map[string]NodeVersion{"n1": {Version: verB, Indirect: true}})
	if !vr.HasDrift() {
		t.Error("indirect mismatch not reported as drift")
	}
}

// With no indirect rows the caveat is absent entirely.
func TestNoDashboardNoteWhenAllDirect(t *testing.T) {
	r := &Resources{
		Nodes:  []NodeRes{{Name: "n", ID: "n1", GuestosVersion: verA}},
		labels: map[string]string{},
	}
	vr := ReconcileNodeVersions(r, map[string]NodeVersion{"n1": {Version: verA}})
	var b strings.Builder
	vr.Render(&b)
	if strings.Contains(b.String(), "dashboard") {
		t.Errorf("dashboard caveat shown with no indirect rows:\n%s", b.String())
	}
}

// The boundary node blanks impl_version (handlers.rs sets it to None), so an
// absent field must be a clear error rather than an empty-string "match".
func TestFetchNodeVersionAbsentImplVersion(t *testing.T) {
	body, _ := cbor.Marshal(map[string]any{"replica_health_status": "healthy"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	if _, err := FetchNodeVersion(srv.URL); err == nil {
		t.Fatal("absent impl_version accepted")
	}
}
