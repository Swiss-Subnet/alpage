package nns

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	registrypb "github.com/swiss-subnet/alpage/nns/pb/registry"
	"google.golang.org/protobuf/proto"
)

// nodeRecordStub serves the explorer's node record shape for the given chips,
// keyed by node id. An id absent from the map is served as unknown.
func nodeRecordStub(t *testing.T, chips map[string]string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/records/nodes/")
		chip, ok := chips[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprintf(w, `[{"key":"node_record_%s","version":10,"value":{"node_operator_id":"op","chip_id":%q}}]`, id, chip)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestSevAdmissionBlocksChiplessNode(t *testing.T) {
	base := nodeRecordStub(t, map[string]string{"n-chip": testChipA64, "n-bare": ""})
	a := checkSevAdmission(base, []string{"n-chip", "n-bare"})
	if len(a.Chipless) != 1 || a.Chipless[0] != "n-bare" {
		t.Fatalf("Chipless = %v; want just n-bare", a.Chipless)
	}
	pf := a.fold(Preflight{"subnet s\n", PreflightClean})
	if pf.Level != PreflightWarn {
		t.Errorf("level = %v; want Warn so apply shows the trap before the dry-run fails", pf.Level)
	}
	if !strings.Contains(pf.Report, "BLOCKS n-bare") {
		t.Errorf("report does not name the chipless node:\n%s", pf.Report)
	}
}

func TestSevAdmissionUndecidedWhenRecordUnread(t *testing.T) {
	// The explorer is a third party, so an unreadable record warns rather than
	// reporting a node as chipless.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	a := checkSevAdmission(srv.URL, []string{"n-1"})
	if len(a.Chipless) != 0 {
		t.Errorf("Chipless = %v; an unread record is not evidence of a missing chip", a.Chipless)
	}
	if len(a.Undecided) != 1 {
		t.Fatalf("Undecided = %v; want n-1", a.Undecided)
	}
	pf := a.fold(Preflight{"subnet s\n", PreflightClean})
	if pf.Level != PreflightWarn || !strings.Contains(pf.Report, "WARNING") {
		t.Errorf("want a warning, got %v:\n%s", pf.Level, pf.Report)
	}
}

func TestSevLaunchGateBlocksVersionWithoutMeasurements(t *testing.T) {
	pf := sevLaunchGate(Preflight{"replica version a -> b\n", PreflightClean}, "b", false, true)
	if pf.Level != PreflightWarn {
		t.Errorf("level = %v; want Warn", pf.Level)
	}
	if !strings.Contains(pf.Report, "BLOCKS b carries no guest launch measurements") {
		t.Errorf("report does not name the missing measurements:\n%s", pf.Report)
	}
}

func TestSevLaunchGateWarnsWhenMeasurementsUnread(t *testing.T) {
	pf := sevLaunchGate(Preflight{"replica version a -> b\n", PreflightClean}, "b", false, false)
	if pf.Level != PreflightWarn || strings.Contains(pf.Report, "BLOCKS") {
		t.Errorf("an unread record must warn rather than block:\n%s", pf.Report)
	}
}

func TestParseVersionRecordReadsLaunchMeasurements(t *testing.T) {
	measured := encodeVersionRecord(t, &registrypb.ReplicaVersionRecord{
		GuestLaunchMeasurements: &registrypb.GuestLaunchMeasurements{
			GuestLaunchMeasurements: []*registrypb.GuestLaunchMeasurement{{Measurement: []byte{1, 2, 3}}},
		},
	})
	got, err := parseVersionRecord([]byte(measured))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !got.Measured || !got.MeasuredKnown {
		t.Errorf("got %+v; want measurements read off the record", got)
	}

	bare := encodeVersionRecord(t, &registrypb.ReplicaVersionRecord{ReleasePackageSha256Hex: "abc"})
	got, err = parseVersionRecord([]byte(bare))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Measured || !got.MeasuredKnown {
		t.Errorf("got %+v; want a decoded record with no measurements", got)
	}
}

func encodeVersionRecord(t *testing.T, rec *registrypb.ReplicaVersionRecord) string {
	t.Helper()
	blob, err := proto.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return fmt.Sprintf(`[{"key":"replica_version_v","version":7,"value":%q}]`,
		base64.StdEncoding.EncodeToString(blob))
}
