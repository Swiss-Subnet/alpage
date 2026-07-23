package nns

import (
	"fmt"
	"strings"
)

// OpKind is the direction of a single membership change.
type OpKind string

const (
	OpAdd    OpKind = "add"
	OpRemove OpKind = "remove"
)

// Op is one planned membership change checked against current on-chain state.
// Warn is set when the operation is a no-op or conflicts with reality: adding a
// node that is already a member, or removing one that is not.
type Op struct {
	Kind   OpKind
	NodeID string
	Warn   bool
	Reason string
}

// ResizePlan is the reconciliation of a resize proposal against a subnet's
// current on-chain membership.
type ResizePlan struct {
	SubnetID string
	Ops      []Op
}

// PlanResize diffs a resize proposal against the subnet's current membership
// (textual principals, as returned by FetchSubnetMembership). Pure: the caller
// fetches membership and passes it in.
func PlanResize(r ResizeProposal, current []string) ResizePlan {
	member := make(map[string]bool, len(current))
	for _, id := range current {
		member[id] = true
	}
	plan := ResizePlan{SubnetID: r.SubnetID.Encode()}
	for _, n := range r.NodeIDsAdd {
		id := n.Encode()
		op := Op{Kind: OpAdd, NodeID: id}
		if member[id] {
			op.Warn, op.Reason = true, "already a member; add is a no-op"
		}
		plan.Ops = append(plan.Ops, op)
	}
	for _, n := range r.NodeIDsRemove {
		id := n.Encode()
		op := Op{Kind: OpRemove, NodeID: id}
		if !member[id] {
			op.Warn, op.Reason = true, "not currently a member; remove is a no-op"
		}
		plan.Ops = append(plan.Ops, op)
	}
	return plan
}

func (p ResizePlan) HasWarnings() bool {
	for _, op := range p.Ops {
		if op.Warn {
			return true
		}
	}
	return false
}

// AllNoOp reports whether every op in a non-empty plan is a no-op, i.e. the
// proposal would change nothing on-chain. This is the strongest refuse signal
// for apply: a resize that does nothing (e.g. one already executed) should not
// be resubmitted.
func (p ResizePlan) AllNoOp() bool {
	if len(p.Ops) == 0 {
		return false
	}
	for _, op := range p.Ops {
		if !op.Warn {
			return false
		}
	}
	return true
}

// PreflightLevel grades a preflight result across all kinds.
type PreflightLevel int

const (
	PreflightClean PreflightLevel = iota // nothing to flag; safe to submit
	PreflightWarn                        // worth review but not blocking
	PreflightNoOp                        // changes nothing on-chain; refuse without --force
)

// Preflight is a kind's reconciliation against live state. Report is empty when
// Clean; non-empty otherwise.
type Preflight struct {
	Report string
	Level  PreflightLevel
}

// resizePreflight grades a resize plan: no warnings is Clean (empty report),
// all-no-op is NoOp, any real op alongside a warning is Warn.
func resizePreflight(p ResizePlan) Preflight {
	if !p.HasWarnings() {
		return Preflight{}
	}
	level := PreflightWarn
	if p.AllNoOp() {
		level = PreflightNoOp
	}
	return Preflight{p.Render(), level}
}

// planDeployGuestos compares a deploy_guestos target version against the
// subnet's current version. Matching versions change nothing.
func planDeployGuestos(target, current string) Preflight {
	if target == current {
		return Preflight{fmt.Sprintf("subnet already runs replica version %s; deploy is a no-op\n", current), PreflightNoOp}
	}
	return Preflight{fmt.Sprintf("replica version %s -> %s\n", current, target), PreflightClean}
}

func (p ResizePlan) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "subnet %s\n", p.SubnetID)
	warnings := 0
	for _, op := range p.Ops {
		sign := "+"
		if op.Kind == OpRemove {
			sign = "-"
		}
		fmt.Fprintf(&b, "  %s %s", sign, op.NodeID)
		if op.Warn {
			warnings++
			fmt.Fprintf(&b, "  WARNING: %s", op.Reason)
		}
		b.WriteByte('\n')
	}
	if warnings > 0 {
		fmt.Fprintf(&b, "%d warning(s)\n", warnings)
	} else {
		b.WriteString("no warnings; plan matches current membership\n")
	}
	return b.String()
}
