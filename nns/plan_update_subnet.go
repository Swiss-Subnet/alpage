package nns

import (
	"fmt"
	"strings"
)

// SubnetSettingChange is one declared setting, diffed against the subnet's
// current record.
type SubnetSettingChange struct {
	Field string
	From  string
	To    string
	// NoOp marks a setting already at its declared value: harmless on its own,
	// but a proposal made only of these changes nothing.
	NoOp bool
	// Blocking marks a change the registry refuses, which it does by trapping
	// after the NNS has adopted the proposal.
	Blocking bool
	// Note explains a change that is legal but easy to make by accident.
	Note string
}

// UpdateSubnetPlan is the preflight verdict for an update_subnet proposal.
type UpdateSubnetPlan struct {
	SubnetID string
	Changes  []SubnetSettingChange
}

// planUpdateSubnet diffs the declared settings against current. Only declared
// fields appear: an absent field is not a change to "false" or "0", the registry
// skips it entirely, so reporting it would invent a change. current is a fully
// populated SubnetSettings (see currentSettings), which lets one comparison
// serve both sides. Pure.
func planUpdateSubnet(subnetID string, want, current SubnetSettings) UpdateSubnetPlan {
	p := UpdateSubnetPlan{SubnetID: subnetID}

	diffScalar(&p, "is_halted", want.IsHalted, current.IsHalted, func(v bool) string { return fmt.Sprintf("%t", v) })
	diffScalar(&p, "halt_at_cup_height", want.HaltAtCupHeight, current.HaltAtCupHeight, func(v bool) string { return fmt.Sprintf("%t", v) })
	diffScalar(&p, "max_number_of_canisters", want.MaxNumberOfCanisters, current.MaxNumberOfCanisters, formatUint)

	diffList(&p, "ssh_readonly_access", want.SSHReadonlyAccess, current.SSHReadonlyAccess)
	diffList(&p, "ssh_backup_access", want.SSHBackupAccess, current.SSHBackupAccess)
	if want.SubnetAdmins != nil {
		var cur []string
		if current.SubnetAdmins != nil {
			cur = encodePrincipals(*current.SubnetAdmins)
		}
		admins := encodePrincipals(*want.SubnetAdmins)
		diffList(&p, "subnet_admins", &admins, &cur)
	}

	diffFeatures(&p, want.Features, current.Features)
	diffScalar(&p, "resource_limits", want.ResourceLimits, current.ResourceLimits, SubnetResourceLimits.String)

	diffScalar(&p, "unit_delay_millis", want.UnitDelayMillis, current.UnitDelayMillis, formatUint)
	diffScalar(&p, "initial_notary_delay_millis", want.InitialNotaryDelayMillis, current.InitialNotaryDelayMillis, formatUint)
	diffScalar(&p, "dkg_interval_length", want.DkgIntervalLength, current.DkgIntervalLength, formatUint)
	diffScalar(&p, "dkg_dealings_per_block", want.DkgDealingsPerBlock, current.DkgDealingsPerBlock, formatUint)
	diffScalar(&p, "max_ingress_bytes_per_message", want.MaxIngressBytesPerMessage, current.MaxIngressBytesPerMessage, formatUint)
	diffScalar(&p, "max_ingress_bytes_per_block", want.MaxIngressBytesPerBlock, current.MaxIngressBytesPerBlock, formatUint)
	diffScalar(&p, "max_ingress_messages_per_block", want.MaxIngressMessagesPerBlock, current.MaxIngressMessagesPerBlock, formatUint)
	diffScalar(&p, "max_block_payload_size", want.MaxBlockPayloadSize, current.MaxBlockPayloadSize, formatUint)

	return p
}

func formatUint(v uint64) string { return fmt.Sprintf("%d", v) }

// diffScalar records a declared setting. A nil current side means the record did
// not carry the field, which is reported as unknown rather than assumed equal:
// claiming a no-op there could hide a real change.
func diffScalar[T comparable](p *UpdateSubnetPlan, field string, want, current *T, render func(T) string) {
	if want == nil {
		return
	}
	if current == nil {
		p.Changes = append(p.Changes, SubnetSettingChange{
			Field: field, From: "unknown", To: render(*want),
			Note: "current value not in the subnet record",
		})
		return
	}
	p.Changes = append(p.Changes, SubnetSettingChange{
		Field: field, From: render(*current), To: render(*want),
		NoOp: *want == *current,
	})
}

// diffList diffs a wholesale-replaced list. Clearing a non-empty list is legal
// and is how SSH access is revoked, but it is also what an incomplete restatement
// looks like, so it carries a note.
func diffList(p *UpdateSubnetPlan, field string, want, current *[]string) {
	if want == nil {
		return
	}
	var cur []string
	if current != nil {
		cur = *current
	}
	c := SubnetSettingChange{
		Field: field, From: renderList(cur), To: renderList(*want),
		NoOp: equalStrings(*want, cur),
	}
	if len(*want) == 0 && len(cur) > 0 {
		c.Note = fmt.Sprintf("clears %d existing entr%s; the list is replaced, not merged", len(cur), plural(len(cur)))
	}
	p.Changes = append(p.Changes, c)
}

// diffFeatures diffs the features record. Only turning sev_enabled off blocks:
// do_update_subnet's validate_sev_is_not_disabled panics on that, after the
// proposal is adopted. Turning it on is allowed, but the SEV subnet invariants
// then require every node to carry a chip ID and the subnet's GuestOS version
// to have launch measurements, and those also trap post-adoption, so enabling
// is reported as the live change it is rather than waved through.
func diffFeatures(p *UpdateSubnetPlan, want, current *SubnetFeatureSet) {
	if want == nil {
		return
	}
	cur := SubnetFeatureSet{}
	if current != nil {
		cur = *current
	}
	c := SubnetSettingChange{
		Field: "features", From: cur.String(), To: want.String(),
		NoOp: *want == cur,
	}
	switch {
	case cur.SevEnabled && !want.SevEnabled:
		c.Blocking = true
		c.Note = "sev_enabled true -> false: the registry refuses to disable SEV. Note that features is replaced wholesale, so omitting sev_enabled reads as disabling it"
	case !cur.SevEnabled && want.SevEnabled:
		c.Note = "sev_enabled false -> true: allowed, but execution traps unless every node already carries a chip_id and the subnet's GuestOS version has launch measurements"
	}
	p.Changes = append(p.Changes, c)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func (p UpdateSubnetPlan) HasBlocking() bool {
	for _, c := range p.Changes {
		if c.Blocking {
			return true
		}
	}
	return false
}

// AllNoOp reports whether every declared setting already holds on-chain, so the
// proposal would change nothing.
func (p UpdateSubnetPlan) AllNoOp() bool {
	if len(p.Changes) == 0 {
		return false
	}
	for _, c := range p.Changes {
		if !c.NoOp {
			return false
		}
	}
	return true
}

func (p UpdateSubnetPlan) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "subnet %s (%d setting(s) declared)\n", p.SubnetID, len(p.Changes))
	for _, c := range p.Changes {
		marker := "~"
		switch {
		case c.Blocking:
			marker = "BLOCKS"
		case c.NoOp:
			marker = "="
		}
		fmt.Fprintf(&b, "  %-6s %s: %s -> %s\n", marker, c.Field, c.From, c.To)
		if c.Note != "" {
			fmt.Fprintf(&b, "         %s\n", c.Note)
		}
	}
	changes := 0
	for _, c := range p.Changes {
		if !c.NoOp {
			changes++
		}
	}
	fmt.Fprintf(&b, "%d change(s), %d already at the declared value\n", changes, len(p.Changes)-changes)
	return b.String()
}

// updateSubnetPreflight grades the plan the way the other kinds do: a change the
// registry refuses is a warning the operator has to see (the dry-run then fails
// hard on it), an all-no-op proposal is refused without --force, and anything
// else is clean. Every level renders the full diff.
func updateSubnetPreflight(p UpdateSubnetPlan) Preflight {
	switch {
	case p.HasBlocking():
		return Preflight{p.Render(), PreflightWarn}
	case p.AllNoOp():
		return Preflight{p.Render(), PreflightNoOp}
	}
	for _, c := range p.Changes {
		if c.Note != "" {
			return Preflight{p.Render(), PreflightWarn}
		}
	}
	return Preflight{p.Render(), PreflightClean}
}
