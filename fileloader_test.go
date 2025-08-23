package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempFile(t *testing.T, pattern, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	name := f.Name()
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		t.Fatalf("write temp file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	return name
}

func TestLoadRepoFile_JSONArray(t *testing.T) {
	content := `[
	  {"name":"repo1"},
	  {"name":"repo2"}
	]`
	path := writeTempFile(t, "repos-*.json", content)
	defer os.Remove(path)

	repos, err := LoadRepoFiles([]string{path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
	if repos[0].Name != "repo1" || repos[1].Name != "repo2" {
		t.Fatalf("unexpected repo names: %v", []string{repos[0].Name, repos[1].Name})
	}
	if filepath.Base(repos[0].SourceFile) != filepath.Base(path) {
		t.Fatalf("source file not set correctly: '%s'", repos[0].SourceFile)
	}
}

func TestLoadRepoFile_JSONSingleObject(t *testing.T) {
	content := `{"name":"single"}`
	path := writeTempFile(t, "repo-single-*.json", content)
	defer os.Remove(path)

	repos, err := LoadRepoFiles([]string{path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].Name != "single" {
		t.Fatalf("unexpected name: '%s'", repos[0].Name)
	}
}

func TestLoadRepoFile_YAMLArray(t *testing.T) {
	content := `- name: yaml1
- name: yaml2
`
	path := writeTempFile(t, "repos-*.yml", content)
	defer os.Remove(path)

	repos, err := LoadRepoFiles([]string{path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
	if repos[0].Name != "yaml1" || repos[1].Name != "yaml2" {
		t.Fatalf("unexpected repo names: %v", []string{repos[0].Name, repos[1].Name})
	}
}

func TestLoadRepoFile_YAMLSingleObject(t *testing.T) {
	content := `name: onlyyaml
`
	path := writeTempFile(t, "repo-single-*.yaml", content)
	defer os.Remove(path)

	repos, err := LoadRepoFiles([]string{path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].Name != "onlyyaml" {
		t.Fatalf("unexpected name: '%s'", repos[0].Name)
	}
}

func TestLoadRepoFile_FilenameFallback(t *testing.T) {
	content := `rclass: local
`
	path := writeTempFile(t, "repo-fallback-*.yaml", content)
	defer os.Remove(path)

	repos, err := LoadRepoFiles([]string{path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}

	shortname := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if repos[0].Name != shortname {
		t.Fatalf("unexpected name: '%s'", repos[0].Name)
	}
}

func TestLoadRepoFile_Invalid(t *testing.T) {
	content := `: not valid :::`
	path := writeTempFile(t, "repo-bad-*.yml", content)
	defer os.Remove(path)

	_, err := LoadRepoFiles([]string{path})
	if err != nil {
		t.Fatalf("unexpected error for invalid content")
	}
}
