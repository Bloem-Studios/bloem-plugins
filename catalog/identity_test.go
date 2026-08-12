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
		"https://github.com/Vondel-Media/vondel-plugin-metadb":             {},
		"https://github.com/Vondel-Media/vondel-plugin-tmdb":               {},
		"https://github.com/Vondel-Media/vondel-plugin-tvdb":               {},
		"https://github.com/Vondel-Media/vondel-plugin-audiobook-metadata": {},
		"https://github.com/Vondel-Media/vondel-plugin-manga-metadata":     {},
	}
	for _, plugin := range index.Plugins {
		if _, ok := allowed[plugin.RepoURL]; !ok {
			t.Errorf("manifest contains non-allowlisted repository %q", plugin.RepoURL)
		}
	}
}
