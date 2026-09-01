package nns

import (
	"fmt"
	"net/url"

	"github.com/aviate-labs/agent-go"
	"github.com/aviate-labs/agent-go/principal"
	registrypb "github.com/swiss-subnet/alpage/nns/pb/registry"
	transport "github.com/swiss-subnet/alpage/nns/pb/registrytransport"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// The dry-run registry used to be seeded with fabricated records, which is
// enough to satisfy the init invariants but not to execute a real payload: a
// node id is the self-authenticating principal of its signing key, so the ids a
// proposal names cannot be invented. Mirroring copies the real records out of
// mainnet verbatim, so change_subnet_membership and friends resolve the same
// entities locally that they will resolve on mainnet.
//
// The mirror is scoped: only what the payload dereferences, plus the closure
// the registry canister walks from there. A full registry copy would be truer
// still but does not fit in an install argument.

// nodeKeyPurposes are the four crypto_record_ purposes every registered node
// carries. The node invariants require all four, so a mirrored node needs all
// four copied.
var nodeKeyPurposes = []int{
	keyPurposeNodeSigning,
	keyPurposeDkgDealingEncryption,
	keyPurposeCommitteeSigning,
	keyPurposeIDkgMEGaEncryption,
}

// MirrorScope is the mainnet state an action's dry-run needs to be real. Only
// the roots are listed: buildMirror walks from them to the records the registry
// dereferences (subnet -> members -> operator -> data center).
type MirrorScope struct {
	Subnets   []principal.Principal
	Nodes     []principal.Principal
	Operators []principal.Principal
	Versions  []string
}

func (s MirrorScope) Empty() bool {
	return len(s.Subnets) == 0 && len(s.Nodes) == 0 && len(s.Operators) == 0 && len(s.Versions) == 0
}

// The registry's init invariants demand at least one system subnet, and the
// subnets a proposal targets are application subnets. Mirroring the real NNS
// subnet instead would mean copying its whole fleet's crypto material for a
// record nothing in the payload reads, so it is seeded with fabricated
// membership under its real id.
const dryRunSystemSubnet = "tdb26-jop6k-aogll-7ltgs-eruif-6kk7m-qpktf-gdiqx-mxtrf-vb5e6-eqe"

// SystemSubnetSeed is the fabricated system subnet that accompanies any mirror.
func SystemSubnetSeed() (SubnetSeed, error) {
	// Decode, not MustDecode: a bad constant must not panic the CLI mid-apply.
	id, err := principal.Decode(dryRunSystemSubnet)
	if err != nil {
		return SubnetSeed{}, fmt.Errorf("decode system subnet id: %w", err)
	}
	return SubnetSeed{SubnetID: id, NumNodes: 1, ReplicaVersion: dryRunSeedVersion}, nil
}

// MirrorScoper is an Action whose dry-run needs real registry records rather
// than fabricated ones. Kinds that implement it get a mainnet-shaped registry;
// kinds that do not keep the bare seeded NNS.
type MirrorScoper interface {
	MirrorScope() MirrorScope
}

// Mirror is the mirrored registry content plus the two aggregates the
// seed has to merge into rather than overwrite: subnet_list and the blessed
// version list are single keys shared with the fabricated system subnet.
type Mirror struct {
	mutations []registryMutation
	subnets   []principal.Principal
	versions  []string
}

// registryValues reads raw registry records by key. ok is false when the key is
// simply absent, which the closure treats differently per key: a missing subnet
// is fatal, a missing optional record is skipped.
type registryValues interface {
	value(key string) ([]byte, bool, error)
}

// mainnetRegistry reads records straight out of the registry canister's
// protobuf get_value endpoint, which returns the stored bytes verbatim.
//
// The query is not certified: get_value has no certified counterpart, and a
// forged record could only weaken the dry-run, never cause a submission --
// preflight still reads mainnet through the certified path, and the operator
// still confirms before anything is submitted.
// Every read is pinned to version, taken once up front: the mirror walks
// dozens of keys, and reading each at "latest" would let a mainnet mutation
// land mid-walk and produce a torn snapshot (a subnet record listing a node
// whose record the next read no longer finds).
type mainnetRegistry struct {
	a       agent.Agent
	version uint64
}

func newMainnetRegistry(host string, fetchRootKey bool) (*mainnetRegistry, error) {
	if host == "" {
		host = MainnetHost
	}
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("parse host %q: %w", host, err)
	}
	a, err := agent.New(agent.Config{
		ClientConfig: clientOptions(u),
		FetchRootKey: fetchRootKey,
	})
	if err != nil {
		return nil, fmt.Errorf("new registry agent: %w", err)
	}
	// get_latest_version takes no argument, so the request is empty bytes; only
	// the response is a protobuf.
	raw, err := a.QueryRaw(RegistryID, "get_latest_version", []byte{})
	if err != nil {
		return nil, fmt.Errorf("get_latest_version: %w", err)
	}
	var latest transport.RegistryGetLatestVersionResponse
	if err := proto.Unmarshal(raw, &latest); err != nil {
		return nil, fmt.Errorf("decode get_latest_version: %w", err)
	}
	return &mainnetRegistry{a: *a, version: latest.GetVersion()}, nil
}

func (m *mainnetRegistry) value(key string) ([]byte, bool, error) {
	req := &transport.RegistryGetValueRequest{
		Key:     []byte(key),
		Version: wrapperspb.UInt64(m.version),
	}
	var resp transport.HighCapacityRegistryGetValueResponse
	if err := m.a.QueryProto(RegistryID, "get_value", req, &resp); err != nil {
		return nil, false, fmt.Errorf("get_value %s: %w", key, err)
	}
	if e := resp.GetError(); e != nil {
		if e.GetCode() == transport.RegistryError_KEY_NOT_PRESENT {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("get_value %s: registry error %s: %s", key, e.GetCode(), e.GetReason())
	}
	// A chunked value means the record outgrew a single response and must be
	// reassembled via get_chunk. No record the mirror reads is that large today.
	if resp.GetLargeValueChunkKeys() != nil {
		return nil, false, fmt.Errorf("get_value %s: value is chunked, which the mirror cannot reassemble", key)
	}
	return resp.GetValue(), true, nil
}

// FetchMirror copies the registry records in scope out of a live IC, ready to
// be planted in the dry-run registry.
func FetchMirror(host string, fetchRootKey bool, scope MirrorScope) (*Mirror, error) {
	r, err := newMainnetRegistry(host, fetchRootKey)
	if err != nil {
		return nil, err
	}
	return buildMirror(r, scope)
}

// mirrorBuilder accumulates mutations while walking the closure, deduplicating
// by key: the same operator backs many nodes, and one query per node would
// multiply the mainnet round trips by the fleet size.
type mirrorBuilder struct {
	src  registryValues
	seen map[string]bool
	out  Mirror
}

// copyKey mirrors one key. required distinguishes a record whose absence means
// the payload names something mainnet does not have (fatal) from one that is
// legitimately optional. Returns the raw value, or nil when absent.
func (b *mirrorBuilder) copyKey(key string, required bool) ([]byte, error) {
	if b.seen[key] {
		return nil, nil
	}
	v, ok, err := b.src.value(key)
	if err != nil {
		return nil, err
	}
	if !ok {
		if required {
			return nil, fmt.Errorf("registry has no %s: the payload names an entity mainnet does not have", key)
		}
		return nil, nil
	}
	b.seen[key] = true
	b.out.mutations = append(b.out.mutations, registryMutation{
		Key:          []byte(key),
		MutationType: mutationInsert,
		Value:        v,
	})
	return v, nil
}

// buildMirror walks the scope's roots down to a closed record set. Order
// matters only for discovery: the mutations land in one atomic init batch, and
// the registry checks its invariants once, after the whole batch is applied.
func buildMirror(src registryValues, scope MirrorScope) (*Mirror, error) {
	b := &mirrorBuilder{src: src, seen: map[string]bool{}}

	nodes := append([]principal.Principal(nil), scope.Nodes...)
	versions := append([]string(nil), scope.Versions...)
	operators := append([]principal.Principal(nil), scope.Operators...)

	for _, sn := range scope.Subnets {
		id := sn.Encode()
		// subnets feeds the merged subnet_list, one key, so a subnet the scope
		// names twice has to be walked and listed once.
		if b.seen["subnet_record_"+id] {
			continue
		}
		blob, err := b.copyKey("subnet_record_"+id, true)
		if err != nil {
			return nil, err
		}
		// The threshold key and the CUP are cross-checked against each other by
		// the crypto invariant, so both are mirrored or the pair is inconsistent.
		if _, err := b.copyKey("crypto_threshold_signing_public_key_"+id, true); err != nil {
			return nil, err
		}
		if _, err := b.copyKey("catch_up_package_contents_"+id, true); err != nil {
			return nil, err
		}
		var rec registrypb.SubnetRecord
		if err := proto.Unmarshal(blob, &rec); err != nil {
			return nil, fmt.Errorf("decode subnet_record %s: %w", id, err)
		}
		for _, m := range rec.GetMembership() {
			nodes = append(nodes, principal.Principal{Raw: m})
		}
		if v := rec.GetReplicaVersionId(); v != "" {
			versions = append(versions, v)
		}
		b.out.subnets = append(b.out.subnets, sn)
	}

	for _, n := range nodes {
		op, err := b.copyNode(n)
		if err != nil {
			return nil, err
		}
		if op != nil {
			operators = append(operators, *op)
		}
	}

	for _, op := range operators {
		if err := b.copyOperator(op); err != nil {
			return nil, err
		}
	}

	for _, v := range versions {
		// Same single-key merge for blessed_replica_versions: a deploy_guestos
		// targeting the version its subnet already runs names it twice.
		if b.seen["replica_version_"+v] {
			continue
		}
		if _, err := b.copyKey("replica_version_"+v, true); err != nil {
			return nil, err
		}
		b.out.versions = append(b.out.versions, v)
	}

	return &b.out, nil
}

// copyNode mirrors a node record with the crypto material the node invariants
// require, and reports the operator that owns it so the operator and its data
// center come along.
func (b *mirrorBuilder) copyNode(n principal.Principal) (*principal.Principal, error) {
	id := n.Encode()
	blob, err := b.copyKey("node_record_"+id, true)
	if err != nil {
		return nil, err
	}
	if blob == nil { // already mirrored via another root
		return nil, nil
	}
	if _, err := b.copyKey("crypto_tls_cert_"+id, true); err != nil {
		return nil, err
	}
	for _, purpose := range nodeKeyPurposes {
		if _, err := b.copyKey(fmt.Sprintf("crypto_record_%s_%d", id, purpose), true); err != nil {
			return nil, err
		}
	}
	var rec registrypb.NodeRecord
	if err := proto.Unmarshal(blob, &rec); err != nil {
		return nil, fmt.Errorf("decode node_record %s: %w", id, err)
	}
	// A node pinned to a HostOS version drags in that version's record: the
	// hostos invariant panics on a version any node references but the registry
	// does not have.
	if hv := rec.GetHostosVersionId(); hv != "" {
		if _, err := b.copyKey("hostos_version_"+hv, true); err != nil {
			return nil, err
		}
	}
	raw := rec.GetNodeOperatorId()
	if len(raw) == 0 {
		return nil, nil
	}
	op := principal.Principal{Raw: raw}
	return &op, nil
}

// copyOperator mirrors an operator record and the data center it names. The
// node-operator invariant reads both, so an operator without its DC fails init.
func (b *mirrorBuilder) copyOperator(op principal.Principal) error {
	blob, err := b.copyKey("node_operator_record_"+op.Encode(), true)
	if err != nil || blob == nil {
		return err
	}
	var rec registrypb.NodeOperatorRecord
	if err := proto.Unmarshal(blob, &rec); err != nil {
		return fmt.Errorf("decode node_operator_record %s: %w", op.Encode(), err)
	}
	if dc := rec.GetDcId(); dc != "" {
		if _, err := b.copyKey("data_center_record_"+dc, true); err != nil {
			return err
		}
	}
	return nil
}
