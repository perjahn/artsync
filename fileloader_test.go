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

	repos := LoadRepoFiles([]string{path}, false)
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

	repos := LoadRepoFiles([]string{path}, false)
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

	repos := LoadRepoFiles([]string{path}, false)
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

	repos := LoadRepoFiles([]string{path}, false)
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

	repos := LoadRepoFiles([]string{path}, false)
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}

	shortname := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if repos[0].Name != shortname {
		t.Fatalf("unexpected name: '%s'", repos[0].Name)
	}
}

func TestLoadRepoFile_FilenameFallbackEmpty(t *testing.T) {
	content := ``
	path := writeTempFile(t, "repo-fallback-*.yaml", content)
	defer os.Remove(path)

	repos := LoadRepoFiles([]string{path}, true)
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}

	shortname := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if repos[0].Name != shortname {
		t.Fatalf("unexpected name: '%s'", repos[0].Name)
	}
}

func TestLoadRepoFile_FilenameFallbackAlmostEmpty(t *testing.T) {
	content := `{}`
	path := writeTempFile(t, "repo-fallback-*.yaml", content)
	defer os.Remove(path)

	repos := LoadRepoFiles([]string{path}, true)
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

	repos := LoadRepoFiles([]string{path}, false)
	if len(repos) != 0 {
		t.Fatalf("unexpected non-error for invalid content")
	}
}

func TestLoadRepoFile_JsonWithExtraFields(t *testing.T) {
	content := `{"name":"repo-with-extra","rclass":"local","customField":"custom value","anotherField":42}`
	path := writeTempFile(t, "repo-extra-*.json", content)
	defer os.Remove(path)

	repos := LoadRepoFiles([]string{path}, false)
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].Name != "repo-with-extra" {
		t.Fatalf("unexpected name: '%s'", repos[0].Name)
	}
	if repos[0].Rclass != "local" {
		t.Fatalf("unexpected rclass: '%s'", repos[0].Rclass)
	}

	if repos[0].ExtraFields == nil {
		t.Fatalf("extra fields should not be nil")
	}
	if repos[0].ExtraFields["customField"] != "custom value" {
		t.Fatalf("extra field 'customField' not captured correctly: %v", repos[0].ExtraFields["customField"])
	}
	if repos[0].ExtraFields["anotherField"] != float64(42) {
		t.Fatalf("extra field 'anotherField' not captured correctly: %v", repos[0].ExtraFields["anotherField"])
	}
}

func TestLoadRepoFile_JsonArrayWithExtraFields(t *testing.T) {
	content := `[
	  {"name":"repo1","rclass":"local","customField":"value 1","version":1},
	  {"name":"repo2","packageType":"maven","anotherField":"value 2","count":100}
	]`
	path := writeTempFile(t, "repos-extra-*.json", content)
	defer os.Remove(path)

	repos := LoadRepoFiles([]string{path}, false)
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}

	if repos[0].Name != "repo1" {
		t.Fatalf("expected repo1, got %s", repos[0].Name)
	}
	if repos[0].Rclass != "local" {
		t.Fatalf("expected rclass local, got %s", repos[0].Rclass)
	}
	if repos[0].ExtraFields == nil {
		t.Fatalf("repo1: extra fields should not be nil")
	}
	if repos[0].ExtraFields["customField"] != "value 1" {
		t.Fatalf("repo1: extra field 'customField' not captured correctly: %v", repos[0].ExtraFields["customField"])
	}
	if repos[0].ExtraFields["version"] != float64(1) {
		t.Fatalf("repo1: extra field 'version' not captured correctly: %v", repos[0].ExtraFields["version"])
	}

	if repos[1].Name != "repo2" {
		t.Fatalf("expected repo2, got %s", repos[1].Name)
	}
	if repos[1].PackageType != "maven" {
		t.Fatalf("expected packageType maven, got %s", repos[1].PackageType)
	}
	if repos[1].ExtraFields == nil {
		t.Fatalf("repo2: extra fields should not be nil")
	}
	if repos[1].ExtraFields["anotherField"] != "value 2" {
		t.Fatalf("repo2: extra field 'anotherField' not captured correctly: %v", repos[1].ExtraFields["anotherField"])
	}
	if repos[1].ExtraFields["count"] != float64(100) {
		t.Fatalf("repo2: extra field 'count' not captured correctly: %v", repos[1].ExtraFields["count"])
	}
}

func TestLoadRepoFile_YAMLSingleObjectWithExtraFields(t *testing.T) {
	content := `name: yaml-single-extra
rclass: local
customYamlField: yaml-value
yamlVersion: 2
`
	path := writeTempFile(t, "repo-yaml-extra-*.yaml", content)
	defer os.Remove(path)

	repos := LoadRepoFiles([]string{path}, false)
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].Name != "yaml-single-extra" {
		t.Fatalf("unexpected name: '%s'", repos[0].Name)
	}
	if repos[0].Rclass != "local" {
		t.Fatalf("unexpected rclass: '%s'", repos[0].Rclass)
	}
	if repos[0].ExtraFields == nil {
		t.Fatalf("extra fields should not be nil")
	}
	if repos[0].ExtraFields["customYamlField"] != "yaml-value" {
		t.Fatalf("extra field 'customYamlField' not captured correctly: %v", repos[0].ExtraFields["customYamlField"])
	}
	yamlVersion, ok := repos[0].ExtraFields["yamlVersion"]
	if !ok {
		t.Fatalf("extra field 'yamlVersion' not found")
	}
	if yamlVersion != uint64(2) {
		t.Fatalf("extra field 'yamlVersion' not captured correctly: %v (type: %T)", yamlVersion, yamlVersion)
	}
}

func TestLoadRepoFile_YAMLArrayWithExtraFields(t *testing.T) {
	content := `- name: yaml-repo1
  rclass: local
  customField: value 1
  priority: 10
- name: yaml-repo2
  packageType: docker
  anotherField: value 2
  enabled: true
`
	path := writeTempFile(t, "repos-yaml-extra-*.yaml", content)
	defer os.Remove(path)

	repos := LoadRepoFiles([]string{path}, false)
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}

	if repos[0].Name != "yaml-repo1" {
		t.Fatalf("expected yaml-repo1, got %s", repos[0].Name)
	}
	if repos[0].Rclass != "local" {
		t.Fatalf("expected rclass local, got %s", repos[0].Rclass)
	}
	if repos[0].ExtraFields == nil {
		t.Fatalf("repo1: extra fields should not be nil")
	}
	if repos[0].ExtraFields["customField"] != "value 1" {
		t.Fatalf("repo1: extra field 'customField' not captured correctly: %v", repos[0].ExtraFields["customField"])
	}
	priority, ok := repos[0].ExtraFields["priority"]
	if !ok {
		t.Fatalf("repo1: extra field 'priority' not found")
	}
	if priority != uint64(10) {
		t.Fatalf("repo1: extra field 'priority' not captured correctly: %v (type: %T)", priority, priority)
	}

	if repos[1].Name != "yaml-repo2" {
		t.Fatalf("expected yaml-repo2, got %s", repos[1].Name)
	}
	if repos[1].PackageType != "docker" {
		t.Fatalf("expected packageType docker, got %s", repos[1].PackageType)
	}
	if repos[1].ExtraFields == nil {
		t.Fatalf("repo2: extra fields should not be nil")
	}
	if repos[1].ExtraFields["anotherField"] != "value 2" {
		t.Fatalf("repo2: extra field 'anotherField' not captured correctly: %v", repos[1].ExtraFields["anotherField"])
	}
	if repos[1].ExtraFields["enabled"] != true {
		t.Fatalf("repo2: extra field 'enabled' not captured correctly: %v", repos[1].ExtraFields["enabled"])
	}
}
