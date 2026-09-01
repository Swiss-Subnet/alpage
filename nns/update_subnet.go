package nns

import (
	"fmt"
	"strings"

	"github.com/aviate-labs/agent-go/candid"
	"github.com/aviate-labs/agent-go/principal"
	"github.com/swiss-subnet/alpage/gen/governance"
	"github.com/swiss-subnet/alpage/gen/registry"
	governancepb "github.com/swiss-subnet/alpage/nns/pb/governance"
)

// --- update_subnet: update_subnet (UPDATE_CONFIG_OF_SUBNET) ---

// The registry merges this payload field by field: a top-level field the payload
// leaves unset is skipped, not reset, so every setting here is optional and an
// omitted one is untouched on-chain. A nested record is the exception. Sending
// features or resource_limits replaces the subnet's whole record, so an unset
// sub-field falls back to its default rather than to its current value. Those
// blocks therefore require all of their fields, which is also what makes the
// wholesale replacement visible in the config rather than implied by it.
//
// Two fields the payload carries are deliberately not settings here: the
// registry asserts subnet_type is unchanged (EXC-408) and rejects any change to
// sev_enabled, which can only be set when the subnet is created. sev_enabled is
// still declared, inside features, because features has to be restated in full;
// Preflight checks that it matches what the subnet already has.

// SubnetSettings is the subset of UpdateSubnetPayload alpage exposes: the fields
// the registry still acts on. A nil field is one the proposal does not touch.
// The gossip config, which the canister destructures and discards, is omitted,
// as is chain_key_config: threshold-key management rather than subnet config.
type SubnetSettings struct {
	IsHalted             *bool
	HaltAtCupHeight      *bool
	MaxNumberOfCanisters *uint64
	// Lists are replaced wholesale, so a declared empty list clears the current
	// one. That is a legitimate change (revoking SSH access) and a classic
	// accident, so the plan flags it.
	SSHReadonlyAccess *[]string
	SSHBackupAccess   *[]string
	SubnetAdmins      *[]principal.Principal
	Features          *SubnetFeatureSet
	ResourceLimits    *SubnetResourceLimits

	UnitDelayMillis            *uint64
	InitialNotaryDelayMillis   *uint64
	DkgIntervalLength          *uint64
	DkgDealingsPerBlock        *uint64
	MaxIngressBytesPerMessage  *uint64
	MaxIngressBytesPerBlock    *uint64
	MaxIngressMessagesPerBlock *uint64
	MaxBlockPayloadSize        *uint64
}

// SubnetFeatureSet is the subnet's whole features record. All three fields are
// stated because sending the record replaces it.
type SubnetFeatureSet struct {
	CanisterSandboxing bool
	HTTPRequests       bool
	SevEnabled         bool
}

// SubnetResourceLimits is the subnet's whole resource_limits record.
type SubnetResourceLimits struct {
	MaximumStateSize  uint64
	MaximumStateDelta uint64
}

// UpdateSubnetAction changes a subnet's configuration record: halting it for a
// recovery, granting SSH access, capping its canister count, or restating its
// features. Every field is optional, so one proposal changes exactly what it
// declares.
type UpdateSubnetAction struct {
	Meta
	SubnetID principal.Principal
	Settings SubnetSettings
}

// settings converts the decoded HCL body, decoding the admin principals and
// refusing a block that declares nothing: an update_subnet payload with no
// settings encodes and executes fine, and changes nothing, so the config is the
// place to catch it.
func (b updateSubnetBody) settings() (SubnetSettings, error) {
	s := SubnetSettings{
		IsHalted:             b.IsHalted,
		HaltAtCupHeight:      b.HaltAtCupHeight,
		MaxNumberOfCanisters: b.MaxNumberOfCanisters,
		SSHReadonlyAccess:    b.SSHReadonlyAccess,
		SSHBackupAccess:      b.SSHBackupAccess,

		UnitDelayMillis:            b.UnitDelayMillis,
		InitialNotaryDelayMillis:   b.InitialNotaryDelayMillis,
		DkgIntervalLength:          b.DkgIntervalLength,
		DkgDealingsPerBlock:        b.DkgDealingsPerBlock,
		MaxIngressBytesPerMessage:  b.MaxIngressBytesPerMessage,
		MaxIngressBytesPerBlock:    b.MaxIngressBytesPerBlock,
		MaxIngressMessagesPerBlock: b.MaxIngressMessagesPerBlock,
		MaxBlockPayloadSize:        b.MaxBlockPayloadSize,
	}
	if b.SubnetAdmins != nil {
		admins := make([]principal.Principal, len(*b.SubnetAdmins))
		for i, a := range *b.SubnetAdmins {
			p, err := principal.Decode(a)
			if err != nil {
				return SubnetSettings{}, fmt.Errorf("subnet_admins: %q: %w", a, err)
			}
			admins[i] = p
		}
		s.SubnetAdmins = &admins
	}
	if f := b.Features; f != nil {
		s.Features = &SubnetFeatureSet{
			CanisterSandboxing: f.CanisterSandboxing,
			HTTPRequests:       f.HTTPRequests,
			SevEnabled:         f.SevEnabled,
		}
	}
	if rl := b.ResourceLimits; rl != nil {
		s.ResourceLimits = &SubnetResourceLimits{
			MaximumStateSize:  rl.MaximumStateSize,
			MaximumStateDelta: rl.MaximumStateDelta,
		}
	}
	if s.Empty() {
		return SubnetSettings{}, fmt.Errorf("update_subnet declares no settings; it must change at least one field")
	}
	return s, nil
}

// Empty reports whether nothing at all is declared.
func (s SubnetSettings) Empty() bool {
	return s.IsHalted == nil && s.HaltAtCupHeight == nil && s.MaxNumberOfCanisters == nil &&
		s.SSHReadonlyAccess == nil && s.SSHBackupAccess == nil && s.SubnetAdmins == nil &&
		s.Features == nil && s.ResourceLimits == nil &&
		s.UnitDelayMillis == nil && s.InitialNotaryDelayMillis == nil &&
		s.DkgIntervalLength == nil && s.DkgDealingsPerBlock == nil &&
		s.MaxIngressBytesPerMessage == nil && s.MaxIngressBytesPerBlock == nil &&
		s.MaxIngressMessagesPerBlock == nil && s.MaxBlockPayloadSize == nil
}

func (u UpdateSubnetAction) Kind() string { return "update_subnet" }
func (u UpdateSubnetAction) NnsFunction() governancepb.NnsFunction {
	return governancepb.NnsFunction_NNS_FUNCTION_UPDATE_CONFIG_OF_SUBNET
}

func (u UpdateSubnetAction) PayloadBlob() ([]byte, error) {
	s := u.Settings
	payload := registry.UpdateSubnetPayload{
		SubnetId:             u.SubnetID,
		IsHalted:             s.IsHalted,
		HaltAtCupHeight:      s.HaltAtCupHeight,
		MaxNumberOfCanisters: s.MaxNumberOfCanisters,
		SshReadonlyAccess:    s.SSHReadonlyAccess,
		SshBackupAccess:      s.SSHBackupAccess,
		SubnetAdmins:         s.SubnetAdmins,

		UnitDelayMillis:            s.UnitDelayMillis,
		InitialNotaryDelayMillis:   s.InitialNotaryDelayMillis,
		DkgIntervalLength:          s.DkgIntervalLength,
		DkgDealingsPerBlock:        s.DkgDealingsPerBlock,
		MaxIngressBytesPerMessage:  s.MaxIngressBytesPerMessage,
		MaxIngressBytesPerBlock:    s.MaxIngressBytesPerBlock,
		MaxIngressMessagesPerBlock: s.MaxIngressMessagesPerBlock,
		MaxBlockPayloadSize:        s.MaxBlockPayloadSize,
	}
	if f := s.Features; f != nil {
		sev := f.SevEnabled
		payload.Features = &registry.SubnetFeatures{
			CanisterSandboxing: f.CanisterSandboxing,
			HttpRequests:       f.HTTPRequests,
			SevEnabled:         &sev,
		}
	}
	if rl := s.ResourceLimits; rl != nil {
		size, delta := rl.MaximumStateSize, rl.MaximumStateDelta
		payload.ResourceLimits = &registry.ResourceLimits{
			MaximumStateSize:  &size,
			MaximumStateDelta: &delta,
		}
	}
	blob, err := candid.Marshal([]any{payload})
	if err != nil {
		return nil, fmt.Errorf("encode UpdateSubnetPayload: %w", err)
	}
	return blob, nil
}

func (u UpdateSubnetAction) ProposalAction() (*governance.ProposalActionRequest, error) {
	return execNnsAction(u)
}

// MirrorScope mirrors the target subnet: update_subnet reads the subnet record
// before merging into it, so without the real record the dry-run fails on a
// missing subnet instead of executing the update. The mirror is also what makes
// the local run reject a sev_enabled change the way mainnet would.
func (u UpdateSubnetAction) MirrorScope() MirrorScope {
	return MirrorScope{Subnets: []principal.Principal{u.SubnetID}}
}

// Preflight diffs the declared settings against the subnet's current record.
// Every field is a trustless registry query, so there is nothing here to
// degrade: a setting already at its declared value is a no-op, and a change the
// registry refuses outright (sev_enabled) blocks.
func (u UpdateSubnetAction) Preflight(host string, fetchRootKey bool, opts ...FetchOption) (Preflight, error) {
	rec, err := getSubnetRecord(host, fetchRootKey, u.SubnetID, opts)
	if err != nil {
		return Preflight{}, fmt.Errorf("fetch subnet record: %w", err)
	}
	plan := planUpdateSubnet(u.SubnetID.Encode(), u.Settings, currentSettings(rec))
	return updateSubnetPreflight(plan), nil
}

// currentSettings reads the comparable fields off a subnet record, every one
// set: it is the "before" side of the diff, so nothing about it is optional.
func currentSettings(rec *registry.SubnetRecord) SubnetSettings {
	admins := append([]principal.Principal(nil), rec.SubnetAdmins...)
	ssh := append([]string(nil), rec.SshReadonlyAccess...)
	backup := append([]string(nil), rec.SshBackupAccess...)
	// A record with no features record at all reads as all-false, which is how
	// the registry itself collapses it (SubnetFeatures::default).
	features := SubnetFeatureSet{}
	if f := rec.Features; f != nil {
		features = SubnetFeatureSet{
			CanisterSandboxing: f.CanisterSandboxing,
			HTTPRequests:       f.HttpRequests,
			SevEnabled:         f.SevEnabled != nil && *f.SevEnabled,
		}
	}
	limits := SubnetResourceLimits{}
	if rl := rec.ResourceLimits; rl != nil {
		if rl.MaximumStateSize != nil {
			limits.MaximumStateSize = *rl.MaximumStateSize
		}
		if rl.MaximumStateDelta != nil {
			limits.MaximumStateDelta = *rl.MaximumStateDelta
		}
	}
	return SubnetSettings{
		IsHalted:             &rec.IsHalted,
		HaltAtCupHeight:      &rec.HaltAtCupHeight,
		MaxNumberOfCanisters: &rec.MaxNumberOfCanisters,
		SSHReadonlyAccess:    &ssh,
		SSHBackupAccess:      &backup,
		SubnetAdmins:         &admins,
		Features:             &features,
		ResourceLimits:       &limits,

		UnitDelayMillis:            &rec.UnitDelayMillis,
		InitialNotaryDelayMillis:   &rec.InitialNotaryDelayMillis,
		DkgIntervalLength:          &rec.DkgIntervalLength,
		DkgDealingsPerBlock:        &rec.DkgDealingsPerBlock,
		MaxIngressBytesPerMessage:  &rec.MaxIngressBytesPerMessage,
		MaxIngressBytesPerBlock:    &rec.MaxIngressBytesPerBlock,
		MaxIngressMessagesPerBlock: &rec.MaxIngressMessagesPerBlock,
		MaxBlockPayloadSize:        &rec.MaxBlockPayloadSize,
	}
}

func (u UpdateSubnetAction) RenderPayload(b *strings.Builder, blob []byte) {
	var p registry.UpdateSubnetPayload
	if err := candid.Unmarshal(blob, []any{&p}); err != nil {
		fmt.Fprintf(b, "    <undecodable payload: %v>\n", err)
		return
	}
	fmt.Fprintf(b, "    subnet_id: %s\n", p.SubnetId.Encode())
	// Only the fields the payload actually sets: the rest are not "false" or
	// "0" on-chain, they are untouched, and rendering them as values would read
	// as a change the proposal does not make.
	for _, f := range []struct {
		name string
		val  string
	}{
		{"is_halted", renderOptBool(p.IsHalted)},
		{"halt_at_cup_height", renderOptBool(p.HaltAtCupHeight)},
		{"max_number_of_canisters", renderOptUint(p.MaxNumberOfCanisters)},
		{"ssh_readonly_access", renderOptStrings(p.SshReadonlyAccess)},
		{"ssh_backup_access", renderOptStrings(p.SshBackupAccess)},
		{"subnet_admins", renderOptPrincipals(p.SubnetAdmins)},
		{"features", renderOptFeatures(p.Features)},
		{"resource_limits", renderOptLimits(p.ResourceLimits)},
		{"unit_delay_millis", renderOptUint(p.UnitDelayMillis)},
		{"initial_notary_delay_millis", renderOptUint(p.InitialNotaryDelayMillis)},
		{"dkg_interval_length", renderOptUint(p.DkgIntervalLength)},
		{"dkg_dealings_per_block", renderOptUint(p.DkgDealingsPerBlock)},
		{"max_ingress_bytes_per_message", renderOptUint(p.MaxIngressBytesPerMessage)},
		{"max_ingress_bytes_per_block", renderOptUint(p.MaxIngressBytesPerBlock)},
		{"max_ingress_messages_per_block", renderOptUint(p.MaxIngressMessagesPerBlock)},
		{"max_block_payload_size", renderOptUint(p.MaxBlockPayloadSize)},
	} {
		if f.val != "" {
			fmt.Fprintf(b, "    %s: %s\n", f.name, f.val)
		}
	}
}

func renderOptBool(v *bool) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%t", *v)
}

func renderOptUint(v *uint64) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%d", *v)
}

func renderOptStrings(v *[]string) string {
	if v == nil {
		return ""
	}
	return renderList(*v)
}

func renderOptPrincipals(v *[]principal.Principal) string {
	if v == nil {
		return ""
	}
	return renderList(encodePrincipals(*v))
}

func renderOptFeatures(v *registry.SubnetFeatures) string {
	if v == nil {
		return ""
	}
	return SubnetFeatureSet{
		CanisterSandboxing: v.CanisterSandboxing,
		HTTPRequests:       v.HttpRequests,
		SevEnabled:         v.SevEnabled != nil && *v.SevEnabled,
	}.String()
}

func renderOptLimits(v *registry.ResourceLimits) string {
	if v == nil {
		return ""
	}
	l := SubnetResourceLimits{}
	if v.MaximumStateSize != nil {
		l.MaximumStateSize = *v.MaximumStateSize
	}
	if v.MaximumStateDelta != nil {
		l.MaximumStateDelta = *v.MaximumStateDelta
	}
	return l.String()
}

func (f SubnetFeatureSet) String() string {
	return fmt.Sprintf("canister_sandboxing=%t http_requests=%t sev_enabled=%t",
		f.CanisterSandboxing, f.HTTPRequests, f.SevEnabled)
}

func (l SubnetResourceLimits) String() string {
	return fmt.Sprintf("maximum_state_size=%d maximum_state_delta=%d",
		l.MaximumStateSize, l.MaximumStateDelta)
}

// renderList keeps an empty list legible: "[]" reads as a value, whereas an
// empty string would read as an absent field.
func renderList(vs []string) string {
	if len(vs) == 0 {
		return "[] (empty)"
	}
	return "[" + strings.Join(vs, ", ") + "]"
}

func encodePrincipals(ps []principal.Principal) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Encode()
	}
	return out
}
