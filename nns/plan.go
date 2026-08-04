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

// MembershipPlan is the reconciliation of a membership proposal against a subnet's
// current on-chain membership.
type MembershipPlan struct {
	SubnetID string
	Ops      []Op
}

// PlanMembership diffs a membership proposal against the subnet's current membership
// (textual principals, as returned by FetchSubnetMembership). Pure: the caller
// fetches membership and passes it in.
func PlanMembership(r MembershipProposal, current []string) MembershipPlan {
	member := make(map[string]bool, len(current))
	for _, id := range current {
		member[id] = true
	}
	plan := MembershipPlan{SubnetID: r.SubnetID.Encode()}
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

func (p MembershipPlan) HasWarnings() bool {
	for _, op := range p.Ops {
		if op.Warn {
			return true
		}
	}
	return false
}

// AllNoOp reports whether every op in a non-empty plan is a no-op, i.e. the
// proposal would change nothing on-chain. This is the strongest refuse signal
// for apply: a membership change that does nothing (e.g. one already executed) should not
// be resubmitted.
func (p MembershipPlan) AllNoOp() bool {
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

// membershipPreflight grades a membership plan: no warnings is Clean (empty report),
// all-no-op is NoOp, any real op alongside a warning is Warn.
func membershipPreflight(p MembershipPlan) Preflight {
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
// subnet's current version, the elected-version set, and (optionally) the
// release the dashboard resolved for the target. Matching versions change
// nothing; an unelected target cannot execute at all, so it is refused with the
// same level as a no-op. rel may be nil when the dashboard resolved no release.
func planDeployGuestos(target, current string, elected ElectedVersions, rel *Release) Preflight {
	if target == current {
		return Preflight{fmt.Sprintf("subnet already runs replica version %s; deploy is a no-op\n", current), PreflightNoOp}
	}
	if elected.Known() && !elected.Elected(target) {
		return Preflight{fmt.Sprintf(
			"replica version %s is not elected [per %s]; the NNS would reject this deploy\n",
			target, elected.Source), PreflightNoOp}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "replica version %s -> %s\n", current, target)
	if rel != nil {
		fmt.Fprintf(&b, "  target release: %s\n", rel.Describe())
	}
	if elected.Unverified {
		fmt.Fprintf(&b, "  WARNING: election of %s not verified; the elected set was unreadable\n", target)
		return Preflight{b.String(), PreflightWarn}
	}
	if elected.Known() {
		fmt.Fprintf(&b, "  target is elected [per %s]\n", elected.Source)
	}
	return Preflight{b.String(), PreflightClean}
}

func (p MembershipPlan) Render() string {
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
