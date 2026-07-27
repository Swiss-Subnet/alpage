package nns

import (
	"fmt"
	"strings"
	"time"
)

// ProposalState is a proposal's lifecycle state as recorded in state.json. It
// mirrors governance's status enum, persisted so `list` can reason about
// terminal proposals without a network round-trip. An empty value means the
// state was never observed (entries predating lifecycle tracking).
type ProposalState string

const (
	StateOpen     ProposalState = "open"
	StateAdopted  ProposalState = "adopted"
	StateExecuted ProposalState = "executed"
	StateRejected ProposalState = "rejected"
	StateFailed   ProposalState = "failed"
)

// Terminal reports whether the proposal can no longer change on-chain. A
// terminal proposal's payload is immutable history: its hash is a record of
// what was submitted, so a config change since then is inert rather than
// actionable.
func (s ProposalState) Terminal() bool {
	switch s {
	case StateExecuted, StateRejected, StateFailed:
		return true
	}
	return false
}

// stateFromGovernance maps governance's numeric status enum onto the persisted
// state. An unrecognized value yields the empty state rather than a guess.
func stateFromGovernance(status int32) ProposalState {
	switch status {
	case 1:
		return StateOpen
	case 2:
		return StateRejected
	case 3:
		return StateAdopted
	case 4:
		return StateExecuted
	case 5:
		return StateFailed
	}
	return ProposalState("")
}

// stateFromDashboard maps the dashboard's uppercase status strings onto the
// persisted state, for proposals governance has purged.
func stateFromDashboard(label string) ProposalState {
	switch strings.ToUpper(strings.TrimSpace(label)) {
	case "OPEN":
		return StateOpen
	case "ADOPTED":
		return StateAdopted
	case "EXECUTED":
		return StateExecuted
	case "REJECTED":
		return StateRejected
	case "FAILED":
		return StateFailed
	}
	return ProposalState("")
}

// resolutionTime picks the on-chain timestamp at which a proposal reached its
// terminal state and renders it RFC3339: execution for executed, failure for
// failed, the decision itself for rejected (it never executes). Non-terminal
// states have no resolution time. A zero timestamp yields "" rather than the
// unix epoch.
func resolutionTime(state ProposalState, executed, failed, decided uint64) string {
	var ts uint64
	switch state {
	case StateExecuted:
		ts = executed
	case StateFailed:
		ts = failed
	case StateRejected:
		ts = decided
	default:
		return ""
	}
	if ts == 0 {
		ts = decided
	}
	return unixRFC3339(ts)
}

// unixRFC3339 renders an on-chain unix timestamp as RFC3339 UTC. A zero
// timestamp is "not set", not the unix epoch.
func unixRFC3339(ts uint64) string {
	if ts == 0 {
		return ""
	}
	return time.Unix(int64(ts), 0).UTC().Format(time.RFC3339)
}

// ListLine renders one proposal's config-vs-state comparison, offline. hash is
// the payload hash as currently declared. Drift against a terminal proposal is
// reported as inert: the payload is immutable history and can never be
// resubmitted under this name, so the config having moved on is expected rather
// than a problem to fix.
func ListLine(name string, e Entry, hash string) string {
	state := ""
	if e.Status != "" {
		state = fmt.Sprintf("  %s", e.Status)
	}
	switch {
	case e.PayloadSHA256 == hash:
		return fmt.Sprintf("%-24s  proposal %d%s  (in sync)", name, e.ProposalID, state)
	case e.Status.Terminal():
		return fmt.Sprintf("%-24s  proposal %d%s  (config has since changed; payload is immutable)", name, e.ProposalID, state)
	default:
		return fmt.Sprintf("%-24s  proposal %d%s  (DRIFT: config payload changed since submit)", name, e.ProposalID, state)
	}
}
