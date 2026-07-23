package nns

import "testing"

func res(nodes []NodeRes, subnets []Subnet) *Resources {
	labels := map[string]string{}
	for _, n := range nodes {
		labels[n.ID] = n.Label
	}
	for _, s := range subnets {
		labels[s.ID] = s.Label
	}
	return &Resources{Nodes: nodes, Subnets: subnets, labels: labels}
}

func rowFor(rc SubnetReconcile, id string) *ReconcileRow {
	for i := range rc.Rows {
		if rc.Rows[i].NodeID == id {
			return &rc.Rows[i]
		}
	}
	return nil
}

func TestReconcileClassifies(t *testing.T) {
	const sub = "3zsyy-cnoqf-tvlun-ymf55-tkpca-ox7uw-kfxoh-7khwq-2gz43-wafem-lqe"
	nodes := []NodeRes{
		{Name: "a", ID: "aaaaa-aa", Label: "A", Subnet: sub}, // declared on sub, live -> in sync
		{Name: "b", ID: "bbbbb-bb", Label: "B", Subnet: sub}, // declared on sub, not live -> missing
		{Name: "c", ID: "ccccc-cc", Label: "C"},              // declared, no subnet -> ignored for this subnet
	}
	subnets := []Subnet{{Name: "swiss", ID: sub, Label: "Swiss"}}
	live := []string{"aaaaa-aa", "ddddd-dd"} // ddddd not declared -> unmanaged

	rc := Reconcile(res(nodes, subnets), sub, live, nil)

	if got := rowFor(rc, "aaaaa-aa"); got == nil || got.Status != ReconcileInSync {
		t.Errorf("aaaaa-aa: got %+v, want in-sync", got)
	}
	if got := rowFor(rc, "bbbbb-bb"); got == nil || got.Status != ReconcileMissing {
		t.Errorf("bbbbb-bb: got %+v, want missing", got)
	}
	if got := rowFor(rc, "ddddd-dd"); got == nil || got.Status != ReconcileUnmanaged {
		t.Errorf("ddddd-dd: got %+v, want unmanaged", got)
	}
	if got := rowFor(rc, "ccccc-cc"); got != nil {
		t.Errorf("ccccc-cc declares no subnet; should not appear: %+v", got)
	}
	if !rc.HasDrift() {
		t.Error("expected drift (missing + unmanaged present)")
	}
}

func TestReconcileDeregistered(t *testing.T) {
	const sub = "3zsyy-cnoqf-tvlun-ymf55-tkpca-ox7uw-kfxoh-7khwq-2gz43-wafem-lqe"
	nodes := []NodeRes{
		{Name: "live", ID: "aaaaa-aa", Label: "", Subnet: sub},
		{Name: "gone", ID: "bbbbb-bb", Label: "", Subnet: sub}, // declared on sub, not live, and deregistered on-chain
	}
	subnets := []Subnet{{Name: "swiss", ID: sub, Label: ""}}
	status := map[string]NodeStatus{
		"aaaaa-aa": {Registered: true},
		"bbbbb-bb": {Registered: false},
	}
	rc := Reconcile(res(nodes, subnets), sub, []string{"aaaaa-aa"}, status)

	if got := rowFor(rc, "bbbbb-bb"); got == nil || got.Status != ReconcileDeregistered {
		t.Errorf("bbbbb-bb: got %+v, want deregistered", got)
	}
	if got := rowFor(rc, "aaaaa-aa"); got == nil || got.Status != ReconcileInSync {
		t.Errorf("aaaaa-aa: got %+v, want in-sync", got)
	}
}

func TestReconcileInSyncNoDrift(t *testing.T) {
	const sub = "3zsyy-cnoqf-tvlun-ymf55-tkpca-ox7uw-kfxoh-7khwq-2gz43-wafem-lqe"
	nodes := []NodeRes{{Name: "a", ID: "aaaaa-aa", Label: "", Subnet: sub}}
	rc := Reconcile(res(nodes, []Subnet{{Name: "swiss", ID: sub, Label: ""}}), sub, []string{"aaaaa-aa"}, nil)
	if rc.HasDrift() {
		t.Errorf("expected no drift, got %+v", rc.Rows)
	}
}
