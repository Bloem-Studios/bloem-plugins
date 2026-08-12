package main

import "testing"

func TestValidateRepositoryUsesLiteralVondelAllowlist(t *testing.T) {
	want := map[string]struct{}{
		"Vondel-Media/vondel-plugin-metadb":             {},
		"Vondel-Media/vondel-plugin-tmdb":               {},
		"Vondel-Media/vondel-plugin-tvdb":               {},
		"Vondel-Media/vondel-plugin-audiobook-metadata": {},
		"Vondel-Media/vondel-plugin-manga-metadata":     {},
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
		"Vondel-Media/vondel-plugin-ebook-metadata",
		"Vondel-Media/vondel-plugin-autoscan-arr",
		"Vondel-Media/vondel-plugin-tmdb/../../attacker",
		"",
	} {
		if err := validateRepository(repo); err == nil {
			t.Errorf("validateRepository(%q) accepted a repository outside the allowlist", repo)
		}
	}
}
