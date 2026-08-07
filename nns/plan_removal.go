package nns

import (
	"fmt"
	"sort"
	"strings"
)

// RemovalIssue is one precondition the registry canister enforces, checked
// before a proposal is cut rather than after it is adopted.
type RemovalIssue struct {
	Target string
	Detail string
	// Blocking marks a precondition whose violation makes the proposal fail or
	// silently do nothing, as opposed to a target that is simply already gone.
	Blocking bool
}

// RemovalPlan is the preflight verdict for a removal proposal.
type RemovalPlan struct {
	Kind   string
	Issues []RemovalIssue
	// NoOp is set when every target is already absent, so the proposal would
	// change nothing.
	NoOp bool
}

// planRemoveNodes checks the precondition do_remove_nodes enforces: a node that
// is still a subnet member makes the canister panic, which happens after the
// NNS has already adopted the proposal. inSubnet maps a node id to the subnet
// that still holds it; absent means unassigned. registered marks nodes the
// registry still knows: an unknown node is skipped by the canister rather than
// failing, so it is reported but not blocking.
func planRemoveNodes(nodeIDs []string, inSubnet map[string]string, registered map[string]bool) RemovalPlan {
	p := RemovalPlan{Kind: "remove_nodes"}
	gone := 0
	for _, id := range nodeIDs {
		if sn, ok := inSubnet[id]; ok && sn != "" {
			p.Issues = append(p.Issues, RemovalIssue{
				Target:   id,
				Detail:   fmt.Sprintf("still a member of subnet %s; execution would fail", sn),
				Blocking: true,
			})
			continue
		}
		if !registered[id] {
			gone++
			p.Issues = append(p.Issues, RemovalIssue{
				Target: id,
				Detail: "not in the registry; the canister will skip it",
			})
		}
	}
	p.NoOp = gone == len(nodeIDs) && len(nodeIDs) > 0
	return p
}

// planRemoveNodeOperators checks the precondition do_remove_node_operators
// enforces by silently filtering: an operator that still owns nodes is dropped
// from the payload, so the proposal executes and the operator remains. That is
// worse than an error, since nothing reports it.
func planRemoveNodeOperators(operatorIDs []string, ownedNodes map[string][]string, known map[string]bool) RemovalPlan {
	p := RemovalPlan{Kind: "remove_node_operators"}
	gone := 0
	for _, id := range operatorIDs {
		if n := ownedNodes[id]; len(n) > 0 {
			p.Issues = append(p.Issues, RemovalIssue{
				Target:   id,
				Detail:   fmt.Sprintf("still owns %d node(s); the canister would silently skip it", len(n)),
				Blocking: true,
			})
			continue
		}
		if !known[id] {
			gone++
			p.Issues = append(p.Issues, RemovalIssue{
				Target: id,
				Detail: "no operator record; the canister will skip it",
			})
		}
	}
	p.NoOp = gone == len(operatorIDs) && len(operatorIDs) > 0
	return p
}

// planRemoveNodeProvider checks what governance validates before the vote: a
// provider that is not registered is rejected outright. Operators still
// referencing the provider are reported too, since removing it orphans them.
func planRemoveNodeProvider(providerID string, registered bool, operators []string) RemovalPlan {
	p := RemovalPlan{Kind: "remove_node_provider"}
	if !registered {
		p.Issues = append(p.Issues, RemovalIssue{
			Target:   providerID,
			Detail:   "not a registered node provider; governance would reject the proposal",
			Blocking: true,
		})
		return p
	}
	if len(operators) > 0 {
		sorted := append([]string(nil), operators...)
		sort.Strings(sorted)
		p.Issues = append(p.Issues, RemovalIssue{
			Target:   providerID,
			Detail:   fmt.Sprintf("still has %d operator(s) (%s); remove them first", len(sorted), strings.Join(sorted, ", ")),
			Blocking: true,
		})
	}
	return p
}

func (p RemovalPlan) HasBlocking() bool {
	for _, i := range p.Issues {
		if i.Blocking {
			return true
		}
	}
	return false
}

func (p RemovalPlan) Render() string {
	var b strings.Builder
	for _, i := range p.Issues {
		marker := "note"
		if i.Blocking {
			marker = "BLOCKS"
		}
		fmt.Fprintf(&b, "  %-6s %s  %s\n", marker, i.Target, i.Detail)
	}
	return b.String()
}

// removalPreflight grades a plan the way membershipPreflight does: a blocking
// precondition is a warning the operator has to see, an all-absent target set
// is a no-op, and anything else is clean.
func removalPreflight(p RemovalPlan) Preflight {
	if len(p.Issues) == 0 {
		return Preflight{}
	}
	level := PreflightWarn
	if !p.HasBlocking() && p.NoOp {
		level = PreflightNoOp
	}
	return Preflight{p.Render(), level}
}
