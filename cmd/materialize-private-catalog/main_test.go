package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	pluginv1 "github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Bloem-Studios/bloem-plugins/catalog"
)

const testToken = "task6-token-canary-892734"

type releaseFixture struct {
	server         *httptest.Server
	blob           *httptest.Server
	index          catalog.RepositoryIndex
	bodies         map[string][]byte
	redirectAuth   bool
	failAsset      string
	wrongAssetURL  bool
	wrongChecksums bool
	canaryAsset    bool
}

func TestMaterializeRequiresProcessBoundaryAuthentication(t *testing.T) {
	fixture := newReleaseFixture(t)
	err := materialize(context.Background(), fixture.server.Client(), "", fixture.server.URL, writeCatalogFixture(t, fixture.index), filepath.Join(t.TempDir(), "static"))
	if err == nil || !strings.Contains(err.Error(), "GITHUB_TOKEN is required") {
		t.Fatalf("materialize() error = %v, want missing-auth failure", err)
	}
}

func TestMaterializeRejectsNonAllowlistedRepositoryAndTraversal(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*catalog.RepositoryIndex)
	}{
		{"repository", func(index *catalog.RepositoryIndex) {
			index.Plugins[0].RepoURL = "https://github.com/Bloem-Studios/bloem-plugin-unrelated"
		}},
		{"plugin traversal", func(index *catalog.RepositoryIndex) { index.Plugins[0].Manifest.PluginId = "../escape" }},
		{"version traversal", func(index *catalog.RepositoryIndex) { index.Plugins[0].Manifest.Version = "../../escape" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newReleaseFixture(t)
			tc.mutate(&fixture.index)
			out := filepath.Join(t.TempDir(), "static")
			if err := materialize(context.Background(), fixture.server.Client(), testToken, fixture.server.URL, writeCatalogFixture(t, fixture.index), out); err == nil {
				t.Fatal("materialize accepted unsafe catalog identity")
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatalf("unsafe materialization published output: %v", err)
			}
		})
	}
}

func TestMaterializeRejectsNonAllowlistedAssetMetadataAndWrongHashes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*releaseFixture)
	}{
		{"asset URL", func(f *releaseFixture) { f.wrongAssetURL = true }},
		{"release checksum", func(f *releaseFixture) { f.wrongChecksums = true }},
		{"catalog checksum", func(f *releaseFixture) {
			f.index.Plugins[0].Binaries["linux/amd64"] = catalog.PlatformBinary{URL: f.index.Plugins[0].Binaries["linux/amd64"].URL, Checksum: strings.Repeat("0", 64)}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newReleaseFixture(t)
			tc.mutate(fixture)
			if err := materialize(context.Background(), fixture.server.Client(), testToken, fixture.server.URL, writeCatalogFixture(t, fixture.index), filepath.Join(t.TempDir(), "static")); err == nil {
				t.Fatal("materialize accepted unverified release content")
			}
		})
	}
}

func TestMaterializeRejectsSymlinkDestination(t *testing.T) {
	fixture := newReleaseFixture(t)
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "static")
	if err := os.Symlink(real, out); err != nil {
		t.Fatal(err)
	}
	if err := materialize(context.Background(), fixture.server.Client(), testToken, fixture.server.URL, writeCatalogFixture(t, fixture.index), out); err == nil {
		t.Fatal("materialize accepted symlink destination")
	}
}

func TestMaterializeStripsAuthorizationOnCrossOriginRedirect(t *testing.T) {
	fixture := newReleaseFixture(t)
	if err := materialize(context.Background(), fixture.server.Client(), testToken, fixture.server.URL, writeCatalogFixture(t, fixture.index), filepath.Join(t.TempDir(), "static")); err != nil {
		t.Fatalf("materialize() error = %v", err)
	}
	if fixture.redirectAuth {
		t.Fatal("Authorization was forwarded to cross-origin asset storage")
	}
}

func TestMaterializeRejectsTokenCanaryAndLeavesNoPartialOutput(t *testing.T) {
	fixture := newReleaseFixture(t)
	fixture.canaryAsset = true
	outParent := t.TempDir()
	out := filepath.Join(outParent, "static")
	if err := materialize(context.Background(), fixture.server.Client(), testToken, fixture.server.URL, writeCatalogFixture(t, fixture.index), out); err == nil {
		t.Fatal("materialize published credential-bearing content")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("partial output exists: %v", err)
	}
	assertNoStagingDirectories(t, outParent)
}

func TestMaterializeFailurePreservesPreviousTreeAndCleansStaging(t *testing.T) {
	fixture := newReleaseFixture(t)
	parent := t.TempDir()
	out := filepath.Join(parent, "static")
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "previous"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture.failAsset = "plugin-linux-arm64"
	if err := materialize(context.Background(), fixture.server.Client(), testToken, fixture.server.URL, writeCatalogFixture(t, fixture.index), out); err == nil {
		t.Fatal("materialize succeeded despite failed asset")
	}
	if body, err := os.ReadFile(filepath.Join(out, "previous")); err != nil || string(body) != "keep" {
		t.Fatalf("previous output was not preserved: %q, %v", body, err)
	}
	assertNoStagingDirectories(t, parent)
}

func TestMaterializeProducesDeterministicTokenFreeAnonymousStaticTree(t *testing.T) {
	fixture := newReleaseFixture(t)
	root := t.TempDir()
	catalogPath := writeCatalogFixture(t, fixture.index)
	out := filepath.Join(root, "static")
	if err := materialize(context.Background(), fixture.server.Client(), testToken, fixture.server.URL, catalogPath, out); err != nil {
		t.Fatalf("first materialize: %v", err)
	}
	first := snapshotTree(t, out)
	if err := materialize(context.Background(), fixture.server.Client(), testToken, fixture.server.URL, catalogPath, out); err != nil {
		t.Fatalf("second materialize: %v", err)
	}
	second := snapshotTree(t, out)
	if fmt.Sprint(first) != fmt.Sprint(second) {
		t.Fatal("repeated materialization was not byte-for-byte deterministic")
	}

	catalogBody, err := os.ReadFile(filepath.Join(out, "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(catalogBody), testToken) || strings.Contains(string(catalogBody), "api.github.com") {
		t.Fatal("static catalog contains credential or API material")
	}
	var index catalog.RepositoryIndex
	if err := json.Unmarshal(catalogBody, &index); err != nil {
		t.Fatal(err)
	}
	if len(index.Plugins) != len(retainedPlugins) {
		t.Fatalf("plugins = %d, want %d", len(index.Plugins), len(retainedPlugins))
	}
	for _, pkg := range index.Plugins {
		if !isSafeRelativeURL(pkg.ChecksumsURL) {
			t.Errorf("unsafe checksums URL %q", pkg.ChecksumsURL)
		}
		for _, binary := range pkg.Binaries {
			if !isSafeRelativeURL(binary.URL) {
				t.Errorf("unsafe binary URL %q", binary.URL)
			}
			info, err := os.Stat(filepath.Join(out, filepath.FromSlash(binary.URL)))
			if err != nil || info.Mode().Perm() != 0o755 {
				t.Errorf("binary mode = %v, %v", info, err)
			}
		}
	}

	server := httptest.NewServer(http.FileServer(http.Dir(out)))
	defer server.Close()
	for _, path := range []string{"/catalog.json", "/" + index.Plugins[0].ChecksumsURL, "/" + index.Plugins[0].Binaries["linux/amd64"].URL} {
		resp, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("anonymous GET %s = %d", path, resp.StatusCode)
		}
	}
}

func newReleaseFixture(t *testing.T) *releaseFixture {
	t.Helper()
	f := &releaseFixture{bodies: map[string][]byte{}}
	f.blob = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			f.redirectAuth = true
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		_, _ = w.Write(f.bodies[name])
	}))
	t.Cleanup(f.blob.Close)

	repos := []struct{ repo, id, version string }{
		{"Bloem-Studios/bloem-plugin-tmdb", "silo.tmdb", "1.2.25"},
		{"Bloem-Studios/bloem-plugin-tvdb", "silo.tvdb", "1.2.28"},
		{"Bloem-Studios/bloem-plugin-ebook-metadata", "silo.ebook-metadata", "0.1.4"},
		{"Bloem-Studios/bloem-plugin-audiobook-metadata", "silo.audiobook-metadata", "0.1.7"},
		{"Bloem-Studios/bloem-plugin-manga-metadata", "silo.manga-metadata", "0.1.4"},
		{"Bloem-Studios/bloem-plugin-autoscan-arr", "silo.autoscan.arr", "0.1.5"},
		{"Bloem-Studios/bloem-plugin-theintrodb", "silo.theintrodb", "0.1.2"},
		{"Bloem-Studios/bloem-plugin-sportarr-metadata", "silo.sportarr", "1.0.4"},
		{"Bloem-Studios/bloem-plugin-watchprovider-floppy", "silo.watchprovider.floppy", "0.2.4"},
		{"Bloem-Studios/bloem-plugin-requests-arr", "silo.requests.arr", "0.1.5"},
		{"Bloem-Studios/bloem-plugin-requests-seerr", "silo.requests.seerr", "0.1.3"},
	}
	for _, item := range repos {
		binaries := map[string]catalog.PlatformBinary{}
		for _, name := range releaseBinaryNames {
			key := item.repo + "/" + name
			body := []byte(key)
			f.bodies[key] = body
			platform := strings.ReplaceAll(strings.TrimPrefix(name, "plugin-"), "-", "/")
			binaries[platform] = catalog.PlatformBinary{URL: "https://github.com/" + item.repo + "/releases/download/v" + item.version + "/" + name, Checksum: fmt.Sprintf("%x", sha256.Sum256(body))}
		}
		f.index.Plugins = append(f.index.Plugins, catalog.CatalogPackage{
			Manifest:     &catalog.SourceManifest{PluginId: item.id, Version: item.version, SiloApiVersion: "v1", SupportedPlatforms: []*pluginv1.SupportedPlatform{{Os: "darwin", Arch: "arm64"}, {Os: "linux", Arch: "amd64"}, {Os: "linux", Arch: "arm64"}}, Capabilities: []*pluginv1.CapabilityDescriptor{{Type: "test.v1", Id: "test"}}},
			RepoURL:      "https://github.com/" + item.repo,
			ChecksumsURL: "https://github.com/" + item.repo + "/releases/download/v" + item.version + "/checksums.txt",
			Binaries:     binaries,
		})
	}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 6 || parts[0] != "repos" || parts[3] != "releases" {
			http.NotFound(w, r)
			return
		}
		repo := parts[1] + "/" + parts[2]
		if parts[4] == "tags" {
			version := strings.TrimPrefix(parts[5], "v")
			assets := make([]catalog.Asset, 0, 4)
			names := append([]string{"checksums.txt"}, releaseBinaryNames...)
			for i, name := range names {
				assetURL := f.server.URL + "/repos/" + repo + "/releases/assets/" + fmt.Sprint(i+1)
				if f.wrongAssetURL && name == "checksums.txt" {
					assetURL = f.server.URL + "/repos/attacker/repo/releases/assets/1"
				}
				assets = append(assets, catalog.Asset{ID: int64(i + 1), Name: name, URL: assetURL, BrowserDownloadURL: "https://github.com/" + repo + "/releases/download/v" + version + "/" + name})
			}
			_ = json.NewEncoder(w).Encode(catalog.Release{TagName: "v" + version, PublishedAt: "2026-08-12T00:00:00Z", Assets: assets})
			return
		}
		if parts[4] == "assets" {
			id := parts[5]
			name := map[string]string{"1": "checksums.txt", "2": "plugin-darwin-arm64", "3": "plugin-linux-amd64", "4": "plugin-linux-arm64"}[id]
			if name == f.failAsset {
				http.Error(w, "failed", http.StatusBadGateway)
				return
			}
			if name == "checksums.txt" {
				var sums strings.Builder
				for _, binaryName := range releaseBinaryNames {
					body := f.bodies[repo+"/"+binaryName]
					digest := sha256.Sum256(body)
					if f.wrongChecksums && binaryName == "plugin-linux-amd64" {
						digest = sha256.Sum256([]byte("wrong"))
					}
					fmt.Fprintf(&sums, "%x  %s\n", digest, binaryName)
				}
				_, _ = w.Write([]byte(sums.String()))
				return
			}
			if f.canaryAsset && name == "plugin-linux-amd64" {
				f.bodies[repo+"/"+name] = []byte(testToken)
			}
			http.Redirect(w, r, f.blob.URL+"/"+repo+"/"+name, http.StatusFound)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func writeCatalogFixture(t *testing.T, index catalog.RepositoryIndex) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "catalog.json")
	body, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func snapshotTree(t *testing.T, root string) []string {
	t.Helper()
	var snapshot []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			snapshot = append(snapshot, filepath.ToSlash(rel)+"/")
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snapshot = append(snapshot, fmt.Sprintf("%s:%o:%x", filepath.ToSlash(rel), info.Mode().Perm(), sha256.Sum256(body)))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(snapshot)
	return snapshot
}

func assertNoStagingDirectories(t *testing.T, parent string) {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".materialize-") {
			t.Errorf("staging entry remains: %s", entry.Name())
		}
	}
}
