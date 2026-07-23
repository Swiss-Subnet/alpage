package nns

import (
	"fmt"
	"sort"
	"strings"
)

// ReconcileStatus classifies one node relative to a subnet's live on-chain
// membership and what resources.hcl declares.
type ReconcileStatus string

const (
	// ReconcileInSync: declared on this subnet and a live member.
	ReconcileInSync ReconcileStatus = "in-sync"
	// ReconcileMissing: declared on this subnet but not a live member.
	ReconcileMissing ReconcileStatus = "missing"
	// ReconcileUnmanaged: a live member with no matching node block declaring
	// this subnet.
	ReconcileUnmanaged ReconcileStatus = "unmanaged"
	// ReconcileDeregistered: declared on this subnet, not a live member, and no
	// longer present in the registry at all (its node record was deleted).
	// Only distinguished from missing when node status is supplied.
	ReconcileDeregistered ReconcileStatus = "deregistered"
)

type ReconcileRow struct {
	NodeID string
	Name   string // resource block name, if declared
	Label  string
	Status ReconcileStatus
}

// SubnetReconcile is the diff of one subnet's declared membership against its
// live on-chain membership.
type SubnetReconcile struct {
	SubnetID    string
	SubnetLabel string
	Rows        []ReconcileRow
}

// Reconcile diffs resources.hcl against a subnet's live membership. It considers
// only node resources that declare this subnet via their `subnet` field; a
// live member with no such declaration is reported as unmanaged. Pure: the
// caller fetches liveMembers and passes them in.
//
// nodeStatus, when non-nil, maps a declared node id to its registry
// registration state; a declared non-member that is not registered is reported
// as deregistered rather than merely missing. Pass nil to skip that refinement.
func Reconcile(r *Resources, subnetID string, liveMembers []string, nodeStatus map[string]NodeStatus) SubnetReconcile {
	live := make(map[string]bool, len(liveMembers))
	for _, id := range liveMembers {
		live[id] = true
	}
	declared := map[string]Resource{}
	for _, n := range r.Nodes {
		if n.Subnet == subnetID {
			declared[n.ID] = n
		}
	}
	rc := SubnetReconcile{SubnetID: subnetID, SubnetLabel: r.LabelFor(subnetID)}
	for id, n := range declared {
		st := ReconcileMissing
		switch {
		case live[id]:
			st = ReconcileInSync
		case nodeStatus != nil:
			if s, ok := nodeStatus[id]; ok && !s.Registered {
				st = ReconcileDeregistered
			}
		}
		rc.Rows = append(rc.Rows, ReconcileRow{NodeID: id, Name: n.Name, Label: n.Label, Status: st})
	}
	for _, id := range liveMembers {
		if _, ok := declared[id]; !ok {
			rc.Rows = append(rc.Rows, ReconcileRow{NodeID: id, Label: r.LabelFor(id), Status: ReconcileUnmanaged})
		}
	}
	sort.Slice(rc.Rows, func(i, j int) bool { return rc.Rows[i].NodeID < rc.Rows[j].NodeID })
	return rc
}

// HasDrift reports whether any row is not in sync.
func (rc SubnetReconcile) HasDrift() bool {
	for _, row := range rc.Rows {
		if row.Status != ReconcileInSync {
			return true
		}
	}
	return false
}

// Render writes a human-readable report for one subnet.
func (rc SubnetReconcile) Render(b *strings.Builder) {
	head := rc.SubnetID
	if rc.SubnetLabel != "" {
		head = fmt.Sprintf("%s (%s)", rc.SubnetLabel, rc.SubnetID)
	}
	fmt.Fprintf(b, "subnet %s\n", head)
	if len(rc.Rows) == 0 {
		b.WriteString("  (no declared or live members)\n")
		return
	}
	statusW, nameW := 0, 0
	for _, row := range rc.Rows {
		if n := len(row.Status); n > statusW {
			statusW = n
		}
		if n := len(rowName(row)); n > nameW {
			nameW = n
		}
	}
	for _, row := range rc.Rows {
		label := row.Label
		if label != "" {
			label = "  " + label
		}
		status := colorize(fmt.Sprintf("%-*s", statusW, row.Status), row.Status.color())
		fmt.Fprintf(b, "  %s  %-*s  %s%s\n", status, nameW, rowName(row), row.NodeID, label)
	}
}

func (s ReconcileStatus) color() string {
	switch s {
	case ReconcileInSync:
		return ansiGreen
	case ReconcileUnmanaged:
		return ansiYellow
	default:
		return ansiRed
	}
}

func rowName(row ReconcileRow) string {
	if row.Name == "" {
		return "-"
	}
	return row.Name
}
