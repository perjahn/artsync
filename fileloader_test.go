package main

import (
	"os"
	"path/filepath"
	"slices"
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

func TestLoadRepoFile_ExtraFieldSemicolonInKey(t *testing.T) {
	tests := []struct {
		name    string
		ext     string
		content string
	}{
		{
			name:    "json",
			ext:     "json",
			content: `{"name":"repo-with-semicolon","semi;colon":"value"}`,
		},
		{
			name:    "yaml",
			ext:     "yaml",
			content: "name: repo-with-semicolon\nsemi;colon: value\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeTempFile(t, "repo-semicolon-*."+test.ext, test.content)
			defer os.Remove(path)

			repos := LoadRepoFiles([]string{path}, false)
			if len(repos) != 1 {
				t.Fatalf("expected 1 repo, got %d", len(repos))
			}
			if got := repos[0].ExtraFields["semi;colon"]; !slices.Equal(got, []string{"value"}) {
				t.Fatalf("semicolon field key not preserved: %v", repos[0].ExtraFields)
			}
		})
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
	content := `{"name":"repo-with-extra","rclass":"local","customField":"custom value","anotherField":42,"largeInteger":9007199254740993}`
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
	if len(repos[0].ExtraFields) != 3 {
		t.Fatalf("expected 3 extra fields, got %v", repos[0].ExtraFields)
	}
	if repos[0].ExtraFields["customField"][0] != "custom value" {
		t.Fatalf("extra field 'customField' not captured correctly: %v", repos[0].ExtraFields["customField"])
	}
	if repos[0].ExtraFields["anotherField"][0] != "42" {
		t.Fatalf("numeric extra field 'anotherField' not converted correctly: %v", repos[0].ExtraFields["anotherField"])
	}
	if repos[0].ExtraFields["largeInteger"][0] != "9007199254740993" {
		t.Fatalf("large integer extra field not preserved: %v", repos[0].ExtraFields["largeInteger"])
	}
}

func TestLoadRepoFile_JsonArrayWithExtraFields(t *testing.T) {
	content := `[
	  {"name":"repo1","rclass":"local","customField":"value 1","version":1,"values":[1,"two",true]},
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
	if len(repos[0].ExtraFields) != 3 {
		t.Fatalf("repo1: expected 3 extra fields, got %v", repos[0].ExtraFields)
	}
	if repos[0].ExtraFields["customField"][0] != "value 1" {
		t.Fatalf("repo1: extra field 'customField' not captured correctly: %v", repos[0].ExtraFields["customField"])
	}
	if repos[0].ExtraFields["version"][0] != "1" {
		t.Fatalf("repo1: numeric extra field 'version' not converted correctly: %v", repos[0].ExtraFields["version"])
	}
	if !slices.Equal(repos[0].ExtraFields["values"], []string{"1", "two", "true"}) {
		t.Fatalf("repo1: mixed array not converted correctly: %v", repos[0].ExtraFields["values"])
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
	if len(repos[1].ExtraFields) != 2 {
		t.Fatalf("repo2: expected 2 extra fields, got %v", repos[1].ExtraFields)
	}
	if repos[1].ExtraFields["anotherField"][0] != "value 2" {
		t.Fatalf("repo2: extra field 'anotherField' not captured correctly: %v", repos[1].ExtraFields["anotherField"])
	}
	if repos[1].ExtraFields["count"][0] != "100" {
		t.Fatalf("repo2: numeric extra field 'count' not converted correctly: %v", repos[1].ExtraFields["count"])
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
	if len(repos[0].ExtraFields) != 2 {
		t.Fatalf("expected 2 extra fields, got %v", repos[0].ExtraFields)
	}
	if repos[0].ExtraFields["customYamlField"][0] != "yaml-value" {
		t.Fatalf("extra field 'customYamlField' not captured correctly: %v", repos[0].ExtraFields["customYamlField"])
	}
	if repos[0].ExtraFields["yamlVersion"][0] != "2" {
		t.Fatalf("numeric extra field 'yamlVersion' not converted correctly: %v", repos[0].ExtraFields["yamlVersion"])
	}
}

func TestLoadRepoFile_YAMLArrayWithExtraFields(t *testing.T) {
	content := `- name: yaml-repo1
  rclass: local
  customField: value 1
  priority: 10
  ratio: 1.25
  values: [1, two, true]
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
	if len(repos[0].ExtraFields) != 4 {
		t.Fatalf("repo1: expected 4 extra fields, got %v", repos[0].ExtraFields)
	}
	if repos[0].ExtraFields["customField"][0] != "value 1" {
		t.Fatalf("repo1: extra field 'customField' not captured correctly: %v", repos[0].ExtraFields["customField"])
	}
	if repos[0].ExtraFields["priority"][0] != "10" {
		t.Fatalf("repo1: numeric extra field 'priority' not converted correctly: %v", repos[0].ExtraFields["priority"])
	}
	if repos[0].ExtraFields["ratio"][0] != "1.25" {
		t.Fatalf("repo1: numeric extra field 'ratio' not converted correctly: %v", repos[0].ExtraFields["ratio"])
	}
	if !slices.Equal(repos[0].ExtraFields["values"], []string{"1", "two", "true"}) {
		t.Fatalf("repo1: mixed array not converted correctly: %v", repos[0].ExtraFields["values"])
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
	if len(repos[1].ExtraFields) != 2 {
		t.Fatalf("repo2: expected 2 extra fields, got %v", repos[1].ExtraFields)
	}
	if repos[1].ExtraFields["anotherField"][0] != "value 2" {
		t.Fatalf("repo2: extra field 'anotherField' not captured correctly: %v", repos[1].ExtraFields["anotherField"])
	}
	if repos[1].ExtraFields["enabled"][0] != "true" {
		t.Fatalf("repo2: boolean extra field 'enabled' not converted correctly: %v", repos[1].ExtraFields["enabled"])
	}
}
