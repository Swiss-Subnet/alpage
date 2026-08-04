package nns

import (
	"reflect"
	"strings"
	"testing"
)

// Every documented field needs a Since entry, and every Since entry needs a
// live field: the same lockstep rule schemadoc enforces for SchemaFieldDocs.
func TestSchemaFieldSinceCoverage(t *testing.T) {
	for key := range SchemaFieldDocs {
		if _, ok := SchemaFieldSince[key]; !ok {
			t.Errorf("SchemaFieldSince is missing %q", key)
		}
	}
	for key := range SchemaFieldSince {
		if _, ok := SchemaFieldDocs[key]; !ok {
			t.Errorf("SchemaFieldSince has a stale entry %q", key)
		}
	}
}

func TestSchemaFieldSinceValues(t *testing.T) {
	want := map[string]string{
		"Subnet.id":                "v0.1.0",
		"Subnet.sev_enabled":       "v0.1.1",
		"NodeRes.decommissioned":   "v0.1.1",
		"Subnet.type":              "v0.2.0",
		"Subnet.cost_schedule":     "v0.2.0",
		"Subnet.admins":            "v0.2.0",
		"NodeRes.guestos_version":  "v0.2.0",
		"GuestosVersionRes.id":     "v0.3.0",
		"GuestosVersionRes.label":  "v0.3.0",
		"membershipBody.subnet_id": "v0.3.0",
	}
	for key, w := range want {
		if got := SchemaFieldSince[key]; got != w {
			t.Errorf("SchemaFieldSince[%q] = %q, want %q", key, got, w)
		}
	}
}

// Blocks carry a Since too, so a renamed block reads as new rather than
// inheriting the old name's version.
func TestSchemaBlockSince(t *testing.T) {
	for _, blk := range SchemaBlocks {
		if blk.Since == "" {
			t.Errorf("block %q has no Since", blk.Name)
		}
		if !strings.HasPrefix(blk.Since, "v") {
			t.Errorf("block %q Since %q is not a vN.N.N tag", blk.Name, blk.Since)
		}
	}
	got := map[string]string{}
	for _, blk := range SchemaBlocks {
		got[blk.Name] = blk.Since
	}
	want := map[string]string{
		"subnet": "v0.1.0", "data_center": "v0.1.0", "node_provider": "v0.1.0",
		"node_operator": "v0.1.0", "node": "v0.1.0", "provider": "v0.1.0",
		"proposal": "v0.1.0", "deploy_guestos": "v0.1.0", "add / remove": "v0.1.0",
		"guestos_version": "v0.3.0", "membership": "v0.3.0",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("block Since map:\ngot  %v\nwant %v", got, want)
	}
}
