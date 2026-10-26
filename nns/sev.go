package nns

import (
	"fmt"
	"strings"

	"github.com/swiss-subnet/alpage/gen/registry"
)

// A sev_enabled subnet is held to two registry invariants that are checked
// after adoption: every member node record carries a chip_id, and the subnet's
// replica version record carries guest launch measurements. Both trap on
// execution, so membership and deploy_guestos report them before submission.

// sevEnabled reports whether a subnet record has SEV on. An absent features
// record, or an absent field in it, reads as off, as it does in the registry's
// own conversion.
func sevEnabled(rec *registry.SubnetRecord) bool {
	return rec.Features != nil && rec.Features.SevEnabled != nil && *rec.Features.SevEnabled
}

// sevAdmission classifies the nodes a membership proposal adds against the
// chip_id invariant. Undecided entries carry their own reason.
type sevAdmission struct {
	Chipless  []string
	Undecided []string
}

// checkSevAdmission reads each incoming node's registry record through the
// explorer, so a record it cannot read leaves that node undecided. A node with
// no record at all is undecided too, since the membership change already fails
// on the missing node before the invariant runs.
func checkSevAdmission(explorerBase string, nodeIDs []string) sevAdmission {
	var a sevAdmission
	for _, id := range nodeIDs {
		st, err := FetchNodeStatus(explorerBase, id)
		switch {
		case err != nil:
			a.Undecided = append(a.Undecided, fmt.Sprintf("%s (%v)", id, err))
		case !st.Registered:
			a.Undecided = append(a.Undecided, id+" (no registry record)")
		case st.ChipID == "":
			a.Chipless = append(a.Chipless, id)
		}
	}
	return a
}

// fold renders the verdict under a membership preflight. A stronger level is
// left alone, since a proposal that changes nothing is more actionable.
func (a sevAdmission) fold(pf Preflight) Preflight {
	if len(a.Chipless) == 0 && len(a.Undecided) == 0 {
		return pf
	}
	var b strings.Builder
	b.WriteString(pf.Report)
	for _, id := range a.Chipless {
		fmt.Fprintf(&b, "  BLOCKS %s carries no chip_id on a SEV-enabled subnet, so execution traps on the invariant\n", id)
	}
	for _, id := range a.Undecided {
		fmt.Fprintf(&b, "  WARNING: chip_id of %s unread, so the SEV invariant could not be checked\n", id)
	}
	level := pf.Level
	if level == PreflightClean {
		level = PreflightWarn
	}
	return Preflight{b.String(), level}
}

// sevLaunchGate renders the launch-measurement invariant under a deploy_guestos
// preflight. has and known come from ElectedVersions.Measured. A NoOp preflight
// is left alone: an unelected or already-running version is the headline.
func sevLaunchGate(pf Preflight, target string, has, known bool) Preflight {
	if pf.Level == PreflightNoOp {
		return pf
	}
	var b strings.Builder
	b.WriteString(pf.Report)
	level := pf.Level
	switch {
	case !known:
		fmt.Fprintf(&b, "  WARNING: guest launch measurements of %s unread on a SEV-enabled subnet\n", target)
		level = PreflightWarn
	case !has:
		fmt.Fprintf(&b, "  BLOCKS %s carries no guest launch measurements on a SEV-enabled subnet, so execution traps on the invariant\n", target)
		level = PreflightWarn
	default:
		b.WriteString("  target carries guest launch measurements, as the SEV-enabled subnet requires\n")
	}
	return Preflight{b.String(), level}
}
