package nns

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/aviate-labs/agent-go/principal"
	registrypb "github.com/swiss-subnet/alpage/nns/pb/registry"
	"google.golang.org/protobuf/proto"
)

// fakeRegistry serves raw records by key, the way the mainnet registry's
// get_value does, and counts reads so the closure cannot silently refetch.
type fakeRegistry struct {
	values map[string][]byte
	reads  map[string]int
}

func (f *fakeRegistry) value(key string) ([]byte, bool, error) {
	if f.reads == nil {
		f.reads = map[string]int{}
	}
	f.reads[key]++
	v, ok := f.values[key]
	return v, ok, nil
}

func mustMarshal(t *testing.T, m proto.Message) []byte {
	t.Helper()
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// mirrorFixture is a subnet of one member node, owned by one operator in one
// data center, plus a second node that is registered but not yet a member --
// the shape a membership swap proposal targets.
func mirrorFixture(t *testing.T) (*fakeRegistry, principal.Principal, principal.Principal, principal.Principal) {
	t.Helper()
	subnet := principal.MustDecode(dryRunSeedSubnet)
	member := principal.MustDecode("ugktj-anayx-3xzbp-hznv5-aznxn-5nodb-4ahcq-7og6o-hzazt-7ziym-zae")
	incoming := principal.MustDecode("uam63-cu4m2-p4uyy-rhhns-rnwtt-dajey-xaipy-i347a-dagxe-sx2jj-pae")
	op := principal.MustDecode(dryRunSeedOperator)
	prov := principal.MustDecode("bvcsg-3od6r-jnydw-eysln-aql7w-td5zn-ay5m6-sibd2-jzojt-anwag-mqe")

	f := &fakeRegistry{values: map[string][]byte{
		"subnet_record_" + subnet.Encode(): mustMarshal(t, &registrypb.SubnetRecord{
			Membership:       [][]byte{member.Raw},
			ReplicaVersionId: "deadbeef",
		}),
		"crypto_threshold_signing_public_key_" + subnet.Encode(): []byte("threshold"),
		"catch_up_package_contents_" + subnet.Encode():           []byte("cup"),
		"node_operator_record_" + op.Encode(): mustMarshal(t, &registrypb.NodeOperatorRecord{
			NodeOperatorPrincipalId: op.Raw,
			NodeProviderPrincipalId: prov.Raw,
			DcId:                    "so1",
		}),
		"data_center_record_so1":   mustMarshal(t, &registrypb.DataCenterRecord{Id: "so1", Region: "Europe,CH,Solothurn"}),
		"replica_version_deadbeef": []byte("version"),
	}}
	for _, n := range []principal.Principal{member, incoming} {
		f.values["node_record_"+n.Encode()] = mustMarshal(t, &registrypb.NodeRecord{NodeOperatorId: op.Raw})
		f.values["crypto_tls_cert_"+n.Encode()] = []byte("cert")
		for _, purpose := range nodeKeyPurposes {
			f.values[fmt.Sprintf("crypto_record_%s_%d", n.Encode(), purpose)] = []byte("key")
		}
	}
	return f, subnet, member, incoming
}

// A subnet in scope must drag in everything the registry canister dereferences
// when it executes a membership change: the members' node records and crypto
// material, and the operator and data center behind them. Mirroring the subnet
// record alone trips the registry's invariants at init.
func TestMirrorSubnetPullsInItsClosure(t *testing.T) {
	f, subnet, member, incoming := mirrorFixture(t)

	got, err := buildMirror(f, MirrorScope{
		Subnets: []principal.Principal{subnet},
		Nodes:   []principal.Principal{incoming},
	})
	if err != nil {
		t.Fatalf("buildMirror: %v", err)
	}

	keys := map[string]bool{}
	for _, m := range got.mutations {
		keys[string(m.Key)] = true
	}
	want := []string{
		"subnet_record_" + subnet.Encode(),
		"crypto_threshold_signing_public_key_" + subnet.Encode(),
		"catch_up_package_contents_" + subnet.Encode(),
		"node_record_" + member.Encode(),
		"node_record_" + incoming.Encode(),
		"crypto_tls_cert_" + member.Encode(),
		"crypto_record_" + member.Encode() + "_1",
		"node_operator_record_" + dryRunSeedOperator,
		"data_center_record_so1",
		"replica_version_deadbeef",
	}
	for _, k := range want {
		if !keys[k] {
			t.Errorf("missing mirrored key %q", k)
		}
	}
	if len(got.subnets) != 1 || got.subnets[0].Encode() != subnet.Encode() {
		t.Errorf("subnets = %v, want just the mirrored subnet", got.subnets)
	}
	if len(got.versions) != 1 || got.versions[0] != "deadbeef" {
		t.Errorf("versions = %v, want [deadbeef]", got.versions)
	}
}

// Both nodes share an operator, and the incoming node is reachable both from
// scope and (once added) from membership. Re-reading a key would multiply the
// mainnet queries by the fleet size and could emit duplicate INSERTs, which the
// registry rejects.
func TestMirrorReadsEachKeyOnce(t *testing.T) {
	f, subnet, _, incoming := mirrorFixture(t)

	got, err := buildMirror(f, MirrorScope{
		Subnets: []principal.Principal{subnet},
		Nodes:   []principal.Principal{incoming, incoming},
	})
	if err != nil {
		t.Fatalf("buildMirror: %v", err)
	}
	for key, n := range f.reads {
		if n > 1 {
			t.Errorf("key %q read %d times, want 1", key, n)
		}
	}
	seen := map[string]bool{}
	for _, m := range got.mutations {
		if seen[string(m.Key)] {
			t.Errorf("duplicate mutation for key %q", m.Key)
		}
		seen[string(m.Key)] = true
	}
}

// Every kind whose payload dereferences registry entities must declare them.
// A kind that silently declares nothing dry-runs against an empty registry and
// its execution failure only shows up after the vote, which is the bug the
// mirror exists to remove.
func TestActionKindsDeclareTheirMirrorScope(t *testing.T) {
	subnet := principal.MustDecode(dryRunSeedSubnet)
	node := principal.MustDecode("uam63-cu4m2-p4uyy-rhhns-rnwtt-dajey-xaipy-i347a-dagxe-sx2jj-pae")
	op := principal.MustDecode(dryRunSeedOperator)

	for _, tc := range []struct {
		name   string
		action Action
		want   MirrorScope
	}{
		{
			"membership",
			MembershipProposal{SubnetID: subnet, NodeIDsAdd: []principal.Principal{node}},
			MirrorScope{Subnets: []principal.Principal{subnet}, Nodes: []principal.Principal{node}},
		},
		{
			"deploy_guestos",
			DeployGuestosAction{SubnetID: subnet, ReplicaVersionID: "deadbeef"},
			MirrorScope{Subnets: []principal.Principal{subnet}, Versions: []string{"deadbeef"}},
		},
		{
			"remove_nodes",
			RemoveNodesAction{NodeIDs: []principal.Principal{node}},
			MirrorScope{Nodes: []principal.Principal{node}},
		},
		{
			"remove_node_operators",
			RemoveNodeOperatorsAction{OperatorIDs: []principal.Principal{op}},
			MirrorScope{Operators: []principal.Principal{op}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scoper, ok := tc.action.(MirrorScoper)
			if !ok {
				t.Fatalf("%T does not implement MirrorScoper", tc.action)
			}
			got := scoper.MirrorScope()
			if got.Empty() {
				t.Fatal("scope is empty, so the dry-run would run against fabricated state")
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("scope = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// subnet_list and blessed_replica_versions are single registry keys the mirror
// merges into, so naming the same subnet or version twice must not produce two
// entries. A deploy_guestos targeting the version its subnet already runs is
// exactly that scope.
func TestMirrorAggregatesAreDeduplicated(t *testing.T) {
	f, subnet, _, _ := mirrorFixture(t)

	got, err := buildMirror(f, MirrorScope{
		Subnets:  []principal.Principal{subnet, subnet},
		Versions: []string{"deadbeef"},
	})
	if err != nil {
		t.Fatalf("buildMirror: %v", err)
	}
	if len(got.subnets) != 1 {
		t.Errorf("subnets = %v, want the subnet once", got.subnets)
	}
	if !reflect.DeepEqual(got.versions, []string{"deadbeef"}) {
		t.Errorf("versions = %v, want [deadbeef]", got.versions)
	}
}

// The fabricated seeds and the mirror are built independently, so the same
// subnet or version can come from both: every mirror rides along with the
// seeded system subnet, and a proposal may target that subnet. The merged keys
// must still name each once, and the mirrored record must not be re-seeded --
// two INSERTs on one key are rejected.
func TestSeedMergesMirroredAggregatesOnce(t *testing.T) {
	subnet := principal.MustDecode(dryRunSeedSubnet)
	seeds := []SubnetSeed{{SubnetID: subnet, NumNodes: 1, ReplicaVersion: "deadbeef"}}

	muts, _, err := seedMutations(seeds, []principal.Principal{subnet}, []string{"deadbeef"})
	if err != nil {
		t.Fatalf("seedMutations: %v", err)
	}
	byKey := map[string][]byte{}
	for _, m := range muts {
		if _, dup := byKey[string(m.Key)]; dup {
			t.Errorf("duplicate mutation for key %q", m.Key)
		}
		byKey[string(m.Key)] = m.Value
	}
	if _, ok := byKey["replica_version_deadbeef"]; ok {
		t.Error("seeded a replica_version record the mirror supplies")
	}

	var list registrypb.SubnetListRecord
	if err := proto.Unmarshal(byKey["subnet_list"], &list); err != nil {
		t.Fatalf("unmarshal subnet_list: %v", err)
	}
	if len(list.Subnets) != 1 {
		t.Errorf("subnet_list has %d entries, want 1", len(list.Subnets))
	}
	// Election is now the replica_version_ record's existence, and the mirror
	// supplies that one. The registry dropped the separate list, so seeding it
	// would write a key the canister no longer knows.
	if _, ok := byKey["blessed_replica_versions"]; ok {
		t.Error("seeded blessed_replica_versions, which the registry no longer has")
	}
}

// A record the payload names but mainnet does not have is the failure the
// dry-run exists to catch. It must surface as an error naming the key, not as a
// silently thinner mirror that lets the proposal appear to execute.
func TestMirrorMissingSubnetIsAnError(t *testing.T) {
	f, _, _, _ := mirrorFixture(t)
	ghost := principal.MustDecode("tdb26-jop6k-aogll-7ltgs-eruif-6kk7m-qpktf-gdiqx-mxtrf-vb5e6-eqe")

	_, err := buildMirror(f, MirrorScope{Subnets: []principal.Principal{ghost}})
	if err == nil {
		t.Fatal("want an error for a subnet mainnet does not have")
	}
	if !strings.Contains(err.Error(), ghost.Encode()) {
		t.Errorf("error %q should name the missing subnet", err)
	}
}
