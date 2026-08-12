package main

import "testing"

func TestValidateRepositoryUsesLiteralVondelAllowlist(t *testing.T) {
	allowed := []string{
		"Vondel-Media/vondel-plugin-metadb",
		"Vondel-Media/vondel-plugin-tmdb",
		"Vondel-Media/vondel-plugin-tvdb",
		"Vondel-Media/vondel-plugin-audiobook-metadata",
		"Vondel-Media/vondel-plugin-manga-metadata",
	}
	for _, repo := range allowed {
		if err := validateRepository(repo); err != nil {
			t.Errorf("validateRepository(%q) error = %v", repo, err)
		}
	}

	for _, repo := range []string{
		"Silo-Server/silo-plugin-metadata-tmdb",
		"Vondel-Media/vondel-plugin-ebook-metadata",
		"Vondel-Media/vondel-plugin-tmdb/../../attacker",
		"",
	} {
		if err := validateRepository(repo); err == nil {
			t.Errorf("validateRepository(%q) accepted a repository outside the allowlist", repo)
		}
	}
}
