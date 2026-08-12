package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	pluginv1 "github.com/Vondel-Media/vondel-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Vondel-Media/vondel-plugins/catalog"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestValidateRepositoryUsesLiteralVondelAllowlist(t *testing.T) {
	want := map[string]struct{}{
		"Vondel-Media/vondel-plugin-tmdb":               {},
		"Vondel-Media/vondel-plugin-tvdb":               {},
		"Vondel-Media/vondel-plugin-ebook-metadata":     {},
		"Vondel-Media/vondel-plugin-audiobook-metadata": {},
		"Vondel-Media/vondel-plugin-manga-metadata":     {},
		"Vondel-Media/vondel-plugin-autoscan-arr":       {},
	}
	if len(allowedRepositories) != len(want) {
		t.Fatalf("allowedRepositories has %d entries, want exactly %d: %v", len(allowedRepositories), len(want), allowedRepositories)
	}
	for repo := range want {
		if _, ok := allowedRepositories[repo]; !ok {
			t.Errorf("allowedRepositories is missing %q", repo)
		}
		if err := validateRepository(repo); err != nil {
			t.Errorf("validateRepository(%q) error = %v", repo, err)
		}
	}
	for repo := range allowedRepositories {
		if _, ok := want[repo]; !ok {
			t.Errorf("allowedRepositories has unexpected entry %q", repo)
		}
	}

	for _, repo := range []string{
		"Silo-Server/silo-plugin-metadata-tmdb",
		"Vondel-Media/vondel-plugin-metadb",
		"Vondel-Media/vondel-plugin-unrelated",
		"Vondel-Media/vondel-plugin-tmdb/../../attacker",
		"",
	} {
		if err := validateRepository(repo); err == nil {
			t.Errorf("validateRepository(%q) accepted a repository outside the allowlist", repo)
		}
	}
}

func TestValidateReleaseAssetsRequiresExactAuthenticatedAPIAssets(t *testing.T) {
	repo := "Vondel-Media/vondel-plugin-tmdb"
	asset := func(id int64, name string) catalog.Asset {
		return catalog.Asset{ID: id, Name: name, URL: fmt.Sprintf("https://api.github.com/repos/%s/releases/assets/%d", repo, id), BrowserDownloadURL: "https://github.com/" + repo + "/releases/download/v1.2.23/" + name}
	}
	valid := catalog.Release{TagName: "v1.2.23", Assets: []catalog.Asset{
		asset(1, "checksums.txt"), asset(2, "plugin-darwin-arm64"),
		asset(3, "plugin-linux-amd64"), asset(4, "plugin-linux-arm64"),
	}}
	if _, err := validateReleaseAssets("https://api.github.com", repo, valid); err != nil {
		t.Fatalf("validateReleaseAssets(valid) error = %v", err)
	}

	for _, tc := range []struct {
		name   string
		mutate func(*catalog.Release)
	}{
		{"missing", func(r *catalog.Release) { r.Assets = r.Assets[:3] }},
		{"extra", func(r *catalog.Release) { r.Assets = append(r.Assets, asset(5, "notes.txt")) }},
		{"duplicate", func(r *catalog.Release) { r.Assets[3].Name = "plugin-linux-amd64" }},
		{"wrong API path", func(r *catalog.Release) {
			r.Assets[0].URL = "https://api.github.com/repos/attacker/repo/releases/assets/1"
		}},
		{"wrong download path", func(r *catalog.Release) {
			r.Assets[0].BrowserDownloadURL = "https://example.invalid/checksums.txt"
		}},
		{"missing ID", func(r *catalog.Release) { r.Assets[0].ID = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := valid
			candidate.Assets = append([]catalog.Asset(nil), valid.Assets...)
			tc.mutate(&candidate)
			if _, err := validateReleaseAssets("https://api.github.com", repo, candidate); err == nil {
				t.Fatal("invalid asset set accepted")
			}
		})
	}
}

func TestDownloadReleaseAssetsAuthenticatesAPIAndStripsTokenOnRedirect(t *testing.T) {
	const token = "catalog-token-canary-87421"
	var redirectSawAuthorization bool
	blob := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectSawAuthorization = r.Header.Get("Authorization") != ""
		_, _ = w.Write([]byte("binary"))
	}))
	defer blob.Close()

	var apiSawAuthorization, apiSawAccept bool
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiSawAuthorization = r.Header.Get("Authorization") == "Bearer "+token
		apiSawAccept = r.Header.Get("Accept") == "application/octet-stream"
		http.Redirect(w, r, blob.URL+"/asset", http.StatusFound)
	}))
	defer api.Close()

	repo := "Vondel-Media/vondel-plugin-tmdb"
	asset := catalog.Asset{ID: 7, Name: "plugin-linux-amd64", URL: api.URL + "/repos/" + repo + "/releases/assets/7"}
	got, err := downloadAsset(context.Background(), api.Client(), token, api.URL, repo, asset)
	if err != nil {
		t.Fatalf("downloadAsset() error = %v", err)
	}
	if string(got) != "binary" || !apiSawAuthorization || !apiSawAccept {
		t.Fatalf("download did not use authenticated octet-stream API request")
	}
	if redirectSawAuthorization {
		t.Fatal("Authorization was forwarded to cross-origin redirect")
	}
}

func TestVerifyReleaseContentsRequiresBareBijectiveMatchingSHA256(t *testing.T) {
	contents := map[string][]byte{
		"plugin-darwin-arm64": []byte("darwin"),
		"plugin-linux-amd64":  []byte("linux-amd64"),
		"plugin-linux-arm64":  []byte("linux-arm64"),
	}
	var checksum strings.Builder
	for _, name := range releaseBinaryNames {
		digest := sha256.Sum256(contents[name])
		fmt.Fprintf(&checksum, "%x  %s\n", digest, name)
	}
	contents["checksums.txt"] = []byte(checksum.String())
	got, err := verifyReleaseContents(contents)
	if err != nil {
		t.Fatalf("verifyReleaseContents(valid) error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("verified checksums = %d, want 3", len(got))
	}

	for _, invalid := range []string{
		strings.Replace(checksum.String(), "plugin-linux-amd64", "dir/plugin-linux-amd64", 1),
		strings.Replace(checksum.String(), "plugin-linux-amd64", "plugin-linux-arm64", 1),
		strings.Replace(checksum.String(), fmt.Sprintf("%x", sha256.Sum256([]byte("linux-amd64"))), strings.Repeat("0", 64), 1),
		checksum.String() + "bad extra\n",
	} {
		candidate := cloneContents(contents)
		candidate["checksums.txt"] = []byte(invalid)
		if _, err := verifyReleaseContents(candidate); err == nil {
			t.Fatalf("invalid checksums accepted: %q", invalid)
		}
	}
}

func cloneContents(source map[string][]byte) map[string][]byte {
	clone := make(map[string][]byte, len(source))
	for name, body := range source {
		clone[name] = append([]byte(nil), body...)
	}
	return clone
}

func TestValidateNativeManifestOutputRequiresExactSourceAndBinaryIdentity(t *testing.T) {
	binary := []byte("native release binary")
	source := &catalog.SourceManifest{
		PluginId: "silo.tmdb", Version: "1.2.23", Checksum: "__CHECKSUM__", SiloApiVersion: "v1",
		Presentation:       &pluginv1.PluginPresentation{SourceUrl: "https://github.com/Vondel-Media/vondel-plugin-tmdb", PublisherName: "Vondel", PublisherUrl: "https://github.com/Vondel-Media"},
		SupportedPlatforms: []*pluginv1.SupportedPlatform{{Os: "darwin", Arch: "arm64"}},
		Capabilities:       []*pluginv1.CapabilityDescriptor{{Type: "metadata_provider.v1", Id: "tmdb"}},
	}
	actual := proto.Clone(source).(*catalog.SourceManifest)
	actual.Checksum = fmt.Sprintf("%x", sha256.Sum256(binary))
	output, err := protojson.Marshal(actual)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateNativeManifestOutput(source, binary, output); err != nil {
		t.Fatalf("validateNativeManifestOutput(valid) error = %v", err)
	}
	actual.PluginId = "silo.attacker"
	output, _ = protojson.Marshal(actual)
	if err := validateNativeManifestOutput(source, binary, output); err == nil {
		t.Fatal("mismatched native manifest accepted")
	}
}

func TestNativeValidationDoesNotInheritSourceCredential(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	if !strings.Contains(source, `command.Env = []string{"PATH=" + os.Getenv("PATH")}`) {
		t.Fatal("native release binary inherits updater credentials")
	}
}
