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
	declared := map[string]NodeRes{}
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

// Table renders the subnet's rows. Label doubles as the detail column, so a
// declared node's human name trails its id.
func (rc SubnetReconcile) Table() Table {
	head := rc.SubnetID
	if rc.SubnetLabel != "" {
		head = fmt.Sprintf("%s (%s)", rc.SubnetLabel, rc.SubnetID)
	}
	t := Table{
		Header: "subnet " + head,
		Empty:  "  (no declared or live members)\n",
	}
	for _, row := range rc.Rows {
		detail := row.Label
		if detail != "" {
			detail = "  " + detail
		}
		t.Rows = append(t.Rows, Row{
			Status: string(row.Status),
			Name:   row.Name,
			ID:     row.NodeID,
			Detail: detail,
			Remedy: remedyOf(row.Status),
			OK:     row.Status == ReconcileInSync,
			Color:  row.Status.color(),
		})
	}
	return t
}

// remedy names what resolves each status. Only proposal kinds alpage actually
// implements are named, so a remedy never points at a command that does not
// exist.
func (s ReconcileStatus) remedy() string {
	switch s {
	case ReconcileMissing:
		return "add it with a membership proposal, or drop the subnet field if it was never meant to join"
	case ReconcileDeregistered:
		return "the node record is gone, so mark the node decommissioned or register it again before adding it back"
	case ReconcileUnmanaged:
		return "declare a node block for it, or take it out with a membership proposal"
	default:
		return ""
	}
}

func (rc SubnetReconcile) HasDrift() bool { return rc.Table().HasDrift() }

func (rc SubnetReconcile) Render(b *strings.Builder) { rc.Table().Render(b) }

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
