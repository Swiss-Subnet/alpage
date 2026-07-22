package pocketic

import (
	"os"
	"testing"
)

// TestLiveInstance boots the real pinned pocket-ic server and creates an
// NNS+application instance. Requires POCKET_IC_BIN (set by the nix devShell).
func TestLiveInstance(t *testing.T) {
	if os.Getenv("POCKET_IC_BIN") == "" {
		t.Skip("POCKET_IC_BIN not set; run inside nix develop")
	}
	c, err := Start("")
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer c.Close()

	inst, err := c.NewInstance()
	if err != nil {
		t.Fatalf("new instance: %v", err)
	}
	t.Logf("created instance %d", inst)
}
