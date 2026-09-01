package nns

import (
	"strings"
	"testing"

	governancepb "github.com/swiss-subnet/alpage/nns/pb/governance"
)

// Every kind is declared once, in the kinds table, and everything else is
// derived from it. This pins what a complete declaration means, because the
// registration that used to be forgotten (the payload renderer) failed silently:
// `alp status` printed a bare function number and nothing else noticed.
func TestKindsAreFullyRegistered(t *testing.T) {
	names := make(map[string]bool, len(kinds))
	functions := make(map[int32]string, len(kinds))

	for _, k := range kinds {
		t.Run(k.Name, func(t *testing.T) {
			if names[k.Name] {
				t.Fatalf("kind %q is declared twice", k.Name)
			}
			names[k.Name] = true

			if k.Decode == nil || k.Body == nil || k.Action == nil {
				t.Fatal("kind needs a Decode, a Body, and an Action")
			}
			if k.Doc == "" || k.Since == "" {
				t.Error("kind is undocumented, so schemadoc would emit an empty description")
			}
			if got := k.Action.Kind(); got != k.Name {
				t.Errorf("action reports kind %q, table says %q", got, k.Name)
			}

			fn := int32(k.Action.NnsFunction())
			if k.Action.NnsFunction() == governancepb.NnsFunction_NNS_FUNCTION_UNSPECIFIED {
				// A native governance action carries no function number, so there
				// is nothing to dispatch rendering on.
				if k.Function != "" {
					t.Errorf("native action carries function label %q", k.Function)
				}
				return
			}
			if prev, dup := functions[fn]; dup {
				t.Errorf("kinds %q and %q both claim function %d", prev, k.Name, fn)
			}
			functions[fn] = k.Name

			if k.Function == "" {
				t.Error("no canister function name, so status would label the proposal by number")
			}
			if funcName[fn] != k.Function {
				t.Errorf("funcName[%d] = %q, want %q", fn, funcName[fn], k.Function)
			}
			if renderer[fn] == nil {
				t.Errorf("no payload renderer for function %d", fn)
			}
		})
	}
}

// The kinds table is also the list of what a config may say, so an unknown kind
// should name the alternatives rather than just rejecting the value.
func TestUnknownKindNamesTheKnownOnes(t *testing.T) {
	s := &Spec{Name: "p", Kind: "make_coffee"}
	_, err := s.Action()
	if err == nil {
		t.Fatal("want an error for an unknown kind")
	}
	for _, want := range []string{"make_coffee", "membership", "update_subnet"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}

// Every kind's nested block must reach docs/config.md, which is what schemadoc
// walks. A kind missing from it would be undocumented config.
func TestKindSchemaBlocksCoverEveryKind(t *testing.T) {
	documented := make(map[string]bool, len(SchemaBlocks))
	for _, blk := range SchemaBlocks {
		documented[blk.Name] = true
	}
	for _, k := range kinds {
		if !documented[k.Name] {
			t.Errorf("kind %q has no schema block", k.Name)
		}
	}
}
