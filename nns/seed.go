package nns

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"math/big"

	"github.com/aviate-labs/agent-go/principal"
	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/fxamacker/cbor/v2"
	"github.com/swiss-subnet/alpage/nns/registrypb"
	"google.golang.org/protobuf/proto"
)

// KeyPurpose registry ids (rs/types crypto.rs).
const (
	keyPurposeNodeSigning          = 1
	keyPurposeDkgDealingEncryption = 3
	keyPurposeCommitteeSigning     = 4
	keyPurposeIDkgMEGaEncryption   = 5
)

// registry mutation type: keys are fresh at init, so INSERT (see the registry
// transport proto: INSERT=0, UPDATE=1, DELETE=2, UPSERT=4).
const mutationInsert int32 = 0

// SubnetSeed is a subnet to plant in a freshly-initialized local registry so
// get_subnet returns real membership and replica version for tests. Members are
// not supplied: node ids must equal the self-authenticating principal of a node
// signing key (the registry's node-id invariant), so the seed generates NumNodes
// nodes with valid keys and reports the resulting member principals.
type SubnetSeed struct {
	SubnetID       principal.Principal
	NumNodes       int
	ReplicaVersion string
}

// ProviderSeed plants a node provider with one operator in one data center, so
// get_node_operators_and_dcs_of_node_provider resolves for tests. Ids are
// textual principals / dc id strings.
type ProviderSeed struct {
	ProviderID string
	OperatorID string
	DcID       string
	DcRegion   string
}

// seedProviderMutations renders node_operator_record_ and data_center_record_
// entries for each provider seed. The query joins an operator's
// node_provider_principal_id and dc_id to the requested provider and its DC
// record, so both records must be present and consistent.
func seedProviderMutations(seeds []ProviderSeed) ([]registryMutation, error) {
	var muts []registryMutation
	for _, s := range seeds {
		provID, err := principal.Decode(s.ProviderID)
		if err != nil {
			return nil, fmt.Errorf("provider seed %q: %w", s.ProviderID, err)
		}
		opID, err := principal.Decode(s.OperatorID)
		if err != nil {
			return nil, fmt.Errorf("operator seed %q: %w", s.OperatorID, err)
		}
		opRec, err := proto.Marshal(&registrypb.NodeOperatorRecord{
			NodeOperatorPrincipalId: opID.Raw,
			NodeProviderPrincipalId: provID.Raw,
			DcId:                    s.DcID,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal node_operator_record %s: %w", s.OperatorID, err)
		}
		muts = append(muts, registryMutation{
			Key:          []byte("node_operator_record_" + s.OperatorID),
			MutationType: mutationInsert,
			Value:        opRec,
		})
		dcRec, err := proto.Marshal(&registrypb.DataCenterRecord{Id: s.DcID, Region: s.DcRegion})
		if err != nil {
			return nil, fmt.Errorf("marshal data_center_record %s: %w", s.DcID, err)
		}
		muts = append(muts, registryMutation{
			Key:          []byte("data_center_record_" + s.DcID),
			MutationType: mutationInsert,
			Value:        dcRec,
		})
	}
	return muts, nil
}

// seedMutations renders the full record set that makes each seeded subnet
// resolvable via get_subnet AND passes the registry's init invariant battery.
// The battery (crypto/CUP, elected replica version, "at least one system
// subnet") only fires when init carries mutations, and it iterates subnet_list.
// So a compliant seed needs, for the whole set: subnet_list, an elected+blessed
// replica version, and for one subnet marked System a consistent
// threshold-pubkey + CUP pair (see seedCryptoRecords).
func seedMutations(seeds []SubnetSeed) ([]registryMutation, map[string][]principal.Principal, error) {
	if len(seeds) == 0 {
		return nil, nil, nil
	}
	var muts []registryMutation
	var list registrypb.SubnetListRecord
	versions := map[string]bool{}
	members := map[string][]principal.Principal{}
	keySalt := 0

	for i, s := range seeds {
		nodeMuts, nodes, err := seedNodes(s.NumNodes, &keySalt)
		if err != nil {
			return nil, nil, err
		}
		muts = append(muts, nodeMuts...)

		membership := make([][]byte, len(nodes))
		for j, n := range nodes {
			membership[j] = n.Raw
		}
		members[s.SubnetID.Encode()] = nodes

		// The first subnet is the system subnet so the "at least one system
		// subnet" invariant holds; the rest are application subnets.
		subnetType := registrypb.SubnetType_SUBNET_TYPE_APPLICATION
		if i == 0 {
			subnetType = registrypb.SubnetType_SUBNET_TYPE_SYSTEM
		}
		rec, err := proto.Marshal(&registrypb.SubnetRecord{
			Membership:       membership,
			ReplicaVersionId: s.ReplicaVersion,
			SubnetType:       subnetType,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("marshal subnet_record %s: %w", s.SubnetID.Encode(), err)
		}
		muts = append(muts, registryMutation{
			Key:          []byte("subnet_record_" + s.SubnetID.Encode()),
			MutationType: mutationInsert,
			Value:        rec,
		})
		list.Subnets = append(list.Subnets, s.SubnetID.Raw)
		versions[s.ReplicaVersion] = true

		crypto, err := seedCryptoRecords(s.SubnetID)
		if err != nil {
			return nil, nil, err
		}
		muts = append(muts, crypto...)
	}

	listBlob, err := proto.Marshal(&list)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal subnet_list: %w", err)
	}
	muts = append(muts, registryMutation{
		Key:          []byte("subnet_list"),
		MutationType: mutationInsert,
		Value:        listBlob,
	})

	verMuts, err := seedReplicaVersions(versions)
	if err != nil {
		return nil, nil, err
	}
	return append(muts, verMuts...), members, nil
}

// seedNodes generates n nodes with the crypto records the node invariants
// require: four unique public keys per node (node id = self-authenticating
// principal of the NodeSigning key's DER, so we derive it, not choose it), a
// unique TLS cert, and a node_record with syntactically valid endpoints.
// keySalt makes every key/cert byte-string unique across nodes and subnets, as
// the uniqueness invariant demands.
func seedNodes(n int, keySalt *int) ([]registryMutation, []principal.Principal, error) {
	var muts []registryMutation
	var ids []principal.Principal
	for i := 0; i < n; i++ {
		signingRaw, signingDER, err := newEd25519Key()
		if err != nil {
			return nil, nil, err
		}
		// node id = self-authenticating principal of the DER-wrapped signing key
		// (what derive_node_id reconstructs), but the record stores the raw key.
		nodeID := principal.NewSelfAuthenticating(signingDER)
		ids = append(ids, nodeID)

		for _, kp := range []struct {
			purpose int
			value   []byte
		}{
			{keyPurposeNodeSigning, signingRaw},
			{keyPurposeCommitteeSigning, uniqueBytes(keySalt)},
			{keyPurposeDkgDealingEncryption, uniqueBytes(keySalt)},
			{keyPurposeIDkgMEGaEncryption, uniqueBytes(keySalt)},
		} {
			pk, err := proto.Marshal(&registrypb.PublicKey{KeyValue: kp.value})
			if err != nil {
				return nil, nil, fmt.Errorf("marshal node key: %w", err)
			}
			muts = append(muts, registryMutation{
				Key:          []byte(fmt.Sprintf("crypto_record_%s_%d", nodeID.Encode(), kp.purpose)),
				MutationType: mutationInsert,
				Value:        pk,
			})
		}

		cert, err := proto.Marshal(&registrypb.X509PublicKeyCert{CertificateDer: uniqueBytes(keySalt)})
		if err != nil {
			return nil, nil, fmt.Errorf("marshal tls cert: %w", err)
		}
		muts = append(muts, registryMutation{
			Key:          []byte("crypto_tls_cert_" + nodeID.Encode()),
			MutationType: mutationInsert,
			Value:        cert,
		})

		// node_operator_id left empty so the node-operator invariant is skipped.
		// Endpoints must be unique across nodes: vary the port by salt.
		*keySalt++
		port := uint32(4100 + *keySalt)
		rec, err := proto.Marshal(&registrypb.NodeRecord{
			Xnet: &registrypb.ConnectionEndpoint{IpAddr: "127.0.0.1", Port: port},
			Http: &registrypb.ConnectionEndpoint{IpAddr: "127.0.0.1", Port: port + 10000},
		})
		if err != nil {
			return nil, nil, fmt.Errorf("marshal node_record: %w", err)
		}
		muts = append(muts, registryMutation{
			Key:          []byte("node_record_" + nodeID.Encode()),
			MutationType: mutationInsert,
			Value:        rec,
		})
	}
	return muts, ids, nil
}

// newEd25519Key returns a fresh ed25519 public key both as raw 32 bytes (what
// the registry stores in the NodeSigning record) and DER/SPKI-wrapped (what
// derive_node_id self-authenticates into the node id).
func newEd25519Key() (raw, der []byte, err error) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	der, err = asn1.Marshal(struct {
		Algo      pkix.AlgorithmIdentifier
		PublicKey asn1.BitString
	}{
		Algo:      pkix.AlgorithmIdentifier{Algorithm: asn1.ObjectIdentifier{1, 3, 101, 112}},
		PublicKey: asn1.BitString{Bytes: pub, BitLength: len(pub) * 8},
	})
	return pub, der, err
}

// uniqueBytes returns distinct bytes each call so per-node key/cert uniqueness
// holds without needing real key material where the invariant does not check it.
func uniqueBytes(salt *int) []byte {
	*salt++
	b := make([]byte, 8)
	for i := 0; i < 8; i++ {
		b[i] = byte(*salt >> (8 * i))
	}
	return append([]byte("seed-unique-"), b...)
}

// seedReplicaVersions elects every version in use (empty ReplicaVersionRecord,
// which the invariant accepts because release_package_urls is empty) and
// blesses them, satisfying check_replica_version_invariants.
func seedReplicaVersions(versions map[string]bool) ([]registryMutation, error) {
	var muts []registryMutation
	var blessed registrypb.BlessedReplicaVersions
	for v := range versions {
		rec, err := proto.Marshal(&registrypb.ReplicaVersionRecord{})
		if err != nil {
			return nil, fmt.Errorf("marshal replica_version %s: %w", v, err)
		}
		muts = append(muts, registryMutation{
			Key:          []byte("replica_version_" + v),
			MutationType: mutationInsert,
			Value:        rec,
		})
		blessed.BlessedVersionIds = append(blessed.BlessedVersionIds, v)
	}
	blob, err := proto.Marshal(&blessed)
	if err != nil {
		return nil, fmt.Errorf("marshal blessed_replica_versions: %w", err)
	}
	muts = append(muts, registryMutation{
		Key:          []byte("blessed_replica_versions"),
		MutationType: mutationInsert,
		Value:        blob,
	})
	return muts, nil
}

// seedCryptoRecords writes the threshold signing public key and CUP contents
// for a subnet such that the crypto invariant's byte-equality holds: the
// public key equals the first public coefficient in the CUP's high-threshold
// NI-DKG transcript. We author both, so we set them to the same real G2 point.
func seedCryptoRecords(subnetID principal.Principal) ([]registryMutation, error) {
	g2 := thresholdKeyBytes()

	pk, err := proto.Marshal(&registrypb.PublicKey{
		Version:   0,
		Algorithm: registrypb.AlgorithmId_ALGORITHM_ID_THRES_BLS12_381,
		KeyValue:  g2,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal threshold pubkey: %w", err)
	}

	transcript, err := cspNiDkgTranscriptCBOR(g2)
	if err != nil {
		return nil, err
	}
	cup, err := proto.Marshal(&registrypb.CatchUpPackageContents{
		InitialNiDkgTranscriptHighThreshold: &registrypb.InitialNiDkgTranscriptRecord{
			Threshold:             1,
			InternalCspTranscript: transcript,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal cup contents: %w", err)
	}

	return []registryMutation{
		{Key: []byte("crypto_threshold_signing_public_key_" + subnetID.Encode()), MutationType: mutationInsert, Value: pk},
		{Key: []byte("catch_up_package_contents_" + subnetID.Encode()), MutationType: mutationInsert, Value: cup},
	}, nil
}

// thresholdKeyBytes returns a real BLS12-381 G2 point in the 96-byte ZCash
// compressed form the IC uses. Derived from a fixed scalar so seeds are
// deterministic across runs.
func thresholdKeyBytes() []byte {
	var pk bls.G2Affine
	_, _, _, g2Gen := bls.Generators()
	pk.ScalarMultiplication(&g2Gen, big.NewInt(0x51a19e))
	b := pk.Bytes() // [96]byte compressed
	return b[:]
}

// cspNiDkgTranscriptCBOR builds the serde_cbor encoding of
// CspNiDkgTranscript::Groth20_Bls12_381(Transcript) that the registry's crypto
// invariant deserializes (serde_cbor::from_slice). The Rust types serialize as:
// externally-tagged enum -> {tag: value}; the struct -> a map keyed by field
// name; G2Bytes/PublicKeyBytes via derive_serde! -> a CBOR byte string. The
// invariant only reads coefficients[0], so one coefficient and empty
// receiver_data suffice.
func cspNiDkgTranscriptCBOR(coefficient []byte) ([]byte, error) {
	transcript := map[string]any{
		"public_coefficients": map[string]any{
			"coefficients": [][]byte{coefficient},
		},
		"receiver_data": map[uint32][]byte{},
	}
	blob, err := cbor.Marshal(map[string]any{"Groth20_Bls12_381": transcript})
	if err != nil {
		return nil, fmt.Errorf("cbor encode ni-dkg transcript: %w", err)
	}
	return blob, nil
}
