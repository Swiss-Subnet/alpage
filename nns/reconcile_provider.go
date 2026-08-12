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
	// OperatorProviderAbsent: the operator's declared provider returned nothing
	// from the registry (unknown provider id).
	OperatorProviderAbsent OperatorStatus = "provider-absent"
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
// provider id to the (operator, dc) pairs the registry returns for it (nil or
// missing => provider unknown). Pure: the caller fetches byProvider.
func ReconcileProviders(r *Resources, byProvider map[string][]ProviderOperator) ProviderReconcile {
	dcRegion := map[string]string{}
	for _, dc := range r.DCs {
		dcRegion[dc.ID] = dc.Region
	}
	var pr ProviderReconcile
	for _, op := range r.Operators {
		if op.Provider == "" {
			continue
		}
		row := OperatorRow{OperatorID: op.ID, Name: op.Name, Provider: op.Provider}
		owned, known := byProvider[op.Provider]
		switch {
		case !known || len(owned) == 0:
			row.Status = OperatorProviderAbsent
			row.Detail = "provider not found in registry"
		default:
			row.Status, row.Detail = classifyOperator(op, owned, dcRegion)
		}
		pr.Operators = append(pr.Operators, row)
	}
	sort.Slice(pr.Operators, func(i, j int) bool { return pr.Operators[i].OperatorID < pr.Operators[j].OperatorID })
	return pr
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
			OK:     row.Status == OperatorOK,
			Color:  row.Status.color(),
		})
	}
	return t
}

func (pr ProviderReconcile) HasDrift() bool { return pr.Table().HasDrift() }

func (pr ProviderReconcile) Render(b *strings.Builder) { pr.Table().Render(b) }

func (s OperatorStatus) color() string {
	if s == OperatorOK {
		return ansiGreen
	}
	return ansiRed
}
