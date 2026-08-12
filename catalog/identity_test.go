package catalog

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestModuleUsesVondelIdentityAndTaggedSDK(t *testing.T) {
	data, err := os.ReadFile("../go.mod")
	if err != nil {
		t.Fatalf("ReadFile(go.mod) error = %v", err)
	}

	module := string(data)
	if !strings.Contains(module, "module github.com/Vondel-Media/vondel-plugins\n") {
		t.Fatal("go.mod does not declare the Vondel catalog module")
	}
	if !strings.Contains(module, "github.com/Vondel-Media/vondel-plugin-sdk v0.13.3") {
		t.Fatal("go.mod does not pin the Vondel SDK at v0.13.3")
	}
	if strings.Contains(module, "\nreplace ") || strings.Contains(module, "\nreplace (") {
		t.Fatal("go.mod contains a replace directive")
	}
}

func TestManifestReferencesOnlyAllowedVondelRepositories(t *testing.T) {
	data, err := os.ReadFile("../manifest.json")
	if err != nil {
		t.Fatalf("ReadFile(manifest.json) error = %v", err)
	}
	var index RepositoryIndex
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatalf("Unmarshal(manifest.json) error = %v", err)
	}

	allowed := map[string]struct{}{
		"https://github.com/Vondel-Media/vondel-plugin-tmdb":               {},
		"https://github.com/Vondel-Media/vondel-plugin-tvdb":               {},
		"https://github.com/Vondel-Media/vondel-plugin-ebook-metadata":     {},
		"https://github.com/Vondel-Media/vondel-plugin-audiobook-metadata": {},
		"https://github.com/Vondel-Media/vondel-plugin-manga-metadata":     {},
		"https://github.com/Vondel-Media/vondel-plugin-autoscan-arr":       {},
	}
	exactVersions := map[string]string{
		"silo.tmdb":               "1.2.23",
		"silo.tvdb":               "1.2.27",
		"silo.ebook-metadata":     "0.1.3",
		"silo.audiobook-metadata": "0.1.6",
		"silo.manga-metadata":     "0.1.3",
		"silo.autoscan.arr":       "0.1.4",
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
		if presentation.GetSourceUrl() != plugin.RepoURL || presentation.GetHomepageUrl() != plugin.RepoURL || presentation.GetPublisherName() != "Vondel" || presentation.GetPublisherUrl() != "https://github.com/Vondel-Media" {
			t.Errorf("%s does not use exact Vondel source/publisher presentation", plugin.Manifest.GetPluginId())
		}
		if len(plugin.Binaries) != 3 {
			t.Errorf("%s has %d binaries, want exactly 3", plugin.Manifest.GetPluginId(), len(plugin.Binaries))
		}
		for _, platform := range []string{"darwin/arm64", "linux/amd64", "linux/arm64"} {
			binary, ok := plugin.Binaries[platform]
			if !ok || len(binary.Checksum) != 64 || !strings.HasPrefix(binary.URL, "https://github.com/Vondel-Media/") {
				t.Errorf("%s has invalid %s binary metadata", plugin.Manifest.GetPluginId(), platform)
			}
		}
		if !strings.HasPrefix(plugin.ChecksumsURL, "https://github.com/Vondel-Media/") {
			t.Errorf("%s has invalid checksums URL", plugin.Manifest.GetPluginId())
		}
		serialized, _ := json.Marshal(plugin)
		if strings.Contains(string(serialized), "Silo-Server/") || strings.Contains(string(serialized), "api.github.com") || strings.Contains(string(serialized), "Authorization: Bearer") {
			t.Errorf("%s catalog package contains upstream/API/credential material", plugin.Manifest.GetPluginId())
		}
	}
}
