package nns

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A real blessed_replica_versions entry from the registry explorer, truncated
// to three ids. Value is the base64 BlessedReplicaVersions protobuf.
func TestVersionElectedPresentRecord(t *testing.T) {
	// A replica_version_<id> record whose latest entry carries a value means
	// the version is elected.
	const rec = `[{"key":"replica_version_abc","version":62444,"value":"CIA="}]`
	got, err := parseVersionElected([]byte(rec))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !got {
		t.Error("a present record means elected")
	}
}

func TestVersionElectedDeletionMutation(t *testing.T) {
	// Elections routinely unelect older versions (replica_versions_to_unelect in
	// the proposal payload), which lands as a deletion mutation. Real shape:
	// feb8e5f2 was elected at 59942 and unelected by proposal 123216 at 62313.
	// Records arrive unordered, so the highest version must win, not the last.
	const rec = `[
	  {"key":"replica_version_feb8e5f2","version":62313,"value":null},
	  {"key":"replica_version_feb8e5f2","version":59942,"value":"CIA="}
	]`
	got, err := parseVersionElected([]byte(rec))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got {
		t.Error("an unelected version must not count as elected")
	}
}

func TestVersionElectedNoRecord(t *testing.T) {
	// The explorer returns null for a key it has never seen.
	got, err := parseVersionElected([]byte(`null`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got {
		t.Error("a version with no record was never elected")
	}
}

func TestElectedVersionsZeroValueNotKnown(t *testing.T) {
	// An unread source must never look like "nothing is elected".
	var e ElectedVersions
	if e.Known() {
		t.Error("zero value must not report Known")
	}
}

func TestElectedVersionsOnlyAnswersForQueried(t *testing.T) {
	// The set is built per-version, so Known must reflect that a lookup ran even
	// when the answer is "not elected".
	e := ElectedVersions{IDs: map[string]bool{"a": true, "b": false}, Source: SourceExplorer}
	if !e.Known() {
		t.Fatal("a populated set must be Known")
	}
	if !e.Elected("a") {
		t.Error("a should be elected")
	}
	if e.Elected("b") {
		t.Error("b should not be elected")
	}
}

// Shape of the dashboard's election-proposal list, matching proposal 143165.
const electionsJSON = `{"data":[
  {"id":143078,"summary":"Release Notes for [release-2026-07-23\\_04-21-security-hotfix](https://github.com/dfinity/ic/tree/x)\n\nblah","payload":{"replica_version_to_elect":"b6c0f508b9028382b68786cafe654ec8b32e319e"}},
  {"id":143165,"summary":"Release Notes for [release-2026-07-23\\_04-21-deterministic-tracker-security-hotfix](https://github.com/dfinity/ic/tree/y)\n\nblah","payload":{"replica_version_to_elect":"bd3d261559a96ef4f55521111527b5e2ff6242a6"}}
]}`

func TestParseReleaseMatchesVersion(t *testing.T) {
	got, err := parseRelease([]byte(electionsJSON), "bd3d261559a96ef4f55521111527b5e2ff6242a6")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got == nil {
		t.Fatal("expected a release for an elected version")
	}
	if got.ProposalID != 143165 {
		t.Errorf("proposal = %d, want 143165", got.ProposalID)
	}
	// The dashboard escapes underscores in markdown; the tag must round-trip.
	if want := "release-2026-07-23_04-21-deterministic-tracker-security-hotfix"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
	if got.Source != SourceDashboard {
		t.Errorf("source = %q, want %q", got.Source, SourceDashboard)
	}
}

func TestParseReleaseUnknownVersion(t *testing.T) {
	got, err := parseRelease([]byte(electionsJSON), "deadbeef")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != nil {
		t.Errorf("unknown version should resolve to nil, got %+v", got)
	}
}

func TestReleaseURLs(t *testing.T) {
	r := Release{VersionID: "bd3d261559a96ef4f55521111527b5e2ff6242a6", ProposalID: 143165}
	if want := "https://dashboard.internetcomputer.org/release/bd3d261559a96ef4f55521111527b5e2ff6242a6"; r.ReleaseURL() != want {
		t.Errorf("release url = %q, want %q", r.ReleaseURL(), want)
	}
	if want := "https://dashboard.internetcomputer.org/proposal/143165"; r.ProposalURL() != want {
		t.Errorf("proposal url = %q, want %q", r.ProposalURL(), want)
	}
}

func TestReleaseDescribeNamesSource(t *testing.T) {
	r := Release{Name: "release-x", ProposalID: 143165, Source: SourceDashboard}
	got := r.Describe()
	for _, want := range []string{"release-x", "143165", string(SourceDashboard)} {
		if !strings.Contains(got, want) {
			t.Errorf("Describe() = %q, missing %q", got, want)
		}
	}
}

func TestFetchElectedVersionsUnreachableErrors(t *testing.T) {
	// A dead explorer must error, not return an empty (thus "nothing elected")
	// set: deploy preflight treats an unreadable elected set as fatal.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	got, err := FetchElectedVersions(srv.URL, "abc")
	if err == nil {
		t.Fatal("an unreadable explorer must error")
	}
	if got.Known() {
		t.Error("a failed fetch must not report a known set")
	}
}

func TestFetchReleaseUnknownVersionIsNotAnError(t *testing.T) {
	// A dashboard that simply has no election for the version resolves to nil
	// without an error, so preflight proceeds without a release name.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":[]}`)
	}))
	defer srv.Close()
	got, err := FetchRelease(srv.URL, "whatever")
	if err != nil {
		t.Fatalf("empty election list should not error: %v", err)
	}
	if got != nil {
		t.Errorf("want nil release, got %+v", got)
	}
}

func TestPreflightUnreadableElectedSetFails(t *testing.T) {
	// A dead explorer blocks the deploy: without the elected set preflight
	// cannot tell whether the proposal would execute.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	d := DeployGuestosAction{SubnetID: subnetX, ReplicaVersionID: "abc"}
	_, err := d.Preflight("", false, WithExplorer(srv.URL))
	if err == nil {
		t.Fatal("an unreadable elected set must fail preflight")
	}
}

func TestPreflightAllowUnverifiedElectionDowngrades(t *testing.T) {
	// With the override, a dead explorer must not fail before the registry is
	// even consulted: the error surfaced must no longer be the elected fetch.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	d := DeployGuestosAction{SubnetID: subnetX, ReplicaVersionID: "abc"}
	_, err := d.Preflight("", false, WithExplorer(srv.URL), AllowUnverifiedElection())
	if err != nil && strings.Contains(err.Error(), "fetch elected versions") {
		t.Fatalf("override must not fail on the elected fetch: %v", err)
	}
}

func TestPlanUnverifiedElectionWarns(t *testing.T) {
	// An unknown elected set with the override set warns rather than blocking.
	pf := planDeployGuestos("newver", "abc123", ElectedVersions{Unverified: true}, nil)
	if pf.Level != PreflightWarn {
		t.Errorf("an unverified election should warn, got %v", pf.Level)
	}
	if !strings.Contains(pf.Report, "not verified") {
		t.Errorf("report should flag the missing verification: %q", pf.Report)
	}
}

func TestReleaseNameMissing(t *testing.T) {
	// A summary without the release-notes header yields no name, not a crash.
	if got := releaseName("Security patch update"); got != "" {
		t.Errorf("releaseName = %q, want empty", got)
	}
}
