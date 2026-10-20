package nns

import (
	"fmt"
	"sort"
	"strings"
)

// DataCenterStatus classifies a declared data_center against its registry
// record. The region is also compared through an operator in
// reconcile_provider.go, but only for a dc some declared operator points at
// under a queried provider, so a data_center block on its own went unchecked.
type DataCenterStatus string

const (
	// DataCenterOK: the registry holds this dc and the region matches.
	DataCenterOK DataCenterStatus = "ok"
	// DataCenterMissing: declared but the registry has no record for it.
	DataCenterMissing DataCenterStatus = "missing"
	// DataCenterRegionMismatch: the record exists under a different region.
	DataCenterRegionMismatch DataCenterStatus = "region-mismatch"
	// DataCenterRegionUnchecked: declared without a region, so the record's
	// region is accepted as-is.
	DataCenterRegionUnchecked DataCenterStatus = "region-unchecked"
)

type DataCenterRow struct {
	DcID   string
	Name   string
	Status DataCenterStatus
	Detail string
}

// DataCenterReconcile is the diff of declared data_center resources against
// their registry records.
type DataCenterReconcile struct {
	DataCenters []DataCenterRow
}

// ReconcileDataCenters checks each declared data_center against the registry.
// onChain maps a dc id to its record, with absent ids left out. Pure: the
// caller fetches onChain, typically with FetchDataCenters.
func ReconcileDataCenters(r *Resources, onChain map[string]DataCenterSpec) DataCenterReconcile {
	var dr DataCenterReconcile
	for _, dc := range r.DCs {
		row := DataCenterRow{DcID: dc.ID, Name: dc.Name}
		have, ok := onChain[dc.ID]
		switch {
		case !ok:
			row.Status = DataCenterMissing
			row.Detail = "no registry record"
		case dc.Region == "":
			row.Status = DataCenterRegionUnchecked
			row.Detail = fmt.Sprintf("no region declared, on-chain %q", have.Region)
		case dc.Region != have.Region:
			row.Status = DataCenterRegionMismatch
			row.Detail = fmt.Sprintf("declared region %q, on-chain %q", dc.Region, have.Region)
		default:
			row.Status = DataCenterOK
		}
		dr.DataCenters = append(dr.DataCenters, row)
	}
	sort.Slice(dr.DataCenters, func(i, j int) bool { return dr.DataCenters[i].DcID < dr.DataCenters[j].DcID })
	return dr
}

func (dr DataCenterReconcile) Table() Table {
	t := Table{Header: "data centers"}
	for _, row := range dr.DataCenters {
		detail := row.Detail
		if detail != "" {
			detail = "  (" + detail + ")"
		}
		t.Rows = append(t.Rows, Row{
			Status: string(row.Status),
			Name:   row.Name,
			ID:     row.DcID,
			Detail: detail,
			Remedy: remedyOf(row.Status),
			OK:     row.Status == DataCenterOK || row.Status == DataCenterRegionUnchecked,
			Color:  row.Status.color(),
		})
	}
	return t
}

// remedy names what clears each data center status. The region case warns off
// the obvious wrong move: add_or_remove_data_centers traps on an id it already
// holds, so a region cannot be corrected by re-adding the record.
func (s DataCenterStatus) remedy() string {
	switch s {
	case DataCenterMissing:
		return "register it with an add_data_centers proposal"
	case DataCenterRegionMismatch:
		return "correct the region in resources.hcl, since the canister traps on re-adding a dc it already holds"
	default:
		return ""
	}
}

func (dr DataCenterReconcile) HasDrift() bool { return dr.Table().HasDrift() }

func (dr DataCenterReconcile) Render(b *strings.Builder) { dr.Table().Render(b) }

func (s DataCenterStatus) color() string {
	if s == DataCenterOK || s == DataCenterRegionUnchecked {
		return ansiGreen
	}
	return ansiRed
}
