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
	if !strings.Contains(module, "github.com/Bloem-Studios/bloem-plugin-sdk v0.26.0") {
		t.Fatal("go.mod does not pin the Bloem SDK at v0.26.0")
	}
	if strings.Contains(module, "\nreplace ") || strings.Contains(module, "\nreplace (") {
		t.Fatal("go.mod contains a replace directive")
	}
}

func TestCustomCatalogDoesNotDuplicateSiloOrAdvertiseUnreadyNativePlugins(t *testing.T) {
	data, err := os.ReadFile("../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var index RepositoryIndex
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatal(err)
	}
	if len(index.Plugins) != 1 || index.Plugins[0].Manifest.GetPluginId() != "bloem.pastime" {
		t.Fatalf("generic catalog must advertise only the supported Pastime plugin; native storage remains outside the ordinary installer")
	}
}
