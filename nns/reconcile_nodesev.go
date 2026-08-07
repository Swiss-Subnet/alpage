package nns

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// chipIDBytes is the size of an AMD SEV-SNP CHIP_ID (ATTESTATION_REPORT offset
// 0x1A0).
const chipIDBytes = 64

// normalizeChipID renders a chip id as lowercase hex, the form AMD's KDS takes
// in its HWID URL parameter. Accepts hex or the base64 the registry explorer
// serves, so a value pasted from either source compares equal.
func normalizeChipID(chip string) (string, error) {
	chip = strings.TrimSpace(chip)
	if chip == "" {
		return "", nil
	}
	if b, err := hex.DecodeString(chip); err == nil {
		if len(b) != chipIDBytes {
			return "", fmt.Errorf("chip id is %d bytes, want %d", len(b), chipIDBytes)
		}
		return hex.EncodeToString(b), nil
	}
	b, err := base64.StdEncoding.DecodeString(chip)
	if err != nil {
		return "", fmt.Errorf("chip id is neither hex nor base64")
	}
	if len(b) != chipIDBytes {
		return "", fmt.Errorf("chip id is %d bytes, want %d", len(b), chipIDBytes)
	}
	return hex.EncodeToString(b), nil
}

// NodeSevStatus classifies one node's declared chip_id against its registry
// record.
type NodeSevStatus string

const (
	NodeSevInSync     NodeSevStatus = "in-sync"
	NodeSevMismatch   NodeSevStatus = "chip-changed"
	NodeSevMissing    NodeSevStatus = "chip-missing"
	NodeSevUndeclared NodeSevStatus = "chip-undeclared"
	// NodeSevMalformed: the declared chip_id is not a 64-byte hex or base64
	// value, so there is nothing meaningful to compare it against.
	NodeSevMalformed NodeSevStatus = "malformed"
	// NodeSevUnverified: the chip matches what the registry records, but AMD
	// will not vouch for it. Config and registry can agree on a chip that is
	// not genuine, so this is drift on its own.
	NodeSevUnverified NodeSevStatus = "amd-unverified"
	// NodeSevUnknown: no record was read, so there is nothing to compare. Not
	// drift.
	NodeSevUnknown NodeSevStatus = "unknown"
)

type NodeSevRow struct {
	NodeID   string
	Name     string
	Declared string
	Live     string
	Status   NodeSevStatus
	Err      string
	// AMD is the KDS verdict for the live chip, when the caller ran one.
	AMD *ChipVerification
}

type NodeSevReconcile struct {
	Nodes []NodeSevRow
}

// ReconcileNodeSev diffs each declared node's chip_id against the one its
// registry record carries. status maps a node id to the record the caller
// fetched; a node absent from it, or one the registry no longer registers, is
// reported unknown rather than drift. Decommissioned nodes are skipped: they
// are expected to have no record at all. Pure.
func ReconcileNodeSev(r *Resources, status map[string]NodeStatus) NodeSevReconcile {
	var sr NodeSevReconcile
	for _, n := range r.Nodes {
		if n.Decommissioned {
			continue
		}
		row := NodeSevRow{NodeID: n.ID, Name: n.Name}
		declared, err := normalizeChipID(n.ChipID)
		if err != nil {
			row.Declared, row.Status, row.Err = n.ChipID, NodeSevMalformed, err.Error()
			sr.Nodes = append(sr.Nodes, row)
			continue
		}
		row.Declared = declared
		st, ok := status[n.ID]
		switch {
		case !ok || !st.Registered:
			row.Status = NodeSevUnknown
		default:
			// A record the registry serves in a form we cannot decode is a bug
			// worth surfacing, not a silent mismatch.
			live, err := normalizeChipID(st.ChipID)
			if err != nil {
				row.Live, row.Status, row.Err = st.ChipID, NodeSevMalformed, err.Error()
				break
			}
			row.Live = live
			switch {
			case row.Declared == row.Live:
				row.Status = NodeSevInSync
			case row.Declared == "":
				row.Status = NodeSevUndeclared
			case row.Live == "":
				row.Status = NodeSevMissing
			default:
				row.Status = NodeSevMismatch
			}
		}
		sr.Nodes = append(sr.Nodes, row)
	}
	sort.Slice(sr.Nodes, func(i, j int) bool { return sr.Nodes[i].NodeID < sr.Nodes[j].NodeID })
	return sr
}

// ApplyChipVerification folds AMD's verdict, keyed by chip id, into rows whose
// chip matched. Only an outright refusal downgrades a row: an inconclusive
// lookup (rate limit, network) is not evidence against the chip. Rows already
// reporting drift keep their verdict, which is the more actionable one.
func (sr *NodeSevReconcile) ApplyChipVerification(byChip map[string]ChipVerification) {
	for i := range sr.Nodes {
		row := &sr.Nodes[i]
		if row.Live == "" {
			continue
		}
		v, ok := byChip[row.Live]
		if !ok {
			continue
		}
		row.AMD = &v
		if row.Status == NodeSevInSync && !v.Verified && !v.Inconclusive {
			row.Status = NodeSevUnverified
		}
	}
}

func (sr NodeSevReconcile) HasDrift() bool {
	for _, row := range sr.Nodes {
		switch row.Status {
		case NodeSevMismatch, NodeSevMissing, NodeSevUndeclared, NodeSevMalformed, NodeSevUnverified:
			return true
		}
	}
	return false
}

func (sr NodeSevReconcile) Render(b *strings.Builder) {
	if len(sr.Nodes) == 0 {
		return
	}
	b.WriteString("node chip_id\n")
	nameW := 0
	for _, row := range sr.Nodes {
		if n := len(nodeSevRowName(row)); n > nameW {
			nameW = n
		}
	}
	for _, row := range sr.Nodes {
		detail := ""
		switch row.Status {
		case NodeSevMismatch:
			detail = fmt.Sprintf("  (declared %s, on-chain %s)", shortChip(row.Declared), shortChip(row.Live))
		case NodeSevMissing:
			detail = fmt.Sprintf("  (declared %s, registry carries none)", shortChip(row.Declared))
		case NodeSevUndeclared:
			detail = fmt.Sprintf("  (on-chain %s, not in resources.hcl)", shortChip(row.Live))
		case NodeSevMalformed:
			detail = fmt.Sprintf("  (%s)", row.Err)
		case NodeSevUnverified:
			detail = fmt.Sprintf("  (%s: %s)", shortChip(row.Live), row.AMD.Err)
		case NodeSevUnknown:
			detail = "  (no registry record read)"
		case NodeSevInSync:
			switch {
			case row.AMD == nil || row.Live == "":
			case row.AMD.Verified:
				detail = fmt.Sprintf("  (AMD-verified, %s)", row.AMD.Product)
			case row.AMD.Inconclusive:
				detail = fmt.Sprintf("  (%s)", row.AMD.Err)
			}
		}
		status := colorize(fmt.Sprintf("%-15s", row.Status), row.Status.color())
		fmt.Fprintf(b, "  %s  %-*s  %s%s\n", status, nameW, nodeSevRowName(row), row.NodeID, detail)
	}
}

// shortChip keeps a report readable: the full 64-byte value is opaque noise and
// the leading bytes are enough to tell two chips apart.
func shortChip(chip string) string {
	if len(chip) <= 12 {
		return chip
	}
	return chip[:12] + "..."
}

func (s NodeSevStatus) color() string {
	switch s {
	case NodeSevInSync:
		return ansiGreen
	case NodeSevUnknown:
		return ansiYellow
	default:
		return ansiRed
	}
}

func nodeSevRowName(row NodeSevRow) string {
	if row.Name == "" {
		return "-"
	}
	return row.Name
}
