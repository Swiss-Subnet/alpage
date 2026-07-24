package nns

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/aviate-labs/agent-go"
	"github.com/swiss-subnet/alpage/gen/governance"
)

// Source records where a proposal's status came from. Governance is
// authoritative but purges older proposals; the ICP dashboard API retains them,
// so a dashboard-sourced status is flagged and surfaced explicitly.
type Source string

const (
	SourceGovernance Source = "governance"
	SourceDashboard  Source = "dashboard"
)

// DashboardAPI is the public ICP dashboard proposal endpoint, used as a
// read-only fallback when governance has purged a proposal we recorded.
const DashboardAPI = "https://ic-api.internetcomputer.org/api/v3/proposals"

// ProposalStatus is the normalized on-chain status of one proposal, from either
// governance or the dashboard fallback.
type ProposalStatus struct {
	Source  Source
	Label   string
	Yes     uint64
	No      uint64
	Total   uint64 // total available voting power; Total-Yes-No did not vote
	Failure string
}

// FetchProposalStatus resolves a recorded proposal's live status. It queries
// governance first; if governance no longer knows the id (purged), it falls
// back to the ICP dashboard API. Returns nil, nil when neither source has the
// proposal (never existed / genuinely gone).
func FetchProposalStatus(host string, fetchRootKey bool, id uint64) (*ProposalStatus, error) {
	info, err := fetchProposalInfo(host, fetchRootKey, id)
	if err != nil {
		return nil, err
	}
	if info != nil {
		return statusFromGovernance(info), nil
	}
	return fetchDashboardStatus(id)
}

func fetchProposalInfo(host string, fetchRootKey bool, id uint64) (*governance.ProposalInfo, error) {
	if host == "" {
		host = MainnetHost
	}
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("parse host %q: %w", host, err)
	}
	a, err := governance.NewGovernanceAgent(GovernanceID, agent.Config{
		ClientConfig: clientOptions(u),
		FetchRootKey: fetchRootKey,
	})
	if err != nil {
		return nil, fmt.Errorf("new governance agent: %w", err)
	}
	var out *governance.ProposalInfo
	if err := a.Query(GovernanceID, "get_proposal_info", []any{id}, []any{&out}); err != nil {
		return nil, fmt.Errorf("get_proposal_info(%d): %w", id, err)
	}
	return out, nil
}

func statusFromGovernance(info *governance.ProposalInfo) *ProposalStatus {
	ps := &ProposalStatus{Source: SourceGovernance, Label: label(statusName, info.Status)}
	if t := info.LatestTally; t != nil {
		ps.Yes, ps.No, ps.Total = t.Yes, t.No, t.Total
	}
	if fr := info.FailureReason; fr != nil {
		ps.Failure = firstLine(fr.ErrorMessage)
	}
	return ps
}

// fetchDashboardStatus queries the ICP dashboard API. A 404 means the dashboard
// has no such proposal (nil, nil); any other transport/parse error is returned.
func fetchDashboardStatus(id uint64) (*ProposalStatus, error) {
	client := &http.Client{Timeout: queryTimeout}
	resp, err := client.Get(fmt.Sprintf("%s/%d", DashboardAPI, id))
	if err != nil {
		return nil, fmt.Errorf("dashboard get %d: %w", id, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dashboard get %d: http %d", id, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("dashboard read %d: %w", id, err)
	}
	return parseDashboardStatus(body)
}

func parseDashboardStatus(body []byte) (*ProposalStatus, error) {
	var d struct {
		Status      string `json:"status"`
		LatestTally *struct {
			Yes   uint64 `json:"yes"`
			No    uint64 `json:"no"`
			Total uint64 `json:"total"`
		} `json:"latest_tally"`
		FailureReason *struct {
			ErrorMessage string `json:"error_message"`
		} `json:"failure_reason"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("parse dashboard response: %w", err)
	}
	ps := &ProposalStatus{Source: SourceDashboard, Label: d.Status}
	if d.LatestTally != nil {
		ps.Yes, ps.No, ps.Total = d.LatestTally.Yes, d.LatestTally.No, d.LatestTally.Total
	}
	if d.FailureReason != nil {
		ps.Failure = firstLine(d.FailureReason.ErrorMessage)
	}
	return ps, nil
}

func pct(part, whole uint64) float64 {
	if whole == 0 {
		return 0
	}
	return float64(part) / float64(whole) * 100
}

// StatusLine renders one proposal's recorded state against its resolved
// on-chain status. A nil status means neither governance nor the dashboard
// could resolve the id. Dashboard-sourced statuses are flagged explicitly so
// the reader knows governance itself no longer holds the proposal.
func StatusLine(name string, e Entry, ps *ProposalStatus) string {
	if ps == nil {
		return fmt.Sprintf("%-24s  proposal %d  status unknown (not in governance or dashboard)", name, e.ProposalID)
	}
	line := fmt.Sprintf("%-24s  proposal %d  %s  yes=%d no=%d", name, e.ProposalID, ps.Label, ps.Yes, ps.No)
	switch {
	case ps.Total >= ps.Yes+ps.No && ps.Total > 0:
		didNotVote := ps.Total - ps.Yes - ps.No
		line += fmt.Sprintf(" (of total voting power: yes %.1f%% / no %.1f%% / did not vote %.1f%%)",
			pct(ps.Yes, ps.Total), pct(ps.No, ps.Total), pct(didNotVote, ps.Total))
	case ps.Yes+ps.No > 0:
		cast := ps.Yes + ps.No
		line += fmt.Sprintf(" (of votes cast: yes %.1f%% / no %.1f%%)", pct(ps.Yes, cast), pct(ps.No, cast))
	}
	if ps.Failure != "" {
		line += fmt.Sprintf("  failure: %s", ps.Failure)
	}
	if ps.Source == SourceDashboard {
		line += "  [via ICP dashboard; purged from governance]"
	}
	return line
}
