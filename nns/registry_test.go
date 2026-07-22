package nns

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenderResourcesHCLParses(t *testing.T) {
	subnetID := "3zsyy-cnoqf-tvlun-ymf55-tkpca-ox7uw-kfxoh-7khwq-2gz43-wafem-lqe"
	nodeIDs := []string{
		"au6oc-imc3w-ssdnk-lzy6e-6fgeh-ejwch-bqohf-vj624-k5xfl-77rpz-xqe",
		"3wbrf-zokqb-6euxi-6lxxo-i5tia-4742s-7jfsj-touui-qwzbm-7rmdw-nae",
	}
	out := RenderResourcesHCL(subnetID, nodeIDs)

	path := filepath.Join(t.TempDir(), "resources.hcl")
	if err := os.WriteFile(path, []byte(out), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := loadResources(path)
	if err != nil {
		t.Fatalf("rendered HCL does not parse: %v\n---\n%s", err, out)
	}
	if len(res.Subnets) != 1 || res.Subnets[0].ID != subnetID {
		t.Errorf("subnet not round-tripped: %+v", res.Subnets)
	}
	if len(res.Nodes) != len(nodeIDs) {
		t.Errorf("got %d nodes, want %d", len(res.Nodes), len(nodeIDs))
	}
	got := map[string]bool{}
	for _, n := range res.Nodes {
		got[n.ID] = true
	}
	for _, id := range nodeIDs {
		if !got[id] {
			t.Errorf("node id %q missing after round-trip", id)
		}
	}
}
