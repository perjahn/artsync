package main

import (
	"slices"
	"testing"
)

func TestValidationSharedPermissions(t *testing.T) {
	reposToProvision := []Repo{
		{
			Name:           "repo1",
			PermissionName: "permission2",
		},
		{
			Name:           "repo2",
			PermissionName: "permission1",
		},
		{
			Name:           "repo3",
			PermissionName: "",
		},
		{
			Name:           "repo4",
			PermissionName: "repo3",
		},
		{
			Name:           "repo5",
			PermissionName: "repo3",
		},
		{
			Name:           "repo7",
			PermissionName: "repo6",
		},
		{
			Name:           "repo6",
			PermissionName: "",
		},
		{
			Name:           "repo8",
			PermissionName: "permission8",
		},
	}

	existingRepos := []ArtifactoryRepoDetailsResponse{}
	existingPermissions := []ArtifactoryPermissionDetails{}

	ignoredInvalidRepoCount = 0
	reposToProvision, err := Validate(reposToProvision, existingRepos, existingPermissions)
	if err != nil {
		t.Errorf("ValidationSharedPermissions: error = %v", err)
	}

	wantCount := 3
	if len(reposToProvision) != wantCount {
		t.Errorf("ValidationSharedPermissions: got %d, want %d repos to provision", len(reposToProvision), wantCount)
	} else {
		wantRepoNames := []string{"repo1", "repo2", "repo8"}
		if reposToProvision[0].Name != wantRepoNames[0] {
			t.Errorf("ValidationSharedPermissions: unexpected repo name: got: '%s', want: '%s'", reposToProvision[0].Name, wantRepoNames[0])
		}
		if reposToProvision[1].Name != wantRepoNames[1] {
			t.Errorf("ValidationSharedPermissions: unexpected repo name: got: '%s', want: '%s'", reposToProvision[1].Name, wantRepoNames[1])
		}
		if reposToProvision[2].Name != wantRepoNames[2] {
			t.Errorf("ValidationSharedPermissions: unexpected repo name: got: '%s', want: '%s'", reposToProvision[2].Name, wantRepoNames[2])
		}
	}
	wantIgnoreCount := 5
	if ignoredInvalidRepoCount != wantIgnoreCount {
		t.Errorf("ValidationSharedPermissions: unexpected ignore count: got: '%d', want: '%d'", ignoredInvalidRepoCount, wantIgnoreCount)
	}
}

func TestValidationSharedExistingPermissions(t *testing.T) {
	reposToProvision := []Repo{
		{
			Name:           "repo1",
			PermissionName: "permission2",
		},
		{
			Name:           "repo2",
			PermissionName: "permission1",
		},
		{
			Name:           "repo3",
			PermissionName: "permission3",
		},
	}
	existingRepos := []ArtifactoryRepoDetailsResponse{}
	existingPermissions := []ArtifactoryPermissionDetails{
		{
			Name: "permission1",
			Resources: ArtifactoryPermissionDetailsResources{
				Artifact: ArtifactoryPermissionDetailsArtifact{
					Actions: ArtifactoryPermissionDetailsActions{},
					Targets: map[string]ArtifactoryPermissionDetailsTarget{
						"repo1": {},
					},
				}},
		},
	}

	ignoredInvalidRepoCount = 0
	reposToProvision, err := Validate(reposToProvision, existingRepos, existingPermissions)
	if err != nil {
		t.Errorf("ValidationSharedExistingPermissions: error = %v", err)
	}

	wantCount := 2
	if len(reposToProvision) != wantCount {
		t.Errorf("ValidationSharedExistingPermissions: expected %d repos to provision, got %d", wantCount, len(reposToProvision))
	} else {
		wantRepoNames := []string{"repo1", "repo3"}
		if reposToProvision[0].Name != wantRepoNames[0] {
			t.Errorf("ValidationSharedExistingPermissions: unexpected repo name: want: '%s', was: '%s'", wantRepoNames[0], reposToProvision[0].Name)
		}
		if reposToProvision[1].Name != wantRepoNames[1] {
			t.Errorf("ValidationSharedExistingPermissions: unexpected repo name: want: '%s', was: '%s'", wantRepoNames[1], reposToProvision[1].Name)
		}
	}
	wantIgnoreCount := 1
	if ignoredInvalidRepoCount != wantIgnoreCount {
		t.Errorf("ValidationSharedExistingPermissions: unexpected ignore count: want: '%d', was: '%d'", wantIgnoreCount, ignoredInvalidRepoCount)
	}
}

func TestValidateCasePermissionsWithUppercase(t *testing.T) {
	reposToProvision := []Repo{
		{
			Name: "repo1",
			Read: []string{"user1", "User2"},
		},
		{
			Name:   "repo2",
			Read:   []string{"user3"},
			Write:  []string{"admin"},
			Delete: []string{"DeleteUser"},
		},
		{
			Name: "repo3",
			Read: []string{"user4"},
		},
	}

	existingRepos := []ArtifactoryRepoDetailsResponse{}
	existingPermissions := []ArtifactoryPermissionDetails{}

	ignoredInvalidRepoCount = 0
	reposToProvision, err := Validate(reposToProvision, existingRepos, existingPermissions)
	if err != nil {
		t.Errorf("ValidateCasePermissions: error = %v", err)
	}

	wantCount := 3
	if len(reposToProvision) != wantCount {
		t.Errorf("ValidateCasePermissions: expected %d repos to provision, got %d", wantCount, len(reposToProvision))
	} else {
		// Check repo1 permissions converted to lowercase
		if !slices.Equal(reposToProvision[0].Read, []string{"user1", "user2"}) {
			t.Errorf("ValidateCasePermissions: repo1 Read not converted: got %v", reposToProvision[0].Read)
		}
		// Check repo2 permissions converted to lowercase
		if !slices.Equal(reposToProvision[1].Delete, []string{"deleteuser"}) {
			t.Errorf("ValidateCasePermissions: repo2 Delete not converted: got %v", reposToProvision[1].Delete)
		}
		// Check repo3 unchanged
		if !slices.Equal(reposToProvision[2].Read, []string{"user4"}) {
			t.Errorf("ValidateCasePermissions: repo3 Read should be unchanged: got %v", reposToProvision[2].Read)
		}
	}
	wantIgnoreCount := 0
	if ignoredInvalidRepoCount != wantIgnoreCount {
		t.Errorf("ValidateCasePermissions: unexpected ignore count: want: '%d', got: '%d'", wantIgnoreCount, ignoredInvalidRepoCount)
	}
}

func TestValidateCasePermissionsAllLowercase(t *testing.T) {
	reposToProvision := []Repo{
		{
			Name:   "repo1",
			Read:   []string{"user1", "user2"},
			Write:  []string{"admin"},
			Delete: []string{"deleteuser"},
		},
		{
			Name:   "repo2",
			Read:   []string{"user3"},
			Manage: []string{"manager"},
		},
	}

	existingRepos := []ArtifactoryRepoDetailsResponse{}
	existingPermissions := []ArtifactoryPermissionDetails{}

	ignoredInvalidRepoCount = 0
	reposToProvision, err := Validate(reposToProvision, existingRepos, existingPermissions)
	if err != nil {
		t.Errorf("ValidateCasePermissionsAllLowercase: error = %v", err)
	}

	wantCount := 2
	if len(reposToProvision) != wantCount {
		t.Errorf("ValidateCasePermissionsAllLowercase: expected %d repos to provision, got %d", wantCount, len(reposToProvision))
	}
	wantIgnoreCount := 0
	if ignoredInvalidRepoCount != wantIgnoreCount {
		t.Errorf("ValidateCasePermissionsAllLowercase: unexpected ignore count: want: '%d', got: '%d'", wantIgnoreCount, ignoredInvalidRepoCount)
	}
}
