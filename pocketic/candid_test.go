package pocketic

import (
	"testing"

	"github.com/aviate-labs/agent-go/candid"
	"github.com/aviate-labs/agent-go/candid/idl"
	"github.com/aviate-labs/agent-go/principal"
)

func TestProvisionalCreateArgsEncode(t *testing.T) {
	id := principal.MustDecode("rrkah-fqaaa-aaaaa-aaaaq-cai")
	amount := 0
	args := provisionalCreateArgs{Amount: &amount, SpecifiedID: &id}
	b, err := candid.Marshal([]any{args})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("empty encoding")
	}
}

func TestInstallCodeArgsEncode(t *testing.T) {
	id := principal.MustDecode("rrkah-fqaaa-aaaaa-aaaaq-cai")
	args := installCodeArgs{
		Mode:       installMode{Install: &idl.Null{}},
		CanisterID: id,
		WasmModule: []byte{0x00, 0x61, 0x73, 0x6d},
		Arg:        []byte{},
	}
	b, err := candid.Marshal([]any{args})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("empty encoding")
	}
}
