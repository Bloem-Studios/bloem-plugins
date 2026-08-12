package catalog

import (
	"os"
	"strings"
	"testing"
)

func TestWorkflowsKeepCredentialsScopedAndNonPersistent(t *testing.T) {
	for _, path := range []string{
		"../.github/workflows/ci.yml",
		"../.github/workflows/update-catalog.yml",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", path, err)
		}
		workflow := string(data)
		if !strings.Contains(workflow, "persist-credentials: false") {
			t.Errorf("%s checkout persists credentials", path)
		}
		if !strings.Contains(workflow, "cache: false") {
			t.Errorf("%s relies on setup-go's dependency cache", path)
		}
		if !strings.Contains(workflow, "go clean -modcache") {
			t.Errorf("%s does not prove private module authentication from a cold module cache", path)
		}
		if strings.Contains(workflow, "gh auth setup-git") {
			t.Errorf("%s persists GitHub authentication in git config", path)
		}
		if !strings.Contains(workflow, "GIT_ASKPASS:") || !strings.Contains(workflow, "Remove private module credentials") {
			t.Errorf("%s does not use temporary module credentials with cleanup", path)
		}
	}

	data, err := os.ReadFile("../.github/workflows/update-catalog.yml")
	if err != nil {
		t.Fatalf("ReadFile(update-catalog.yml) error = %v", err)
	}
	workflow := string(data)
	if got := strings.Count(workflow, "secrets.VONDEL_CATALOG_PUSH_TOKEN"); got != 1 {
		t.Fatalf("write token appears %d times, want exactly once", got)
	}
	pushStep := strings.Index(workflow, "- name: Commit and push catalog changes")
	token := strings.Index(workflow, "secrets.VONDEL_CATALOG_PUSH_TOKEN")
	if pushStep < 0 || token < pushStep {
		t.Fatal("write token is exposed before the final commit/push step")
	}
}
