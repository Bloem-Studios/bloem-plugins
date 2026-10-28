package catalog

import (
	"encoding/json"
	"os"
	"testing"
)

func TestInventoryContainsOnlyCustomPluginsWithExplicitPrerequisites(t *testing.T) {
	data, err := os.ReadFile("../plugins.json")
	if err != nil {
		t.Fatal(err)
	}
	var inventory struct {
		SchemaVersion int `json:"schema_version"`
		Plugins       []struct {
			ID           string   `json:"plugin_id"`
			Repository   string   `json:"repository_url"`
			Version      string   `json:"source_version"`
			SDK          string   `json:"sdk_version"`
			InstallMode  string   `json:"installation_mode"`
			Status       string   `json:"status"`
			Requirements []string `json:"requirements"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(data, &inventory); err != nil {
		t.Fatal(err)
	}
	if inventory.SchemaVersion != 1 || len(inventory.Plugins) != 5 {
		t.Fatal("expected five custom inventory entries with schema version 1")
	}
	expected := map[string]struct{ repo, version, sdk, mode, status string }{
		"bloem.storage.bookwarehouse":  {"bloem-plugin-bookwarehouse", "0.3.3", "0.27.0", "native", "requires_native_approval"},
		"bloem.storage.s3":             {"bloem-storage-s3", "0.3.2", "0.27.0", "native", "requires_native_approval"},
		"bloem.requests.bookwarehouse": {"bloem-plugin-requests-bookwarehouse", "0.1.2", "0.26.0", "ordinary", "requires_connection_and_policy"},
		"bloem.requests.audiobooks":    {"bloem-plugin-requests-audiobooks", "0.1.0", "0.26.0", "ordinary", "requires_connection_and_download_client"},
		"bloem.pastime":                {"bloem-plugin-pastime", "0.2.0", "0.26.0", "ordinary", "requires_connection_and_policy"},
	}
	for _, p := range inventory.Plugins {
		want, ok := expected[p.ID]
		if !ok {
			t.Fatalf("unexpected or duplicate plugin %q", p.ID)
		}
		delete(expected, p.ID)
		if p.Repository != "https://github.com/Bloem-Studios/"+want.repo || p.Version != want.version || p.SDK != want.sdk || p.InstallMode != want.mode || p.Status != want.status || len(p.Requirements) == 0 {
			t.Errorf("invalid identity/version/install prerequisites for %s", p.ID)
		}
	}
	if len(expected) != 0 {
		t.Fatal("missing custom plugin")
	}
}
