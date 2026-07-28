package nns

import (
	"fmt"
	"sort"
	"strings"
)

// NodeVersionStatus classifies one node's declared guestos_version against the
// version the node itself reports running.
type NodeVersionStatus string

const (
	// NodeVersionOK: the node reports the declared version.
	NodeVersionOK NodeVersionStatus = "ok"
	// NodeVersionMismatch: the node reports a different version than declared.
	NodeVersionMismatch NodeVersionStatus = "version-mismatch"
	// NodeVersionUnreachable: the node's status endpoint could not be read, so
	// its version is unknown. Not drift: being down is a different problem from
	// running the wrong version, and reconcile should not fail on it.
	NodeVersionUnreachable NodeVersionStatus = "unreachable"
)

// NodeVersion is what one node reported. Err set means the read failed and
// Version is meaningless. Indirect marks a version that came from the dashboard
// rather than the node itself, so it may lag reality.
type NodeVersion struct {
	Version  string
	Err      string
	Indirect bool
}

type NodeVersionRow struct {
	NodeID   string
	Name     string
	Declared string
	Actual   string
	Err      string
	Status   NodeVersionStatus
	Indirect bool
}

// NodeVersionReconcile is the diff of declared node versions against what the
// nodes report running.
type NodeVersionReconcile struct {
	Nodes []NodeVersionRow
}

// ReconcileNodeVersions diffs each node's declared guestos_version against the
// version it reports. reported maps node id to what FetchNodeVersion got for it;
// the caller does the fetching. Pure.
//
// Only nodes declaring a version are checked, and decommissioned nodes are
// skipped: they are expected to be gone, so an unreachable status endpoint is
// the correct outcome rather than something to report.
func ReconcileNodeVersions(r *Resources, reported map[string]NodeVersion) NodeVersionReconcile {
	var vr NodeVersionReconcile
	for _, n := range r.Nodes {
		if n.GuestosVersion == "" || n.Decommissioned {
			continue
		}
		got := reported[n.ID]
		row := NodeVersionRow{
			NodeID: n.ID, Name: n.Name,
			Declared: n.GuestosVersion, Actual: got.Version, Err: got.Err,
			Indirect: got.Indirect && got.Version != "",
		}
		switch {
		case got.Err != "" || got.Version == "":
			row.Status = NodeVersionUnreachable
		case got.Version == n.GuestosVersion:
			row.Status = NodeVersionOK
		default:
			row.Status = NodeVersionMismatch
		}
		vr.Nodes = append(vr.Nodes, row)
	}
	sort.Slice(vr.Nodes, func(i, j int) bool { return vr.Nodes[i].NodeID < vr.Nodes[j].NodeID })
	return vr
}

func (vr NodeVersionReconcile) HasDrift() bool {
	for _, row := range vr.Nodes {
		if row.Status == NodeVersionMismatch {
			return true
		}
	}
	return false
}

func (vr NodeVersionReconcile) Render(b *strings.Builder) {
	if len(vr.Nodes) == 0 {
		return
	}
	b.WriteString("node versions\n")
	nameW := 0
	for _, row := range vr.Nodes {
		if n := len(versionRowName(row)); n > nameW {
			nameW = n
		}
	}
	indirect := false
	for _, row := range vr.Nodes {
		detail := ""
		switch row.Status {
		case NodeVersionOK:
			detail = fmt.Sprintf("  (%s)", shortVersion(row.Declared))
		case NodeVersionMismatch:
			detail = fmt.Sprintf("  (declared %s, running %s)", shortVersion(row.Declared), shortVersion(row.Actual))
		case NodeVersionUnreachable:
			detail = fmt.Sprintf("  (declared %s, status unread: %s)", shortVersion(row.Declared), row.Err)
		}
		if row.Indirect {
			indirect = true
			detail += " via dashboard"
		}
		status := colorize(fmt.Sprintf("%-17s", row.Status), row.Status.color())
		fmt.Fprintf(b, "  %s  %-*s  %s%s\n", status, nameW, versionRowName(row), row.NodeID, detail)
	}
	if indirect {
		b.WriteString(colorize("  note: versions marked \"via dashboard\" were read from the public dashboard\n"+
			"        because the node was unreachable; that data may lag the node's real state.\n", ansiYellow))
	}
}

func (s NodeVersionStatus) color() string {
	switch s {
	case NodeVersionOK:
		return ansiGreen
	case NodeVersionUnreachable:
		return ansiYellow
	default:
		return ansiRed
	}
}

// shortVersion abbreviates a git-revision version for display; other version
// strings are left alone.
func shortVersion(v string) string {
	if len(v) == 40 && strings.Trim(v, "0123456789abcdef") == "" {
		return v[:12]
	}
	if v == "" {
		return "-"
	}
	return v
}

func versionRowName(row NodeVersionRow) string {
	if row.Name == "" {
		return "-"
	}
	return row.Name
}
