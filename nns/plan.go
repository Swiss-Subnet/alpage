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
