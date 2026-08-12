package catalog

import (
	"os"
	"strings"
	"testing"
)

func TestWorkflowsKeepCredentialsScopedAndNonPersistent(t *testing.T) {
	for _, path := range []string{"../.github/workflows/ci.yml", "../.github/workflows/update-catalog.yml"} {
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
		if strings.Contains(workflow, "gh auth setup-git") {
			t.Errorf("%s persists GitHub authentication in git config", path)
		}
	}

	data, err := os.ReadFile("../.github/workflows/update-catalog.yml")
	if err != nil {
		t.Fatalf("ReadFile(update-catalog.yml) error = %v", err)
	}
	workflow := string(data)
	checkout := strings.Index(workflow, "actions/checkout@")
	moduleToken := strings.Index(workflow, "secrets.VONDEL_MODULES_TOKEN")
	if moduleToken < 0 || checkout < 0 || moduleToken > checkout {
		t.Fatal("private SDK token is not isolated in the checkout-free prefetch job")
	}
	if got := strings.Count(workflow, "secrets.VONDEL_MODULES_TOKEN"); got != 1 {
		t.Fatalf("private SDK token appears %d times, want exactly once", got)
	}
	if !strings.Contains(workflow, "actions/upload-artifact@") || !strings.Contains(workflow, "actions/download-artifact@") {
		t.Fatal("sanitized private SDK cache does not cross the job boundary as an artifact")
	}
	if got := strings.Count(workflow, "secrets.VONDEL_CATALOG_SOURCE_TOKEN"); got != 1 {
		t.Fatalf("source token appears %d times, want exactly once", got)
	}
	updateStep := strings.Index(workflow, "- name: Update catalog manifest")
	sourceToken := strings.Index(workflow, "secrets.VONDEL_CATALOG_SOURCE_TOKEN")
	if updateStep < 0 || sourceToken < updateStep {
		t.Fatal("source token is exposed outside the catalog update step")
	}
	if got := strings.Count(workflow, "secrets.VONDEL_CATALOG_PUSH_TOKEN"); got != 1 {
		t.Fatalf("write token appears %d times, want exactly once", got)
	}
	pushStep := strings.Index(workflow, "- name: Commit and push catalog changes")
	token := strings.Index(workflow, "secrets.VONDEL_CATALOG_PUSH_TOKEN")
	if pushStep < 0 || token < pushStep {
		t.Fatal("write token is exposed before the final commit/push step")
	}
	for _, forbidden := range []string{"visibility public", "gh release", "docker push", "npm publish", "repository-dispatch"} {
		if strings.Contains(strings.ToLower(workflow), forbidden) {
			t.Fatalf("update workflow contains forbidden publication behavior %q", forbidden)
		}
	}
}
