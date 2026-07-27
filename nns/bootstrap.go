package nns

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// The PocketIC dry-run needs the pocket-ic server binary and three NNS canister
// wasms. Under nix these come from the flake via env vars; everywhere else alp
// fetches them from the pinned IC release and caches them, so a plain
// `go install` build can dry-run without a nix devShell.
//
// The pins below must match flake.nix. The flake remains the source of truth
// for nix builds; these constants are the same artifacts addressed by content
// hash so a download cannot silently differ from what nix would have supplied.
const (
	// ICReleaseTag is the pinned IC release the dry-run artifacts come from.
	ICReleaseTag = "release-2026-07-09_04-35-base"

	icReleaseBase   = "https://github.com/dfinity/ic/releases/download/"
	canistersSHA256 = "32b517094574f3321ffe61e403ce1f8b3b82a7406553cd40a20100c5d16e6a93"
)

// pocketICSHA256 is the sha256 of each platform's gzipped pocket-ic asset.
var pocketICSHA256 = map[string]string{
	"x86_64-linux":  "d1b63d8863281f3051db04fb2da3185c0d6abc0bc74d681040b8a34c5bf7e33f",
	"arm64-linux":   "e3aae8f1db901d7c63bd46c7b5bbac211afc6495227024b5e198a6f6fc074c03",
	"x86_64-darwin": "a274bb77568e4dc6df0f7e073e662a8e151fd3010e41fa85f1e2f997d4e34a6c",
	"arm64-darwin":  "a915936311b1311c408b69ec30125599093ad964d26c9867527e61cfb34f9a1f",
}

// canisterMembers maps the tar members alp needs to their cached basenames.
// Governance is the _test build: it permits the test-only calls the dry-run
// makes to seed a neuron.
var canisterMembers = []string{
	"governance-canister_test.wasm.gz",
	"registry-canister.wasm.gz",
	"root-canister.wasm.gz",
}

// Artifacts is a resolved set of dry-run inputs, whatever their source.
type Artifacts struct {
	PocketIC       string
	GovernanceWASM string
	RegistryWASM   string
	RootWASM       string
}

// BootstrapConfig tunes artifact resolution.
type BootstrapConfig struct {
	// CacheDir overrides the default per-user cache location.
	CacheDir string
	// Offline forbids downloading: only env vars and an existing cache are used.
	Offline bool
	// Progress, when non-nil, is called before each download starts.
	Progress func(what string, bytes int64)
}

// ResolveArtifacts locates the dry-run inputs. Explicit env vars always win, so
// nix devShells and CI keep full control and nothing is ever downloaded behind
// their back. Otherwise the pinned release artifacts are used from cache, or
// fetched into it.
func ResolveArtifacts(cfg BootstrapConfig) (Artifacts, error) {
	a := Artifacts{
		PocketIC:       os.Getenv("POCKET_IC_BIN"),
		GovernanceWASM: os.Getenv("GOVERNANCE_WASM"),
		RegistryWASM:   os.Getenv("REGISTRY_WASM"),
		RootWASM:       os.Getenv("ROOT_WASM"),
	}
	if a.complete() {
		return a, a.validate()
	}

	dir := cfg.CacheDir
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return a, fmt.Errorf("locate cache dir: %w", err)
		}
		dir = filepath.Join(base, "alpage")
	}
	dir = releaseCacheDir(dir, ICReleaseTag)

	cached := a.fillFromCache(dir)
	if cached.complete() && cached.validate() == nil {
		return cached, nil
	}
	if cfg.Offline {
		return a, fmt.Errorf("dry-run artifacts missing from %s and offline mode is set; "+
			"run without --offline to fetch them, or set POCKET_IC_BIN/GOVERNANCE_WASM/REGISTRY_WASM/ROOT_WASM", dir)
	}
	return fetchAll(dir, cfg.Progress)
}

func (a Artifacts) complete() bool {
	return a.PocketIC != "" && a.GovernanceWASM != "" && a.RegistryWASM != "" && a.RootWASM != ""
}

// validate checks every resolved path exists, so a stale env var or a
// half-populated cache fails here rather than deep inside the dry-run.
func (a Artifacts) validate() error {
	for name, p := range map[string]string{
		"POCKET_IC_BIN": a.PocketIC, "GOVERNANCE_WASM": a.GovernanceWASM,
		"REGISTRY_WASM": a.RegistryWASM, "ROOT_WASM": a.RootWASM,
	} {
		if p == "" {
			return fmt.Errorf("%s not set", name)
		}
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("%s=%s: %w", name, p, err)
		}
	}
	return nil
}

// fillFromCache fills any unset field from the cache directory, leaving
// env-provided values untouched.
func (a Artifacts) fillFromCache(dir string) Artifacts {
	if a.PocketIC == "" {
		a.PocketIC = filepath.Join(dir, "pocket-ic")
	}
	if a.GovernanceWASM == "" {
		a.GovernanceWASM = filepath.Join(dir, canisterMembers[0])
	}
	if a.RegistryWASM == "" {
		a.RegistryWASM = filepath.Join(dir, canisterMembers[1])
	}
	if a.RootWASM == "" {
		a.RootWASM = filepath.Join(dir, canisterMembers[2])
	}
	return a
}

// releaseCacheDir keys the cache by release tag, so bumping the pin fetches
// fresh artifacts instead of reusing stale ones under the same path.
func releaseCacheDir(base, tag string) string { return filepath.Join(base, tag) }

// pocketICAsset maps a Go platform onto the IC release's asset naming.
func pocketICAsset(goos, goarch string) (string, error) {
	arch := map[string]string{"amd64": "x86_64", "arm64": "arm64"}[goarch]
	if arch == "" || (goos != "linux" && goos != "darwin") {
		return "", fmt.Errorf("no pocket-ic release asset for %s/%s; set POCKET_IC_BIN to a local build", goos, goarch)
	}
	return fmt.Sprintf("%s-%s", arch, goos), nil
}

func fetchAll(dir string, progress func(string, int64)) (Artifacts, error) {
	var a Artifacts
	asset, err := pocketICAsset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return a, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return a, err
	}
	if progress != nil {
		progress("pocket-ic", 0)
	}
	binURL := fmt.Sprintf("%s%s/pocket-ic-%s.gz", icReleaseBase, ICReleaseTag, asset)
	bin, err := fetchPocketIC(binURL, pocketICSHA256[asset], dir)
	if err != nil {
		return a, err
	}
	if progress != nil {
		progress("NNS canisters", 0)
	}
	tarURL := fmt.Sprintf("%s%s/canisters.tar", icReleaseBase, ICReleaseTag)
	a, err = fetchCanisters(tarURL, canistersSHA256, dir)
	if err != nil {
		return a, err
	}
	a.PocketIC = bin
	return a, a.validate()
}

// download streams url to a temp file in dir, verifying sha256 as it goes. The
// temp file is returned only on a hash match, so a corrupted or substituted
// download never lands in the cache under its real name.
func download(url, wantSHA, dir, tmpPattern string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch %s: http %d", url, resp.StatusCode)
	}
	tmp, err := os.CreateTemp(dir, tmpPattern)
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", fmt.Errorf("fetch %s: %w", url, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != wantSHA {
		os.Remove(tmpName)
		return "", fmt.Errorf("fetch %s: sha256 mismatch: got %s, want %s", url, got, wantSHA)
	}
	return tmpName, nil
}

// fetchPocketIC downloads the gzipped server binary, verifies it against the
// pinned hash, and decompresses it into dir as an executable.
func fetchPocketIC(url, wantSHA, dir string) (string, error) {
	if wantSHA == "" {
		return "", errors.New("no pinned pocket-ic hash for this platform")
	}
	gzPath, err := download(url, wantSHA, dir, "pocket-ic-*.gz.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(gzPath)

	f, err := os.Open(gzPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("decompress pocket-ic: %w", err)
	}
	defer zr.Close()

	out, err := os.CreateTemp(dir, "pocket-ic-*.bin.tmp")
	if err != nil {
		return "", err
	}
	outName := out.Name()
	if _, err := io.Copy(out, zr); err != nil {
		out.Close()
		os.Remove(outName)
		return "", fmt.Errorf("decompress pocket-ic: %w", err)
	}
	if err := out.Close(); err != nil {
		os.Remove(outName)
		return "", err
	}
	if err := os.Chmod(outName, 0o755); err != nil {
		os.Remove(outName)
		return "", err
	}
	final := filepath.Join(dir, "pocket-ic")
	if err := os.Rename(outName, final); err != nil {
		os.Remove(outName)
		return "", err
	}
	return final, nil
}

// fetchCanisters downloads canisters.tar, verifies it, and extracts only the
// members alp installs. A missing member is an error: a half-populated cache
// would otherwise fail later, deep inside the dry-run.
func fetchCanisters(url, wantSHA, dir string) (Artifacts, error) {
	var a Artifacts
	tarPath, err := download(url, wantSHA, dir, "canisters-*.tar.tmp")
	if err != nil {
		return a, err
	}
	defer os.Remove(tarPath)

	f, err := os.Open(tarPath)
	if err != nil {
		return a, err
	}
	defer f.Close()

	want := map[string]bool{}
	for _, m := range canisterMembers {
		want[m] = true
	}
	found := map[string]string{}
	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return a, fmt.Errorf("read canisters.tar: %w", err)
		}
		name := filepath.Base(hdr.Name)
		if !want[name] {
			continue
		}
		dst := filepath.Join(dir, name)
		out, err := os.CreateTemp(dir, name+".tmp-*")
		if err != nil {
			return a, err
		}
		tmpName := out.Name()
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			os.Remove(tmpName)
			return a, fmt.Errorf("extract %s: %w", name, err)
		}
		if err := out.Close(); err != nil {
			os.Remove(tmpName)
			return a, err
		}
		if err := os.Rename(tmpName, dst); err != nil {
			os.Remove(tmpName)
			return a, err
		}
		found[name] = dst
	}
	for _, m := range canisterMembers {
		if found[m] == "" {
			return a, fmt.Errorf("canisters.tar is missing %s", m)
		}
	}
	a.GovernanceWASM = found[canisterMembers[0]]
	a.RegistryWASM = found[canisterMembers[1]]
	a.RootWASM = found[canisterMembers[2]]
	return a, nil
}
