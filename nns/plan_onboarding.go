package nns

import (
	"fmt"
	"sort"
	"strings"
)

// Mirror image of the removal planners: same PreconditionPlan, opposite
// expectation. A registration wants its target absent and the records it points
// at present.

// planAddDataCenters checks the two things do_add_or_remove_data_centers does
// badly. An id the registry already holds makes it panic after adoption, even
// though the mutation it builds is an upsert. A record failing the canister's
// length validation is dropped from the batch with only a log line, so the
// proposal executes, reports success, and registers nothing.
func planAddDataCenters(specs []DataCenterSpec, current map[string]DataCenterSpec) PreconditionPlan {
	p := PreconditionPlan{Kind: "add_data_centers"}
	for _, s := range specs {
		p.Targets = append(p.Targets, s.ID)
		if have, ok := current[s.ID]; ok {
			detail := "already registered, so execution would trap"
			if d := have.diff(s); d != "" {
				detail += " (" + d + ")"
			}
			p.Issues = append(p.Issues, PreconditionIssue{
				Target: s.ID, Detail: detail, Blocking: true,
			})
			continue
		}
		if field, n := s.tooLong(); field != "" {
			p.Issues = append(p.Issues, PreconditionIssue{
				Target:   s.ID,
				Detail:   fmt.Sprintf("%s is %d bytes, over the canister's 255 limit. The record would be skipped and the proposal would still report success", field, n),
				Blocking: true,
			})
		}
	}
	return p
}

// planAddNodeProvider checks what governance validates before the vote: ToAdd
// is rejected outright for a principal already on the provider list. An absent
// reward_account is not an issue. get_node_provider_reward falls back to the
// default ledger account of the provider principal.
func planAddNodeProvider(providerID string, registered bool) PreconditionPlan {
	p := PreconditionPlan{Kind: "add_node_provider", Targets: []string{providerID}}
	if registered {
		p.Issues = append(p.Issues, PreconditionIssue{
			Target:   providerID,
			Detail:   "already a registered node provider, so governance would reject the proposal",
			Blocking: true,
		})
	}
	return p
}

// planAddNodeOperator checks the three conditions that decide whether this
// lands a working operator. do_add_node_operator traps on an existing record
// and the node-operator invariant rejects an unknown dc_id, both after
// adoption. Governance rejects an unregistered provider at submission.
func planAddNodeOperator(operatorID, providerID, dcID string, operatorExists, dcExists, providerRegistered bool, allowance uint64) PreconditionPlan {
	p := PreconditionPlan{Kind: "add_node_operator", Targets: []string{operatorID}}
	if operatorExists {
		p.Issues = append(p.Issues, PreconditionIssue{
			Target:   operatorID,
			Detail:   "a node operator record already exists for this principal, so execution would trap",
			Blocking: true,
		})
	}
	if !dcExists {
		p.Issues = append(p.Issues, PreconditionIssue{
			Target:   operatorID,
			Detail:   fmt.Sprintf("data center %q is not in the registry. Submit the add_data_centers proposal first", dcID),
			Blocking: true,
		})
	}
	if !providerRegistered {
		p.Issues = append(p.Issues, PreconditionIssue{
			Target:   operatorID,
			Detail:   fmt.Sprintf("node provider %s is not registered; governance would reject this at submission. Submit the add_node_provider proposal first", providerID),
			Blocking: true,
		})
	}
	if allowance == 0 {
		p.Issues = append(p.Issues, PreconditionIssue{
			Target: operatorID,
			Detail: "node_allowance is 0. Cloud Engine operators do not need an allowance, but any other operator cannot register a node until update_node_operator_config raises it",
		})
	}
	return p
}

// tooLong reports the first field over the canister's 255 byte limit, which is
// all DataCenterRecord::validate checks.
func (d DataCenterSpec) tooLong() (string, int) {
	for _, f := range []struct {
		name  string
		value string
	}{{"id", d.ID}, {"region", d.Region}, {"owner", d.Owner}} {
		if len(f.value) > 255 {
			return f.name, len(f.value)
		}
	}
	return "", 0
}

// diff names the fields that differ between the record the registry holds (d)
// and the one the payload would write.
func (d DataCenterSpec) diff(want DataCenterSpec) string {
	var parts []string
	if d.Region != want.Region {
		parts = append(parts, fmt.Sprintf("region %q -> %q", d.Region, want.Region))
	}
	if d.Owner != want.Owner {
		parts = append(parts, fmt.Sprintf("owner %q -> %q", d.Owner, want.Owner))
	}
	if !gpsEqual(d.GPS, want.GPS) {
		parts = append(parts, fmt.Sprintf("gps %s -> %s", gpsString(d.GPS), gpsString(want.GPS)))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func gpsEqual(a, b *GPSCoordinates) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func gpsString(g *GPSCoordinates) string {
	if g == nil {
		return "none"
	}
	return fmt.Sprintf("%g,%g", g.Latitude, g.Longitude)
}
