package main

import (
	"os"
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	tests := []struct {
		repos             []ArtifactoryRepoDetailsResponse
		permissiondetails []ArtifactoryPermissionDetails
		filename          string
		wantErr           bool
		yamlOutput        string
	}{
		{
			[]ArtifactoryRepoDetailsResponse{},
			[]ArtifactoryPermissionDetails{},
			"/tmp/invalid/path/testfile.yaml",
			true,
			""},
		{
			[]ArtifactoryRepoDetailsResponse{
				{
					Key:           "test-repo1",
					Description:   "Test repository 1",
					Rclass:        "local",
					PackageType:   "generic",
					RepoLayoutRef: "simple-default",
				},
			},
			[]ArtifactoryPermissionDetails{},
			"/tmp/testfile1.yaml",
			false,
			`- name: test-repo1
  description: Test repository 1
`},
		{
			[]ArtifactoryRepoDetailsResponse{
				{
					Key:           "test-repo1",
					Description:   "Test repository 1",
					Rclass:        "virtual",
					PackageType:   "maven",
					RepoLayoutRef: "maven-2-default",
				},
			},
			[]ArtifactoryPermissionDetails{},
			"/tmp/testfile1.yaml",
			false,
			`- name: test-repo1
  description: Test repository 1
  rclass: virtual
  packageType: maven
  layout: maven-2-default
`},
		{
			[]ArtifactoryRepoDetailsResponse{
				{
					Key:           "test-repo1",
					Description:   "Test repository",
					Rclass:        "local",
					PackageType:   "generic",
					RepoLayoutRef: "simple-default",
				},
				{
					Key:           "test-repo2",
					Description:   "Test repository",
					Rclass:        "local",
					PackageType:   "generic",
					RepoLayoutRef: "simple-default",
				},
			},
			[]ArtifactoryPermissionDetails{},
			"/tmp/testfile1.yaml",
			false,
			`- names:
  - test-repo1
  - test-repo2
  description: Test repository
`},
	}
	for i, tc := range tests {
		err := Generate(tc.repos, tc.permissiondetails, false, false, false, false, true, false, tc.filename, true)
		if err != nil {
			if !tc.wantErr {
				t.Errorf("Generate (%d/%d): error = %v, wantErr %v",
					i+1, len(tests), err, tc.wantErr)
			}
		} else if tc.wantErr {
			t.Errorf("Generate (%d/%d): error = %v, wantErr %v",
				i+1, len(tests), err, tc.wantErr)
		} else {
			data, err := os.ReadFile(tc.filename)
			if err != nil {
				t.Errorf("Generate (%d/%d): failed to read file %s: %v",
					i+1, len(tests), tc.filename, err)
				continue
			}
			if string(data) != tc.yamlOutput {
				t.Errorf("Generate (%d/%d): output mismatch:\nGot:\n%s\nWant:\n%s",
					i+1, len(tests), string(data), tc.yamlOutput)
			}
		}
	}
}

func TestGenerateSplitEmpty(t *testing.T) {
	tests := []struct {
		repos             []ArtifactoryRepoDetailsResponse
		permissiondetails []ArtifactoryPermissionDetails
		folder            string
		filename          string
		wantErr           bool
		generateyaml      bool
		output            string
	}{
		{
			[]ArtifactoryRepoDetailsResponse{
				{
					Key:           "test-repo2",
					Description:   "",
					Rclass:        "local",
					PackageType:   "generic",
					RepoLayoutRef: "simple-default",
				},
			},
			[]ArtifactoryPermissionDetails{},
			"/tmp/testrepos",
			"/tmp/testrepos/test-repo2.yaml",
			false,
			true,
			``},
		{
			[]ArtifactoryRepoDetailsResponse{
				{
					Key:           "test-repo2",
					Description:   "",
					Rclass:        "local",
					PackageType:   "generic",
					RepoLayoutRef: "simple-default",
				},
			},
			[]ArtifactoryPermissionDetails{},
			"/tmp/testrepos",
			"/tmp/testrepos/test-repo2.yaml",
			false,
			false,
			``},
	}
	for i, tc := range tests {
		err := Generate(tc.repos, tc.permissiondetails, false, false, false, false, true, true, tc.folder, tc.generateyaml)
		if err != nil {
			if !tc.wantErr {
				t.Errorf("Generate (%d/%d): error = %v, wantErr %v",
					i+1, len(tests), err, tc.wantErr)
			}
		} else if tc.wantErr {
			t.Errorf("Generate (%d/%d): error = %v, wantErr %v",
				i+1, len(tests), err, tc.wantErr)
		} else {
			data, err := os.ReadFile(tc.filename)
			if err != nil {
				t.Errorf("Generate (%d/%d): failed to read file %s: %v",
					i+1, len(tests), tc.filename, err)
				continue
			}
			if string(data) != tc.output {
				t.Errorf("Generate (%d/%d): output mismatch:\nGot:\n%s\nWant:\n%s",
					i+1, len(tests), string(data), tc.output)
			}
		}
	}
}

func TestGenerateRenamedPermissions(t *testing.T) {
	repos := []ArtifactoryRepoDetailsResponse{
		{
			Key:           "test-repo-renamed1",
			Rclass:        "local",
			PackageType:   "generic",
			RepoLayoutRef: "simple-default",
		},
		{
			Key:           "test-repo-renamed2",
			Rclass:        "local",
			PackageType:   "generic",
			RepoLayoutRef: "simple-default",
		},
		{
			Key:           "test-repo-renamed4",
			Rclass:        "local",
			PackageType:   "generic",
			RepoLayoutRef: "simple-default",
		},
	}

	permissions := []ArtifactoryPermissionDetails{
		{
			Name: "test-permission1",
			Resources: ArtifactoryPermissionDetailsResources{
				Artifact: ArtifactoryPermissionDetailsArtifact{
					Actions: ArtifactoryPermissionDetailsActions{
						Users: map[string][]string{
							"user1": {"READ", "WRITE"},
						},
						Groups: map[string][]string{},
					},
					Targets: map[string]ArtifactoryPermissionDetailsTarget{
						"test-repo-renamed1": {
							IncludePatterns: []string{"**"},
							ExcludePatterns: []string{},
						},
					},
				},
			},
		},
		{
			Name: "test-permission2",
			Resources: ArtifactoryPermissionDetailsResources{
				Artifact: ArtifactoryPermissionDetailsArtifact{
					Actions: ArtifactoryPermissionDetailsActions{
						Users: map[string][]string{
							"user1": {"READ", "ANNOTATE", "WRITE"},
						},
						Groups: map[string][]string{},
					},
					Targets: map[string]ArtifactoryPermissionDetailsTarget{
						"test-repo-renamed2": {
							IncludePatterns: []string{"**"},
							ExcludePatterns: []string{},
						},
					},
				},
			},
		},
		{
			Name: "test-permission3",
			Resources: ArtifactoryPermissionDetailsResources{
				Artifact: ArtifactoryPermissionDetailsArtifact{
					Actions: ArtifactoryPermissionDetailsActions{
						Users: map[string][]string{
							"user1": {"READ", "WRITE", "DELETE"},
						},
						Groups: map[string][]string{},
					},
					Targets: map[string]ArtifactoryPermissionDetailsTarget{
						"test-repo-renamed2": {
							IncludePatterns: []string{"**"},
							ExcludePatterns: []string{},
						},
					},
				},
			},
		},
		{
			Name: "old-permission-name-for-rename4",
			Resources: ArtifactoryPermissionDetailsResources{
				Artifact: ArtifactoryPermissionDetailsArtifact{
					Actions: ArtifactoryPermissionDetailsActions{
						Users: map[string][]string{
							"user1": {"READ", "WRITE", "MANAGE"},
						},
						Groups: map[string][]string{},
					},
					Targets: map[string]ArtifactoryPermissionDetailsTarget{
						"test-repo-renamed2": {
							IncludePatterns: []string{"**"},
							ExcludePatterns: []string{},
						},
					},
				},
			},
		},
		{
			Name: "test-permission4",
			Resources: ArtifactoryPermissionDetailsResources{
				Artifact: ArtifactoryPermissionDetailsArtifact{
					Actions: ArtifactoryPermissionDetailsActions{
						Users: map[string][]string{
							"user2": {"READ"},
						},
						Groups: map[string][]string{},
					},
					Targets: map[string]ArtifactoryPermissionDetailsTarget{
						"test-repo-renamed4": {
							IncludePatterns: []string{"**"},
							ExcludePatterns: []string{},
						},
						"another-repo": {
							IncludePatterns: []string{"**"},
							ExcludePatterns: []string{},
						},
					},
				},
			},
		},
	}

	filename := "/tmp/testrepos/test-repo-renamed.yaml"

	err := Generate(repos, permissions, false, false, false, true, true, false, filename, true)
	if err != nil {
		t.Errorf("GenerateRenamedPermissions: error = %v", err)
		return
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		t.Errorf("GenerateRenamedPermissions: failed to read file %s: %v", filename, err)
		return
	}

	if strings.Contains(string(data), "test-repo-renamed4") {
		t.Errorf("GenerateRenamedPermissions: repo should not contain test-repo-renamed4 (only referenced by multi-target permissions):\n%s", string(data))
	}
	if strings.Contains(string(data), "MANAGE") {
		t.Errorf("GenerateRenamedPermissions: repo should not contain MANAGE permission:\n%s", string(data))
	}

	want := `- name: test-repo-renamed1
  permissionName: test-permission1
  read:
  - user1
  write:
  - user1
`
	if string(data) != want {
		t.Errorf("GenerateRenamedPermissions: output mismatch:\nGot (%d):\n'%s'\nWant:\n'%s'\n", len(string(data)), string(data), want)
	}
}

func TestEqualStringSlices(t *testing.T) {
	a := []string{"a", "b", "c"}
	b := []string{"c", "b", "a"}
	if !equalStringSlices(a, b) {
		t.Errorf("equalStringSlices (1/2): failed for same elements in different order")
	}
	c := []string{"a", "b"}
	if equalStringSlices(a, c) {
		t.Errorf("equalStringSlices (2/2): failed for different slices")
	}
}

func TestIsClean(t *testing.T) {
	tests := []struct {
		include   []string
		exclude   []string
		wantClean bool
	}{
		{[]string{}, []string{}, false},
		{[]string{}, []string{""}, false},
		{[]string{}, []string{"*"}, false},
		{[]string{}, []string{"**"}, false},
		{[]string{}, []string{"bar"}, false},
		{[]string{""}, []string{}, false},
		{[]string{""}, []string{""}, false},
		{[]string{""}, []string{"*"}, false},
		{[]string{""}, []string{"**"}, false},
		{[]string{""}, []string{"bar"}, false},
		{[]string{"*"}, []string{}, false},
		{[]string{"*"}, []string{""}, false},
		{[]string{"*"}, []string{"*"}, false},
		{[]string{"*"}, []string{"**"}, false},
		{[]string{"*"}, []string{"bar"}, false},
		{[]string{"**"}, []string{}, true},
		{[]string{"**"}, []string{""}, true},
		{[]string{"**"}, []string{"*"}, false},
		{[]string{"**"}, []string{"**"}, false},
		{[]string{"**"}, []string{"bar"}, false},
		{[]string{"foo"}, []string{}, false},
		{[]string{"foo"}, []string{""}, false},
		{[]string{"foo"}, []string{"*"}, false},
		{[]string{"foo"}, []string{"**"}, false},
		{[]string{"foo"}, []string{"bar"}, false},
	}
	for i, tc := range tests {
		target := ArtifactoryPermissionDetailsTarget{
			IncludePatterns: tc.include,
			ExcludePatterns: tc.exclude,
		}
		got := isClean("repo", "perm", target)
		if got != tc.wantClean {
			includes := ""
			for _, inc := range tc.include {
				if includes == "" {
					includes = "'" + inc + "'"
				} else {
					includes += ",'" + inc + "'"
				}
			}
			excludes := ""
			for _, exc := range tc.exclude {
				if excludes == "" {
					excludes = "'" + exc + "'"
				} else {
					excludes += ",'" + exc + "'"
				}
			}
			t.Errorf("isClean (%d/%d): IncludePatterns (%d): %s, ExcludePatterns (%d): %s: isClean() = %v, want %v",
				i+1, len(tests), len(tc.include), includes, len(tc.exclude), excludes, got, tc.wantClean)
		}
	}
}

func TestAddPermissionsToRepo(t *testing.T) {
	repo := &Repo{}
	perms := map[string][]string{
		"user1": {"READ", "WRITE"},
		"user2": {"READ", "ANNOTATE"},
	}
	addPermissionsToRepo(repo, perms)
	addPermissionsToRepo(repo, perms)
	if len(repo.Read) != 2 || ((repo.Read[0] != "user1" || repo.Read[1] != "user2") && (repo.Read[0] != "user2" || repo.Read[1] != "user1")) {
		t.Errorf("addPermissionsToRepo (1/1): failed to add user1 and/or user2 to READ: (%d) '%s'", len(repo.Read), strings.Join(repo.Read, "', '"))
	}
	if len(repo.Annotate) != 1 || repo.Annotate[0] != "user2" {
		t.Errorf("addPermissionsToRepo (1/1): failed to add user2 to ANNOTATE: (%d) '%s'", len(repo.Annotate), strings.Join(repo.Annotate, "', '"))
	}
	if len(repo.Write) != 1 || repo.Write[0] != "user1" {
		t.Errorf("addPermissionsToRepo (1/1): failed to add user1 to WRITE: (%d) '%s'", len(repo.Write), strings.Join(repo.Write, "', '"))
	}
	if len(repo.Delete) != 0 {
		t.Errorf("addPermissionsToRepo (1/1): failed to not add any user to DELETE: (%d) '%s'", len(repo.Delete), strings.Join(repo.Delete, "', '"))
	}
	if len(repo.Manage) != 0 {
		t.Errorf("addPermissionsToRepo (1/1): failed to not add any user to MANAGE: (%d) '%s'", len(repo.Manage), strings.Join(repo.Manage, "', '"))
	}
	if len(repo.Scan) != 0 {
		t.Errorf("addPermissionsToRepo (1/1): failed to not add any user to SCAN: (%d) '%s'", len(repo.Scan), strings.Join(repo.Scan, "', '"))
	}
}
