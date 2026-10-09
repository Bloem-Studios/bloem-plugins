package catalog

import (
	"os"
	"strings"
	"testing"
)

func TestPublicWorkflowsKeepCredentialsScopedAndNonPersistent(t *testing.T) {
	for _, path := range []string{"../.github/workflows/ci.yml", "../.github/workflows/update-catalog.yml"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		workflow := string(data)
		if !strings.Contains(workflow, "persist-credentials: false") || !strings.Contains(workflow, "cache: false") {
			t.Errorf("%s persists checkout credentials or module cache", path)
		}
		for _, forbidden := range []string{"secrets.BLOEM_MODULES_TOKEN", "secrets.BLOEM_CATALOG_SOURCE_TOKEN", "gh auth setup-git", "visibility public", "gh release", "docker push", "npm publish", "pull_request_target"} {
			if strings.Contains(workflow, forbidden) {
				t.Errorf("%s contains unwanted credential/publication behavior %q", path, forbidden)
			}
		}
	}
	data, err := os.ReadFile("../.github/workflows/update-catalog.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	pushStep := strings.Index(workflow, "- name: Commit and push catalog changes")
	token := strings.Index(workflow, "secrets.BLOEM_CATALOG_PUSH_TOKEN")
	if pushStep < 0 || token < pushStep || strings.Count(workflow, "secrets.BLOEM_CATALOG_PUSH_TOKEN") != 1 {
		t.Fatal("push credential must appear only in final push step")
	}
}
