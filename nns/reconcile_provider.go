package nns

import (
	"fmt"
	"sort"
	"strings"
)

// OperatorStatus classifies a declared node_operator against what the registry
// says its provider owns.
type OperatorStatus string

const (
	// OperatorOK: the declared provider owns this operator and the dc matches.
	OperatorOK OperatorStatus = "ok"
	// OperatorDcMismatch: the provider owns the operator, but at a different dc.
	OperatorDcMismatch OperatorStatus = "dc-mismatch"
	// OperatorUnknown: the declared provider does not own this operator.
	OperatorUnknown OperatorStatus = "unknown"
	// OperatorProviderUnchecked: the declared provider was never queried, so
	// there is nothing to compare against.
	OperatorProviderUnchecked OperatorStatus = "provider-unchecked"
	// OperatorDecommissioned: the operator, or its provider, is declared
	// decommissioned and is absent from the registry as expected. Not drift.
	OperatorDecommissioned OperatorStatus = "decommissioned"
	// OperatorStillRegistered: declared decommissioned but the registry still
	// records it. The removal has not landed.
	OperatorStillRegistered OperatorStatus = "still-registered"
)

type OperatorRow struct {
	OperatorID string
	Name       string
	Provider   string // declared provider id
	Status     OperatorStatus
	Detail     string
}

// ProviderReconcile is the diff of declared node_operator resources against the
// registry's provider ownership.
type ProviderReconcile struct {
	Operators []OperatorRow
}

// ReconcileProviders checks each declared node_operator against the operators
// its declared provider actually owns on-chain. byProvider maps a declared
// provider id to the (operator, dc) pairs the registry returns for it; a missing
// key means that provider was never queried. An empty slice only means the
// provider owns no operators, not that it is absent: the registry cannot tell
// those apart. Pure: the caller fetches byProvider.
func ReconcileProviders(r *Resources, byProvider map[string][]ProviderOperator) ProviderReconcile {
	dcRegion := map[string]string{}
	for _, dc := range r.DCs {
		dcRegion[dc.ID] = dc.Region
	}
	goneProvider := make(map[string]bool, len(r.Providers))
	for _, p := range r.Providers {
		if p.Decommissioned {
			goneProvider[p.ID] = true
		}
	}
	var pr ProviderReconcile
	for _, op := range r.Operators {
		if op.Provider == "" {
			continue
		}
		row := OperatorRow{OperatorID: op.ID, Name: op.Name, Provider: op.Provider}
		owned, known := byProvider[op.Provider]
		// A decommissioned operator, or one under a decommissioned provider, is
		// expected to be absent; only its lingering presence is drift.
		retired := op.Decommissioned || goneProvider[op.Provider]
		switch {
		case retired && operatorOwned(op.ID, owned):
			row.Status = OperatorStillRegistered
			row.Detail = retiredDetail(op, goneProvider[op.Provider])
		case retired:
			row.Status = OperatorDecommissioned
			row.Detail = retiredDetail(op, goneProvider[op.Provider])
		case !known:
			row.Status = OperatorProviderUnchecked
			row.Detail = "provider not queried"
		default:
			row.Status, row.Detail = classifyOperator(op, owned, dcRegion)
		}
		pr.Operators = append(pr.Operators, row)
	}
	sort.Slice(pr.Operators, func(i, j int) bool { return pr.Operators[i].OperatorID < pr.Operators[j].OperatorID })
	return pr
}

func operatorOwned(opID string, owned []ProviderOperator) bool {
	for _, o := range owned {
		if o.OperatorID == opID {
			return true
		}
	}
	return false
}

// retiredDetail says which declaration retired the operator: its own, or its
// provider's.
func retiredDetail(op NodeOperator, providerGone bool) string {
	switch {
	case op.Decommissioned && providerGone:
		return "operator and provider decommissioned"
	case op.Decommissioned:
		return "operator decommissioned"
	default:
		return "provider decommissioned"
	}
}

func classifyOperator(op NodeOperator, owned []ProviderOperator, dcRegion map[string]string) (OperatorStatus, string) {
	for _, o := range owned {
		if o.OperatorID != op.ID {
			continue
		}
		if op.Dc != "" && o.DcID != op.Dc {
			return OperatorDcMismatch, fmt.Sprintf("declared dc %q, on-chain dc %q", op.Dc, o.DcID)
		}
		if want := dcRegion[op.Dc]; want != "" && o.DcRegion != "" && want != o.DcRegion {
			return OperatorDcMismatch, fmt.Sprintf("dc %q region declared %q, on-chain %q", op.Dc, want, o.DcRegion)
		}
		return OperatorOK, ""
	}
	return OperatorUnknown, "provider does not own this operator on-chain"
}

func (pr ProviderReconcile) Table() Table {
	t := Table{Header: "node operators"}
	for _, row := range pr.Operators {
		detail := row.Detail
		if detail != "" {
			detail = "  (" + detail + ")"
		}
		t.Rows = append(t.Rows, Row{
			Status: string(row.Status),
			Name:   row.Name,
			ID:     row.OperatorID,
			Detail: detail,
			Remedy: remedyOf(row.Status),
			OK:     row.Status == OperatorOK || row.Status == OperatorDecommissioned,
			Color:  row.Status.color(),
		})
	}
	return t
}

// remedy names what clears each operator status. A dc change needs
// update_node_operator_config, which alpage does not implement, so that one
// says to correct the declaration rather than pointing at a kind that is not
// there.
func (s OperatorStatus) remedy() string {
	switch s {
	case OperatorDcMismatch:
		return "correct the operator's dc in resources.hcl, since moving it on-chain needs update_node_operator_config and alpage has no kind for that"
	case OperatorUnknown:
		return "point the operator at the provider that owns it, or register it with an add_node_operator proposal"
	case OperatorProviderUnchecked:
		return "the declared provider was never queried, so check that its id is right"
	case OperatorStillRegistered:
		return "the removal has not landed, so submit a remove_node_operators proposal or drop the decommissioned marker"
	default:
		return ""
	}
}

func (pr ProviderReconcile) HasDrift() bool { return pr.Table().HasDrift() }

func (pr ProviderReconcile) Render(b *strings.Builder) { pr.Table().Render(b) }

func (s OperatorStatus) color() string {
	if s == OperatorOK || s == OperatorDecommissioned {
		return ansiGreen
	}
	return ansiRed
}
