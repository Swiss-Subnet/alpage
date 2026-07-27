package nns

import (
	"fmt"
	"strings"
)

// SubnetFeatures is a subnet record's feature flags as read from the registry.
// A nil SevEnabled is the registry omitting the field, which means off.
type SubnetFeatures struct {
	SevEnabled *bool
}

// FeatureStatus classifies one declared subnet feature against the live record.
type FeatureStatus string

const (
	// FeatureInSync: the declared feature matches the live subnet record.
	FeatureInSync FeatureStatus = "in-sync"
	// FeatureMismatch: the declared feature disagrees with the live record.
	FeatureMismatch FeatureStatus = "mismatch"
)

// FeatureReconcile is the diff of one subnet's declared features against its
// live on-chain record.
type FeatureReconcile struct {
	SubnetID string
	Name     string // feature name, e.g. "sev_enabled"
	Status   FeatureStatus
	Declared string
	Live     string
}

// ReconcileSubnetFeatures diffs a subnet's declared features against the live
// subnet record. Pure: the caller fetches live and passes it in. Omitting
// sev_enabled asserts not-enabled rather than skipping the check, so a subnet
// that gains SEV on-chain without a config change surfaces as drift.
func ReconcileSubnetFeatures(sn Subnet, live SubnetFeatures) FeatureReconcile {
	liveOn := live.SevEnabled != nil && *live.SevEnabled
	fr := FeatureReconcile{
		SubnetID: sn.ID,
		Name:     "sev_enabled",
		Declared: fmt.Sprintf("%v", sn.SevEnabled),
		Live:     fmt.Sprintf("%v", liveOn),
		Status:   FeatureMismatch,
	}
	if sn.SevEnabled == liveOn {
		fr.Status = FeatureInSync
	}
	return fr
}

// HasDrift reports whether the declared feature disagrees with the live record.
func (fr FeatureReconcile) HasDrift() bool { return fr.Status == FeatureMismatch }

// Render writes a human-readable line for one subnet feature.
func (fr FeatureReconcile) Render(b *strings.Builder) {
	status := colorize(string(fr.Status), fr.Status.color())
	fmt.Fprintf(b, "  %s  %s  declared=%s live=%s\n", status, fr.Name, fr.Declared, fr.Live)
}

func (s FeatureStatus) color() string {
	if s == FeatureInSync {
		return ansiGreen
	}
	return ansiRed
}
