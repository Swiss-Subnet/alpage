package nns

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sha256hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// tarOf builds a flat tar the way the IC release canisters.tar is laid out.
func tarOf(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func gzipOf(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(b); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// An explicit env var always wins: nix and CI must never be overridden by a
// download.
func TestResolveArtifactsPrefersEnv(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "pocket-ic")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"gov.wasm.gz", "reg.wasm.gz", "root.wasm.gz"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("POCKET_IC_BIN", bin)
	t.Setenv("GOVERNANCE_WASM", filepath.Join(dir, "gov.wasm.gz"))
	t.Setenv("REGISTRY_WASM", filepath.Join(dir, "reg.wasm.gz"))
	t.Setenv("ROOT_WASM", filepath.Join(dir, "root.wasm.gz"))

	// A cache dir that does not exist: if resolve tried to download it would fail.
	a, err := ResolveArtifacts(BootstrapConfig{CacheDir: filepath.Join(dir, "nope"), Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if a.PocketIC != bin {
		t.Errorf("env POCKET_IC_BIN ignored: %q", a.PocketIC)
	}
	if a.GovernanceWASM != filepath.Join(dir, "gov.wasm.gz") {
		t.Errorf("env GOVERNANCE_WASM ignored: %q", a.GovernanceWASM)
	}
}

// Offline with nothing cached must explain what to do rather than hang or
// silently produce an unusable config.
func TestResolveArtifactsOfflineWithoutCache(t *testing.T) {
	t.Setenv("POCKET_IC_BIN", "")
	t.Setenv("GOVERNANCE_WASM", "")
	t.Setenv("REGISTRY_WASM", "")
	t.Setenv("ROOT_WASM", "")
	_, err := ResolveArtifacts(BootstrapConfig{CacheDir: t.TempDir(), Offline: true})
	if err == nil {
		t.Fatal("offline with an empty cache must error")
	}
	if !strings.Contains(err.Error(), "offline") {
		t.Errorf("error should mention offline mode: %v", err)
	}
}

func TestFetchPocketICVerifiesHash(t *testing.T) {
	payload := []byte("fake pocket-ic binary")
	body := gzipOf(t, payload)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	cache := t.TempDir()
	path, err := fetchPocketIC(srv.URL, sha256hex(body), cache)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("decompressed content mismatch: %q", got)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&0o111 == 0 {
		t.Error("pocket-ic must be executable")
	}
}

// A corrupted or substituted download must be rejected, not cached.
func TestFetchPocketICRejectsBadHash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(gzipOf(t, []byte("tampered")))
	}))
	defer srv.Close()

	cache := t.TempDir()
	_, err := fetchPocketIC(srv.URL, sha256hex([]byte("expected-something-else")), cache)
	if err == nil {
		t.Fatal("hash mismatch must error")
	}
	if !strings.Contains(err.Error(), "sha256") {
		t.Errorf("error should name the hash mismatch: %v", err)
	}
	ents, _ := os.ReadDir(cache)
	for _, e := range ents {
		if !strings.Contains(e.Name(), ".tmp") {
			t.Errorf("failed download must not be cached: %s", e.Name())
		}
	}
}

func TestFetchCanistersExtractsOnlyNeeded(t *testing.T) {
	files := map[string][]byte{
		"governance-canister_test.wasm.gz": []byte("gov"),
		"registry-canister.wasm.gz":        []byte("reg"),
		"root-canister.wasm.gz":            []byte("root"),
		"cycles-minting-canister.wasm.gz":  []byte("unused"),
	}
	body := tarOf(t, files)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	cache := t.TempDir()
	got, err := fetchCanisters(srv.URL, sha256hex(body), cache)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"governance": "gov", "registry": "reg", "root": "root",
	} {
		var p string
		switch name {
		case "governance":
			p = got.GovernanceWASM
		case "registry":
			p = got.RegistryWASM
		case "root":
			p = got.RootWASM
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if string(b) != want {
			t.Errorf("%s: got %q, want %q", name, b, want)
		}
	}
	if _, err := os.Stat(filepath.Join(cache, "cycles-minting-canister.wasm.gz")); err == nil {
		t.Error("unneeded canisters should not be extracted")
	}
}

func TestFetchCanistersRejectsBadHash(t *testing.T) {
	body := tarOf(t, map[string][]byte{"governance-canister_test.wasm.gz": []byte("gov")})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()
	if _, err := fetchCanisters(srv.URL, sha256hex([]byte("nope")), t.TempDir()); err == nil {
		t.Fatal("hash mismatch must error")
	}
}

// A tar missing a canister alp needs must fail loudly rather than yield a
// half-populated set that fails later at install time.
func TestFetchCanistersMissingMember(t *testing.T) {
	body := tarOf(t, map[string][]byte{"governance-canister_test.wasm.gz": []byte("gov")})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()
	_, err := fetchCanisters(srv.URL, sha256hex(body), t.TempDir())
	if err == nil {
		t.Fatal("missing canister must error")
	}
	if !strings.Contains(err.Error(), "registry-canister") {
		t.Errorf("error should name the missing member: %v", err)
	}
}

// Every platform the flake pins must have a compiled-in hash, or that platform
// silently loses self-bootstrap.
func TestPocketICHashesCoverFlakePlatforms(t *testing.T) {
	for _, p := range []string{"x86_64-linux", "arm64-linux", "x86_64-darwin", "arm64-darwin"} {
		if pocketICSHA256[p] == "" {
			t.Errorf("no pinned hash for %s", p)
		}
	}
}

func TestPocketICAssetForGOOSGOARCH(t *testing.T) {
	tests := []struct {
		goos, goarch, want string
	}{
		{"linux", "amd64", "x86_64-linux"},
		{"linux", "arm64", "arm64-linux"},
		{"darwin", "amd64", "x86_64-darwin"},
		{"darwin", "arm64", "arm64-darwin"},
	}
	for _, tt := range tests {
		got, err := pocketICAsset(tt.goos, tt.goarch)
		if err != nil {
			t.Errorf("%s/%s: %v", tt.goos, tt.goarch, err)
			continue
		}
		if got != tt.want {
			t.Errorf("%s/%s: got %q, want %q", tt.goos, tt.goarch, got, tt.want)
		}
	}
	if _, err := pocketICAsset("windows", "amd64"); err == nil {
		t.Error("unsupported platform must error")
	}
}

// The cache is keyed by release tag so bumping the IC release fetches fresh
// artifacts instead of reusing stale ones under the same path.
func TestCacheDirIsReleaseKeyed(t *testing.T) {
	a := releaseCacheDir("/c", "release-A")
	b := releaseCacheDir("/c", "release-B")
	if a == b {
		t.Fatal("different releases must not share a cache dir")
	}
	if !strings.Contains(a, "release-A") {
		t.Errorf("cache dir should name the release: %q", a)
	}
}
