package nns

import (
	"fmt"
	"sort"
	"strings"

	registrypb "github.com/swiss-subnet/alpage/nns/pb/registry"
)

// Subnet type and cost schedule names as written in HCL, derived from the proto
// enums rather than restated: the proto spells each variant as a prefix plus the
// HCL name uppercased (SUBNET_TYPE_CLOUD_ENGINE <-> cloud_engine), which is also
// how the registry's Candid variant names them. Deriving means a variant added
// upstream becomes declarable without a change here.
var (
	SubnetTypeApplication         = subnetTypeName(registrypb.SubnetType_SUBNET_TYPE_APPLICATION)
	SubnetTypeVerifiedApplication = subnetTypeName(registrypb.SubnetType_SUBNET_TYPE_VERIFIED_APPLICATION)
	SubnetTypeSystem              = subnetTypeName(registrypb.SubnetType_SUBNET_TYPE_SYSTEM)
	SubnetTypeCloudEngine         = subnetTypeName(registrypb.SubnetType_SUBNET_TYPE_CLOUD_ENGINE)

	CostScheduleNormal = costScheduleName(registrypb.CanisterCyclesCostSchedule_CANISTER_CYCLES_COST_SCHEDULE_NORMAL)
	CostScheduleFree   = costScheduleName(registrypb.CanisterCyclesCostSchedule_CANISTER_CYCLES_COST_SCHEDULE_FREE)
)

func subnetTypeName(t registrypb.SubnetType) string {
	return strings.ToLower(strings.TrimPrefix(t.String(), "SUBNET_TYPE_"))
}

func costScheduleName(s registrypb.CanisterCyclesCostSchedule) string {
	return strings.ToLower(strings.TrimPrefix(s.String(), "CANISTER_CYCLES_COST_SCHEDULE_"))
}

// subnetTypeNames is every declarable subnet type, so an unknown one is rejected
// against the enum rather than a hand-written list. UNSPECIFIED is excluded: it
// is the proto's zero value, not a type a subnet can be declared as.
func subnetTypeNames() map[string]bool {
	out := make(map[string]bool, len(registrypb.SubnetType_name))
	for v := range registrypb.SubnetType_name {
		t := registrypb.SubnetType(v)
		if t == registrypb.SubnetType_SUBNET_TYPE_UNSPECIFIED {
			continue
		}
		out[subnetTypeName(t)] = true
	}
	return out
}

func costScheduleNames() map[string]bool {
	out := make(map[string]bool, len(registrypb.CanisterCyclesCostSchedule_name))
	for v := range registrypb.CanisterCyclesCostSchedule_name {
		s := registrypb.CanisterCyclesCostSchedule(v)
		if s == registrypb.CanisterCyclesCostSchedule_CANISTER_CYCLES_COST_SCHEDULE_UNSPECIFIED {
			continue
		}
		out[costScheduleName(s)] = true
	}
	return out
}

// maxSubnetAdmins mirrors MAX_SUBNET_ADMINS in the registry invariants
// (rs/registry/canister/src/invariants/subnet.rs). Rust-side, so not derivable
// from the protos.
const maxSubnetAdmins = 10

// SubnetRecordFacts is the subset of a subnet record reconcile compares against,
// as read from the registry. An empty Type or CostSchedule is the record
// carrying no value, which reads as that field's default.
type SubnetRecordFacts struct {
	Type         string
	CostSchedule string
	Admins       []string
	Features     SubnetFeatures
}

// AdminsReconcile is the diff of a subnet's declared admins against the live
// record. Missing are declared but absent on-chain; Unexpected are on-chain but
// undeclared.
type AdminsReconcile struct {
	Status     FeatureStatus
	Missing    []string
	Unexpected []string
}

// SubnetRecordReconcile is the diff of one subnet's declared record fields
// against the live registry record.
type SubnetRecordReconcile struct {
	SubnetID     string
	Type         FeatureReconcile
	CostSchedule FeatureReconcile
	Admins       AdminsReconcile
	Features     FeatureReconcile
}

// DeclaredType is the subnet's declared type, defaulting to application.
// Omitting it asserts application rather than skipping the check, so a subnet
// whose type changes on-chain surfaces as drift.
func (s Subnet) DeclaredType() string {
	if s.Type == "" {
		return SubnetTypeApplication
	}
	return s.Type
}

// DeclaredCostSchedule is the subnet's declared cost schedule: free for cloud
// engines, which the registry requires to be free, and normal otherwise.
func (s Subnet) DeclaredCostSchedule() string {
	if s.CostSchedule != "" {
		return s.CostSchedule
	}
	if s.DeclaredType() == SubnetTypeCloudEngine {
		return CostScheduleFree
	}
	return CostScheduleNormal
}

// ValidateSubnet rejects a declaration the registry would refuse, so the error
// surfaces at plan time rather than as a failed proposal. It mirrors
// check_subnet_cost_schedule_invariant and check_subnet_admins_invariant.
func ValidateSubnet(s Subnet) error {
	typ := s.DeclaredType()
	if !subnetTypeNames()[typ] {
		return fmt.Errorf("subnet %q: unknown type %q", s.Name, s.Type)
	}
	sched := s.DeclaredCostSchedule()
	if !costScheduleNames()[sched] {
		return fmt.Errorf("subnet %q: unknown cost_schedule %q", s.Name, s.CostSchedule)
	}
	if typ == SubnetTypeCloudEngine && sched != CostScheduleFree {
		return fmt.Errorf("subnet %q: a cloud_engine must be on the free cost schedule, got %q", s.Name, sched)
	}
	if sched == CostScheduleFree && typ != SubnetTypeApplication && typ != SubnetTypeCloudEngine {
		return fmt.Errorf("subnet %q: only application and cloud_engine subnets may be on the free cost schedule, got %q", s.Name, typ)
	}
	if len(s.Admins) == 0 {
		return nil
	}
	// Admins are allowed only on a rented subnet or a cloud engine. The registry
	// infers rented-ness from application + free; there is no rented subnet type
	// to check against.
	rented := typ == SubnetTypeApplication && sched == CostScheduleFree
	engine := typ == SubnetTypeCloudEngine && sched == CostScheduleFree
	if !rented && !engine {
		return fmt.Errorf("subnet %q: admins are allowed only on a cloud_engine or a rented (application + free) subnet, got type %q on the %q schedule", s.Name, typ, sched)
	}
	if len(s.Admins) > maxSubnetAdmins {
		return fmt.Errorf("subnet %q: %d admins exceeds the maximum of %d", s.Name, len(s.Admins), maxSubnetAdmins)
	}
	return nil
}

// ReconcileSubnetRecord diffs a subnet's declared record fields against the live
// registry record. Pure: the caller fetches live and passes it in.
func ReconcileSubnetRecord(sn Subnet, live SubnetRecordFacts) SubnetRecordReconcile {
	liveType := live.Type
	if liveType == "" {
		liveType = SubnetTypeApplication
	}
	liveSched := live.CostSchedule
	if liveSched == "" {
		liveSched = CostScheduleNormal
	}
	return SubnetRecordReconcile{
		SubnetID:     sn.ID,
		Type:         compareField(sn.ID, "type", sn.DeclaredType(), liveType),
		CostSchedule: compareField(sn.ID, "cost_schedule", sn.DeclaredCostSchedule(), liveSched),
		Admins:       reconcileAdmins(sn.Admins, live.Admins),
		Features:     ReconcileSubnetFeatures(sn, live.Features),
	}
}

func compareField(subnetID, name, declared, live string) FeatureReconcile {
	fr := FeatureReconcile{
		SubnetID: subnetID,
		Name:     name,
		Declared: declared,
		Live:     live,
		Status:   FeatureMismatch,
	}
	if declared == live {
		fr.Status = FeatureInSync
	}
	return fr
}

// reconcileAdmins diffs the declared admin set against the live one. Order is
// not meaningful on-chain, so only set membership is compared. Declaring none
// asserts none rather than skipping the check.
func reconcileAdmins(declared, live []string) AdminsReconcile {
	liveSet := make(map[string]bool, len(live))
	for _, p := range live {
		liveSet[p] = true
	}
	declSet := make(map[string]bool, len(declared))
	for _, p := range declared {
		declSet[p] = true
	}
	ar := AdminsReconcile{Status: FeatureInSync}
	for p := range declSet {
		if !liveSet[p] {
			ar.Missing = append(ar.Missing, p)
		}
	}
	for p := range liveSet {
		if !declSet[p] {
			ar.Unexpected = append(ar.Unexpected, p)
		}
	}
	sort.Strings(ar.Missing)
	sort.Strings(ar.Unexpected)
	if len(ar.Missing) > 0 || len(ar.Unexpected) > 0 {
		ar.Status = FeatureMismatch
	}
	return ar
}

// HasDrift reports whether any declared record field disagrees with the live
// record.
func (rs SubnetRecordReconcile) HasDrift() bool {
	return rs.Type.HasDrift() || rs.CostSchedule.HasDrift() ||
		rs.Admins.Status == FeatureMismatch || rs.Features.HasDrift()
}

// Render writes one line per record field, matching the subnet-feature layout.
func (rs SubnetRecordReconcile) Render(b *strings.Builder) {
	rs.Type.Render(b)
	rs.CostSchedule.Render(b)
	rs.Features.Render(b)
	rs.Admins.render(b)
}

func (ar AdminsReconcile) render(b *strings.Builder) {
	fmt.Fprintf(b, "  %s  admins\n", colorize(string(ar.Status), ar.Status.color()))
	for _, p := range ar.Missing {
		fmt.Fprintf(b, "      + %s (declared, not on-chain)\n", p)
	}
	for _, p := range ar.Unexpected {
		fmt.Fprintf(b, "      - %s (on-chain, not declared)\n", p)
	}
}
