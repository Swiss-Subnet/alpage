package nns

import (
	"fmt"
	"sort"
	"strings"
)

// NodeOwnershipStatus classifies one node relative to which operator the
// registry says owns it and what resources.hcl declares.
type NodeOwnershipStatus string

const (
	// NodeOwnershipOK: declared under operator X and on-chain owned by X.
	NodeOwnershipOK NodeOwnershipStatus = "ok"
	// NodeOwnershipMismatch: declared under operator X but on-chain owned by a
	// different declared operator.
	NodeOwnershipMismatch NodeOwnershipStatus = "operator-mismatch"
	// NodeOwnershipGone: declared under an operator but no declared operator
	// owns it on-chain (moved to an operator we don't track, or deregistered).
	NodeOwnershipGone NodeOwnershipStatus = "gone"
	// NodeOwnershipUnmanaged: on-chain owned by a declared operator but no node
	// block ties it to that operator.
	NodeOwnershipUnmanaged NodeOwnershipStatus = "unmanaged"
	// NodeOwnershipDecommissioned: declared decommissioned and absent from the
	// registry, as expected. Not drift.
	NodeOwnershipDecommissioned NodeOwnershipStatus = "decommissioned"
)

type NodeOwnershipRow struct {
	NodeID         string
	Name           string
	Operator       string // declared operator id, if any
	Owner          string // on-chain owning operator id, if any
	Decommissioned bool   // declared decommissioned
	Status         NodeOwnershipStatus
}

// NodeOwnershipReconcile is the diff of declared node->operator links against
// the registry's operator ownership.
type NodeOwnershipReconcile struct {
	Nodes []NodeOwnershipRow
}

// ReconcileOperatorNodes checks node ownership both directions against the
// registry. byOperator maps a declared operator id to the node ids the registry
// says it owns; the caller fetches it (one FetchOperatorNodes per declared
// operator). Pure.
//
// Forward: each node that declares an operator is matched to its on-chain owner
// among the declared operators. Reverse: any on-chain-owned node with no node
// block tying it to that operator is reported as unmanaged, so operator-owned
// nodes missing from resources.hcl surface as drift rather than being invisible.
func ReconcileOperatorNodes(r *Resources, byOperator map[string][]string) NodeOwnershipReconcile {
	owner := map[string]string{} // node id -> declared operator that owns it on-chain
	for opID, nodes := range byOperator {
		for _, n := range nodes {
			owner[n] = opID
		}
	}
	declared := map[string]NodeRes{} // node id -> block, only those with an operator
	for _, n := range r.Nodes {
		if n.Operator != "" {
			declared[n.ID] = n
		}
	}

	var nr NodeOwnershipReconcile
	for _, n := range r.Nodes {
		if n.Operator == "" {
			continue
		}
		row := NodeOwnershipRow{
			NodeID: n.ID, Name: n.Name, Operator: n.Operator,
			Owner: owner[n.ID], Decommissioned: n.Decommissioned,
		}
		switch {
		case n.Decommissioned && owner[n.ID] == "":
			row.Status = NodeOwnershipDecommissioned
		case n.Decommissioned:
			// Declared gone but the registry still records an owner.
			row.Status = NodeOwnershipMismatch
		case owner[n.ID] == "":
			row.Status = NodeOwnershipGone
		case owner[n.ID] == n.Operator:
			row.Status = NodeOwnershipOK
		default:
			row.Status = NodeOwnershipMismatch
		}
		nr.Nodes = append(nr.Nodes, row)
	}
	for nodeID, opID := range owner {
		if _, ok := declared[nodeID]; ok {
			continue
		}
		nr.Nodes = append(nr.Nodes, NodeOwnershipRow{
			NodeID: nodeID,
			Owner:  opID,
			Status: NodeOwnershipUnmanaged,
		})
	}
	sort.Slice(nr.Nodes, func(i, j int) bool { return nr.Nodes[i].NodeID < nr.Nodes[j].NodeID })
	return nr
}

func (nr NodeOwnershipReconcile) HasDrift() bool {
	for _, row := range nr.Nodes {
		if row.Status != NodeOwnershipOK && row.Status != NodeOwnershipDecommissioned {
			return true
		}
	}
	return false
}

func (nr NodeOwnershipReconcile) Render(b *strings.Builder) {
	if len(nr.Nodes) == 0 {
		return
	}
	b.WriteString("nodes\n")
	nameW := 0
	for _, row := range nr.Nodes {
		if n := len(nodeRowName(row)); n > nameW {
			nameW = n
		}
	}
	for _, row := range nr.Nodes {
		detail := ""
		switch row.Status {
		case NodeOwnershipMismatch:
			if row.Owner != "" && row.Decommissioned {
				detail = fmt.Sprintf("  (declared decommissioned but still owned by %s)", row.Owner)
				break
			}
			detail = fmt.Sprintf("  (declared operator %s, on-chain %s)", row.Operator, row.Owner)
		case NodeOwnershipGone:
			detail = fmt.Sprintf("  (declared operator %s owns it no longer)", row.Operator)
		case NodeOwnershipUnmanaged:
			detail = fmt.Sprintf("  (owned by declared operator %s, not in resources.hcl)", row.Owner)
		}
		status := colorize(fmt.Sprintf("%-17s", row.Status), row.Status.color())
		fmt.Fprintf(b, "  %s  %-*s  %s%s\n", status, nameW, nodeRowName(row), row.NodeID, detail)
	}
}

func (s NodeOwnershipStatus) color() string {
	switch s {
	case NodeOwnershipOK, NodeOwnershipDecommissioned:
		return ansiGreen
	case NodeOwnershipUnmanaged:
		return ansiYellow
	default:
		return ansiRed
	}
}

func nodeRowName(row NodeOwnershipRow) string {
	if row.Name == "" {
		return "-"
	}
	return row.Name
}
