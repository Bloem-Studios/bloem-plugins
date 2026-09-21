package catalog

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestModuleUsesBloemIdentityAndTaggedSDK(t *testing.T) {
	data, err := os.ReadFile("../go.mod")
	if err != nil {
		t.Fatalf("ReadFile(go.mod) error = %v", err)
	}

	module := string(data)
	if !strings.Contains(module, "module github.com/Bloem-Studios/bloem-plugins\n") {
		t.Fatal("go.mod does not declare the Bloem catalog module")
	}
	if !strings.Contains(module, "github.com/Bloem-Studios/bloem-plugin-sdk v0.16.1") {
		t.Fatal("go.mod does not pin the Bloem SDK at v0.16.1")
	}
	if strings.Contains(module, "\nreplace ") || strings.Contains(module, "\nreplace (") {
		t.Fatal("go.mod contains a replace directive")
	}
}

func TestManifestReferencesOnlyAllowedBloemRepositories(t *testing.T) {
	data, err := os.ReadFile("../manifest.json")
	if err != nil {
		t.Fatalf("ReadFile(manifest.json) error = %v", err)
	}
	var index RepositoryIndex
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatalf("Unmarshal(manifest.json) error = %v", err)
	}

	allowed := map[string]struct{}{
		"https://github.com/Bloem-Studios/bloem-plugin-tmdb":                 {},
		"https://github.com/Bloem-Studios/bloem-plugin-tvdb":                 {},
		"https://github.com/Bloem-Studios/bloem-plugin-ebook-metadata":       {},
		"https://github.com/Bloem-Studios/bloem-plugin-audiobook-metadata":   {},
		"https://github.com/Bloem-Studios/bloem-plugin-manga-metadata":       {},
		"https://github.com/Bloem-Studios/bloem-plugin-autoscan-arr":         {},
		"https://github.com/Bloem-Studios/bloem-plugin-theintrodb":           {},
		"https://github.com/Bloem-Studios/bloem-plugin-sportarr-metadata":    {},
		"https://github.com/Bloem-Studios/bloem-plugin-watchprovider-floppy": {},
		"https://github.com/Bloem-Studios/bloem-plugin-requests-arr":         {},
		"https://github.com/Bloem-Studios/bloem-plugin-requests-seerr":       {},
	}
	exactVersions := map[string]string{
		"silo.tmdb":                 "1.2.25",
		"silo.tvdb":                 "1.2.28",
		"silo.ebook-metadata":       "0.1.4",
		"silo.audiobook-metadata":   "0.1.7",
		"silo.manga-metadata":       "0.1.4",
		"silo.autoscan.arr":         "0.1.5",
		"silo.theintrodb":           "0.1.2",
		"silo.sportarr":             "1.0.4",
		"silo.watchprovider.floppy": "0.2.4",
		"silo.requests.arr":         "0.1.5",
		"silo.requests.seerr":       "0.1.3",
	}
	if len(index.Plugins) != len(allowed) {
		t.Fatalf("manifest contains %d plugins, want exactly %d", len(index.Plugins), len(allowed))
	}
	seen := map[string]struct{}{}
	for _, plugin := range index.Plugins {
		if _, ok := allowed[plugin.RepoURL]; !ok {
			t.Errorf("manifest contains non-allowlisted repository %q", plugin.RepoURL)
		}
		if plugin.Manifest == nil || plugin.Manifest.GetPluginId() == "" {
			t.Fatal("manifest contains package without plugin identity")
		}
		if _, exists := seen[plugin.Manifest.GetPluginId()]; exists {
			t.Errorf("manifest contains duplicate plugin ID %q", plugin.Manifest.GetPluginId())
		}
		seen[plugin.Manifest.GetPluginId()] = struct{}{}
		if plugin.Manifest.GetVersion() != exactVersions[plugin.Manifest.GetPluginId()] {
			t.Errorf("%s version = %q, want exact retained release %q", plugin.Manifest.GetPluginId(), plugin.Manifest.GetVersion(), exactVersions[plugin.Manifest.GetPluginId()])
		}
		if plugin.Manifest.GetSiloApiVersion() != "v1" {
			t.Errorf("%s uses API %q, want v1", plugin.Manifest.GetPluginId(), plugin.Manifest.GetSiloApiVersion())
		}
		presentation := plugin.Manifest.GetPresentation()
		if presentation.GetSourceUrl() != plugin.RepoURL || presentation.GetHomepageUrl() != plugin.RepoURL || presentation.GetPublisherName() != "Bloem Studios" || presentation.GetPublisherUrl() != "https://github.com/Bloem-Studios" {
			t.Errorf("%s does not use exact Bloem source/publisher presentation", plugin.Manifest.GetPluginId())
		}
		if len(plugin.Binaries) != 3 {
			t.Errorf("%s has %d binaries, want exactly 3", plugin.Manifest.GetPluginId(), len(plugin.Binaries))
		}
		for _, platform := range []string{"darwin/arm64", "linux/amd64", "linux/arm64"} {
			binary, ok := plugin.Binaries[platform]
			if !ok || len(binary.Checksum) != 64 || !strings.HasPrefix(binary.URL, "https://github.com/Bloem-Studios/") {
				t.Errorf("%s has invalid %s binary metadata", plugin.Manifest.GetPluginId(), platform)
			}
		}
		if !strings.HasPrefix(plugin.ChecksumsURL, "https://github.com/Bloem-Studios/") {
			t.Errorf("%s has invalid checksums URL", plugin.Manifest.GetPluginId())
		}
		// Everything the server fetches or links as the source must be Bloem's. Descriptions
		// may link the upstream project to credit it.
		for _, link := range []string{plugin.RepoURL, plugin.ChecksumsURL, presentation.GetSourceUrl(), presentation.GetHomepageUrl(),
			presentation.GetSupportUrl(), presentation.GetChangelogUrl(), presentation.GetPublisherUrl()} {
			if !strings.HasPrefix(link, "https://github.com/Bloem-Studios") {
				t.Errorf("%s links outside Bloem-Studios: %q", plugin.Manifest.GetPluginId(), link)
			}
		}
		serialized, _ := json.Marshal(plugin)
		if strings.Contains(string(serialized), "api.github.com") || strings.Contains(string(serialized), "Authorization: Bearer") {
			t.Errorf("%s catalog package contains API or credential material", plugin.Manifest.GetPluginId())
		}
	}
}
