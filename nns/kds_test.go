package nns

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// The VCEK AMD served for testChipA, captured from
// kdsintf.amd.com/vcek/v1/Milan/<testChipA>. Its hwID extension (OID
// 1.3.6.1.4.1.3704.1.4) is testChipA verbatim.
func testVCEK(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "vcek_milan.der"))
	if err != nil {
		t.Fatalf("read vcek fixture: %v", err)
	}
	return b
}

func TestVCEKHwIDMatchesChip(t *testing.T) {
	got, err := vcekHwID(testVCEK(t))
	if err != nil {
		t.Fatalf("extract hwID: %v", err)
	}
	if got != testChipA {
		t.Errorf("hwID = %q, want %q", got, testChipA)
	}
}

func TestVCEKProductName(t *testing.T) {
	got, err := vcekProduct(testVCEK(t))
	if err != nil {
		t.Fatalf("extract product: %v", err)
	}
	if got != "Milan-B0" {
		t.Errorf("product = %q, want Milan-B0", got)
	}
}

// A cert whose hwID does not match the chip we asked about must not count as
// verified: that is the whole assertion.
func TestVerifyChipRejectsHwIDMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(testVCEK(t))
	}))
	defer srv.Close()

	got := VerifyChip(srv.URL, testChipB)
	if got.Verified {
		t.Error("a cert for a different chip must not verify")
	}
	if got.Err == "" {
		t.Error("expected an error explaining the mismatch")
	}
}

func TestVerifyChipAcceptsMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(testVCEK(t))
	}))
	defer srv.Close()

	got := VerifyChip(srv.URL, testChipA)
	if !got.Verified {
		t.Fatalf("expected verified, got %+v", got)
	}
	if got.Product != "Milan-B0" {
		t.Errorf("product = %q, want Milan-B0", got.Product)
	}
}

// KDS answers 404 for the wrong product line, so an unknown chip is reported
// unverified rather than as a hard failure.
func TestVerifyChipNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	got := VerifyChip(srv.URL, testChipA)
	if got.Verified {
		t.Error("a 404 must not verify")
	}
	if got.Err == "" {
		t.Error("expected an error noting the chip was not found")
	}
}

// A rate-limited lookup is explicitly inconclusive: it must not read as a chip
// AMD refuses to vouch for.
func TestVerifyChipRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	got := VerifyChip(srv.URL, testChipA)
	if got.Verified {
		t.Error("a 429 must not verify")
	}
	if !got.Inconclusive {
		t.Error("a 429 is inconclusive, not a failed verification")
	}
}
