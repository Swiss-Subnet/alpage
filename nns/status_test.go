package nns

import (
	"strings"
	"testing"

	"github.com/swiss-subnet/alpage/gen/governance"
)

func piWithStatus(id uint64, status int32) *governance.ProposalInfo {
	return &governance.ProposalInfo{
		Id:     &governance.ProposalId{Id: id},
		Status: status,
	}
}

func TestStatusFromGovernance(t *testing.T) {
	tests := []struct {
		name   string
		status int32
		want   string
	}{
		{"executed", 4, "Executed"},
		{"open", 1, "Open"},
		{"rejected", 2, "Rejected"},
		{"failed", 5, "Failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := statusFromGovernance(piWithStatus(141235, tt.status))
			if ps.Source != SourceGovernance {
				t.Errorf("source = %q, want governance", ps.Source)
			}
			if !strings.Contains(ps.Label, tt.want) {
				t.Errorf("label %q does not contain %q", ps.Label, tt.want)
			}
		})
	}
}

func TestStatusFromGovernanceFailureReason(t *testing.T) {
	pi := piWithStatus(999, 5)
	pi.FailureReason = &governance.GovernanceError{ErrorMessage: "canister trapped"}
	ps := statusFromGovernance(pi)
	if !strings.Contains(ps.Failure, "canister trapped") {
		t.Errorf("failure = %q, want it to mention the trap", ps.Failure)
	}
}

func TestStatusFromDashboardJSON(t *testing.T) {
	body := []byte(`{"proposal_id":50000,"status":"EXECUTED","latest_tally":{"yes":40350302715620554,"no":0,"total":41010072541292836},"failure_reason":null}`)
	ps, err := parseDashboardStatus(body)
	if err != nil {
		t.Fatal(err)
	}
	if ps.Source != SourceDashboard {
		t.Errorf("source = %q, want dashboard", ps.Source)
	}
	if !strings.Contains(ps.Label, "EXECUTED") {
		t.Errorf("label %q missing EXECUTED", ps.Label)
	}
	if ps.Yes != 40350302715620554 || ps.No != 0 {
		t.Errorf("tally not parsed: yes=%d no=%d", ps.Yes, ps.No)
	}
}

func TestStatusLineMentionsDashboardFallback(t *testing.T) {
	e := Entry{ProposalID: 50000, PayloadSHA256: "abc"}
	ps := &ProposalStatus{Source: SourceDashboard, Label: "EXECUTED", Yes: 5, No: 1}
	line := StatusLine("old-one", e, ps)
	if !strings.Contains(line, "50000") || !strings.Contains(line, "EXECUTED") {
		t.Errorf("missing id/status: %q", line)
	}
	if !strings.Contains(strings.ToLower(line), "dashboard") {
		t.Errorf("dashboard fallback must be explicitly mentioned in %q", line)
	}
}

func TestStatusLineGovernanceNoDashboardMention(t *testing.T) {
	e := Entry{ProposalID: 142931, PayloadSHA256: "abc"}
	ps := &ProposalStatus{Source: SourceGovernance, Label: "Executed (4)", Yes: 5, No: 1}
	line := StatusLine("wave1", e, ps)
	if strings.Contains(strings.ToLower(line), "dashboard") {
		t.Errorf("governance-sourced line should not mention dashboard: %q", line)
	}
}

func TestStatusLineTallyPercentOfTotal(t *testing.T) {
	e := Entry{ProposalID: 1, PayloadSHA256: "abc"}
	// yes=600 no=200 total=1000 -> 200 did not vote.
	ps := &ProposalStatus{Source: SourceGovernance, Label: "Executed (4)", Yes: 600, No: 200, Total: 1000}
	line := StatusLine("p", e, ps)
	for _, want := range []string{"yes 60.0%", "no 20.0%", "did not vote 20.0%"} {
		if !strings.Contains(line, want) {
			t.Errorf("expected %q in %q", want, line)
		}
	}
}

func TestStatusLineTallyFallbackToCast(t *testing.T) {
	e := Entry{ProposalID: 1, PayloadSHA256: "abc"}
	ps := &ProposalStatus{Source: SourceGovernance, Label: "Executed (4)", Yes: 750, No: 250, Total: 0}
	line := StatusLine("p", e, ps)
	if !strings.Contains(line, "votes cast") || !strings.Contains(line, "yes 75.0%") {
		t.Errorf("expected cast-based fallback with 75.0%%, got %q", line)
	}
}

func TestStatusLineTallyZeroNoPercent(t *testing.T) {
	e := Entry{ProposalID: 1, PayloadSHA256: "abc"}
	ps := &ProposalStatus{Source: SourceGovernance, Label: "Open (1)", Yes: 0, No: 0, Total: 0}
	line := StatusLine("p", e, ps)
	if strings.Contains(line, "%") {
		t.Errorf("no votes and no total should not render a percentage, got %q", line)
	}
}

func TestStatusLineUnknown(t *testing.T) {
	e := Entry{ProposalID: 141235, PayloadSHA256: "abc"}
	line := StatusLine("gone", e, nil)
	if !strings.Contains(strings.ToLower(line), "unknown") {
		t.Errorf("nil status should render as unknown: %q", line)
	}
}
