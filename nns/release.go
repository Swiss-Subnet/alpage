package nns

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	registrypb "github.com/swiss-subnet/alpage/nns/pb/registry"
	"google.golang.org/protobuf/proto"
)

// SourceExplorer marks data read from the registry explorer HTTP API rather
// than from the registry canister itself. Like SourceDashboard, it is surfaced
// explicitly: the explorer is a trusted third party, not a trustless read.
const SourceExplorer Source = "registry explorer"

// ElectedVersions records, per version id queried, whether the NNS has elected
// it. Only an elected version can be deployed; deploying anything else fails at
// proposal execution.
//
// Election is read as the presence of a replica_version_<id> registry record,
// not from blessed_replica_versions. That key exists but is not maintained as
// the live elected set -- on mainnet its newest write predates elections that
// have since happened, so versions running in production are absent from it.
//
// The per-version record tracks both directions faithfully. An election
// proposal usually unelects older versions at the same time
// (replica_versions_to_unelect in its payload), which lands as a deletion
// mutation on that version's key, so a version is elected iff its
// highest-versioned record still carries a value.
//
// TODO: this comes from the registry explorer over HTTP, so it inherits the
// same trust caveat as FetchNodeStatus. agent-go ships a certificate-verifying
// KV read path (clients/registry) that would make this trustless; see the same
// TODO on FetchNodeStatus.
type ElectedVersions struct {
	IDs    map[string]bool
	Source Source
	// Measured records, per version, whether its record carries guest launch
	// measurements, which a SEV-enabled subnet requires of the version it runs.
	// A version absent from the map is one whose record could not be decoded,
	// so the answer is unknown rather than false.
	Measured map[string]bool
	// Unverified marks a set that could not be read while the caller chose to
	// proceed anyway (AllowUnverifiedElection). Nothing can be checked against
	// it, so the deploy is warned about rather than blocked or silently passed.
	Unverified bool
}

func (e ElectedVersions) Elected(versionID string) bool { return e.IDs[versionID] }

// Known reports whether any lookup actually resolved. A zero value means the
// source could not be read, which callers must not mistake for "no version is
// elected".
func (e ElectedVersions) Known() bool { return len(e.IDs) > 0 }

// FetchElectedVersions resolves whether each given version id is elected, one
// registry record per version. An error from any lookup fails the whole set:
// a partial answer would silently read as "not elected".
func FetchElectedVersions(explorerBase string, versionIDs ...string) (ElectedVersions, error) {
	base := strings.TrimRight(explorerBase, "/")
	ids := make(map[string]bool, len(versionIDs))
	measured := make(map[string]bool, len(versionIDs))
	for _, v := range versionIDs {
		if v == "" || ids[v] {
			continue
		}
		rec, err := fetchVersionRecord(base, v)
		if err != nil {
			return ElectedVersions{}, err
		}
		ids[v] = rec.Elected
		if rec.MeasuredKnown {
			measured[v] = rec.Measured
		}
	}
	if len(ids) == 0 {
		return ElectedVersions{}, nil
	}
	return ElectedVersions{IDs: ids, Measured: measured, Source: SourceExplorer}, nil
}

func fetchVersionRecord(base, versionID string) (versionRecord, error) {
	client := &http.Client{Timeout: queryTimeout}
	u := base + "/api/records/replica_version_" + url.PathEscape(versionID)
	resp, err := client.Get(u)
	if err != nil {
		return versionRecord{}, fmt.Errorf("get replica_version_%s: %w", versionID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return versionRecord{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return versionRecord{}, fmt.Errorf("replica_version_%s: http %d", versionID, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return versionRecord{}, fmt.Errorf("read replica_version_%s: %w", versionID, err)
	}
	return parseVersionRecord(body)
}

// replicaVersionRecord is one versioned entry of a replica_version_<id> key. A
// nil Value is a deletion mutation: the version was unelected at that version.
// Value itself is the base64 ReplicaVersionRecord protobuf.
type replicaVersionRecord struct {
	Version uint64  `json:"version"`
	Value   *string `json:"value"`
}

// versionRecord is the current state of a replica_version_<id> key.
type versionRecord struct {
	Elected bool
	// Measured reports guest launch measurements on the record, and is
	// meaningful only when MeasuredKnown is set. An unelected version, or one
	// whose value does not decode, leaves it unknown rather than false, since a
	// missing measurement blocks a deploy to a SEV-enabled subnet.
	Measured      bool
	MeasuredKnown bool
}

// parseVersionRecord reduces a version's record history to its current state:
// elected iff the highest-version entry carries a value, measured iff that
// entry's protobuf carries guest_launch_measurements.
func parseVersionRecord(body []byte) (versionRecord, error) {
	var records []replicaVersionRecord
	if err := json.Unmarshal(body, &records); err != nil {
		return versionRecord{}, fmt.Errorf("parse replica_version record: %w", err)
	}
	var latest *replicaVersionRecord
	for i := range records {
		if latest == nil || records[i].Version > latest.Version {
			latest = &records[i]
		}
	}
	if latest == nil || latest.Value == nil {
		return versionRecord{}, nil
	}
	rec := versionRecord{Elected: true}
	blob, err := base64.StdEncoding.DecodeString(*latest.Value)
	if err != nil {
		return rec, nil
	}
	var decoded registrypb.ReplicaVersionRecord
	if err := proto.Unmarshal(blob, &decoded); err != nil {
		return rec, nil
	}
	rec.Measured, rec.MeasuredKnown = decoded.GetGuestLaunchMeasurements() != nil, true
	return rec, nil
}

// Release is the human-readable identity of a GuestOS version: the dfinity/ic
// release it was cut from and the NNS proposal that elected it. The registry
// itself records neither -- a ReplicaVersionRecord carries only the artifact
// hash and URLs -- so this is dashboard-sourced and always attributed.
type Release struct {
	VersionID  string
	Name       string
	ProposalID uint64
	Source     Source
}

// ReleaseURL is the dashboard page for a release, keyed by git revision.
func (r Release) ReleaseURL() string {
	return "https://dashboard.internetcomputer.org/release/" + r.VersionID
}

// ProposalURL is the dashboard page for the election proposal.
func (r Release) ProposalURL() string {
	return fmt.Sprintf("https://dashboard.internetcomputer.org/proposal/%d", r.ProposalID)
}

// Describe renders the release for humans, naming the dashboard as its source.
func (r Release) Describe() string {
	var b strings.Builder
	if r.Name != "" {
		b.WriteString(r.Name)
	} else {
		b.WriteString("unnamed release")
	}
	if r.ProposalID != 0 {
		fmt.Fprintf(&b, ", elected by proposal %d", r.ProposalID)
	}
	fmt.Fprintf(&b, " [via %s]", r.Source)
	return b.String()
}

// dashboardElectionLimit caps how far back the election-proposal scan reads.
// GuestOS elections are frequent, so a recent window resolves current versions;
// an older version simply does not resolve and is reported as such.
const dashboardElectionLimit = 100

// FetchRelease resolves a GuestOS version id to its release name and election
// proposal via the ICP dashboard. Returns nil, nil when the dashboard has no
// election for the version (e.g. older than the scan window).
func FetchRelease(dashboardBase, versionID string) (*Release, error) {
	body, err := fetchElections(dashboardBase)
	if err != nil {
		return nil, err
	}
	return parseRelease(body, versionID)
}

// fetchElections reads the dashboard's recent executed GuestOS elections.
func fetchElections(dashboardBase string) ([]byte, error) {
	u := fmt.Sprintf("%s?limit=%d&include_topic=TOPIC_IC_OS_VERSION_ELECTION&include_status=EXECUTED",
		strings.TrimRight(dashboardBase, "/"), dashboardElectionLimit)
	client := &http.Client{Timeout: queryTimeout}
	resp, err := client.Get(u)
	if err != nil {
		return nil, fmt.Errorf("dashboard elections: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dashboard elections: http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read dashboard elections: %w", err)
	}
	return body, nil
}

// electionProposal is one TOPIC_IC_OS_VERSION_ELECTION proposal. The elected
// version is payload.replica_version_to_elect; the same proposal usually
// unelects older versions. The release name lives only in the summary markdown.
type electionProposal struct {
	ID      uint64 `json:"id"`
	Summary string `json:"summary"`
	Payload struct {
		ReplicaVersionToElect    string   `json:"replica_version_to_elect"`
		ReplicaVersionsToUnelect []string `json:"replica_versions_to_unelect"`
	} `json:"payload"`
}

func parseRelease(body []byte, versionID string) (*Release, error) {
	var d struct {
		Data []electionProposal `json:"data"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("parse dashboard elections: %w", err)
	}
	for _, p := range d.Data {
		if p.Payload.ReplicaVersionToElect != versionID {
			continue
		}
		return &Release{
			VersionID:  versionID,
			Name:       releaseName(p.Summary),
			ProposalID: p.ID,
			Source:     SourceDashboard,
		}, nil
	}
	return nil, nil
}

// releaseNameRe pulls the release tag out of an election summary, whose first
// line reads "Release Notes for [release-2026-07-23_04-21-base](...)". The
// escaped underscores are the dashboard's own markdown escaping.
var releaseNameRe = regexp.MustCompile(`Release Notes for \[([^\]]+)\]`)

func releaseName(summary string) string {
	m := releaseNameRe.FindStringSubmatch(summary)
	if len(m) < 2 {
		return ""
	}
	// Some summaries bold the link text ("[**release-...-base**]"). The
	// asterisks are markup, not part of the tag, and left in they would break
	// both the generated block label and the -base variant match.
	return strings.Trim(strings.ReplaceAll(m[1], `\_`, "_"), "* ")
}
