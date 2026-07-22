package nns

import (
	"os"
	"testing"

	"github.com/aviate-labs/agent-go/principal"
	"github.com/swiss-subnet/alpage/pocketic"
)

// testController generates a real ed25519 identity and returns its principal.
func testController() principal.Principal {
	id, err := NewIdentity()
	if err != nil {
		panic(err)
	}
	return id.Principal()
}

func TestBringUp(t *testing.T) {
	if os.Getenv("POCKET_IC_BIN") == "" {
		t.Skip("POCKET_IC_BIN not set; run inside nix develop")
	}
	c, err := pocketic.Start("")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer c.Close()
	inst, err := c.NewInstance()
	if err != nil {
		t.Fatalf("instance: %v", err)
	}
	if _, err := BringUp(c, inst, testController()); err != nil {
		t.Fatalf("bring up NNS: %v", err)
	}
	t.Log("NNS installed: registry, root, governance")
}
