package main

import (
	"fmt"
	"regexp"
	"slices"
)

func Validate(reposToProvision []Repo, existingRepos []ArtifactoryRepoDetailsResponse, existingPermissions []ArtifactoryPermissionDetails) (repos []Repo, err error) {
	reposToProvision = validateSharedPermissions(reposToProvision, existingPermissions)

	reposToProvision = validateRepoNames(reposToProvision)

	return reposToProvision, nil
}

func validateSharedPermissions(reposToProvision []Repo, existingPermissions []ArtifactoryPermissionDetails) (repos []Repo) {
	for i := 0; i < len(reposToProvision)-1; i++ {
		repo1 := reposToProvision[i]
		var permissionName1 string
		if repo1.PermissionName == "" {
			permissionName1 = repo1.Name
		} else {
			permissionName1 = repo1.PermissionName
		}

		found := false

		for j := i + 1; j < len(reposToProvision); j++ {
			repo2 := reposToProvision[j]
			var permissionName2 string
			if repo2.PermissionName == "" {
				permissionName2 = repo2.Name
			} else {
				permissionName2 = repo2.PermissionName
			}

			if permissionName1 == permissionName2 {
				if !found {
					fmt.Printf("Warning: Ignoring repo '%s', due to shared permission with repo '%s', permission name: '%s' (new permission/1)\n", repo1.Name, repo2.Name, permissionName1)
					ignoredInvalidRepoCount++
					reposToProvision = slices.Delete(reposToProvision, i, i+1)
					found = true
					i--
					j--
				}

				fmt.Printf("Warning: Ignoring repo '%s', due to shared permission with repo '%s', permission name: '%s' (new permission/2)\n", repo2.Name, repo1.Name, permissionName1)
				ignoredInvalidRepoCount++
				reposToProvision = slices.Delete(reposToProvision, j, j+1)
				j--
			}
		}
	}

	for i := 0; i < len(reposToProvision); i++ {
		repo1 := reposToProvision[i]
		var permissionName1 string
		if repo1.PermissionName == "" {
			permissionName1 = repo1.Name
		} else {
			permissionName1 = repo1.PermissionName
		}

		for _, permission := range existingPermissions {
			if permissionName1 == permission.Name {
				for targetName := range permission.Resources.Artifact.Targets {
					if repo1.Name != targetName {
						fmt.Printf("Warning: Ignoring repo '%s', due to shared permission with repo '%s', permission name: '%s' (existing permission)\n", repo1.Name, targetName, permissionName1)
						ignoredInvalidRepoCount++
						reposToProvision = slices.Delete(reposToProvision, i, i+1)
						break
					}
				}
			}
		}
	}

	return reposToProvision
}

func validateRepoNames(reposToProvision []Repo) []Repo {
	for i := 0; i < len(reposToProvision); i++ {
		repo := reposToProvision[i]
		if repo.Name == "" {
			fmt.Printf("'%s': Warning: Ignoring repo: missing name for repo.\n", repo.Name)
			ignoredInvalidRepoCount++
			reposToProvision = slices.Delete(reposToProvision, i, i+1)
			i--
		}

		if !isValidRepoName(repo.Name) {
			fmt.Printf("'%s': Warning: Ignoring repo: invalid name for repo.\n", repo.Name)
			ignoredInvalidRepoCount++
			reposToProvision = slices.Delete(reposToProvision, i, i+1)
			i--
		}
	}

	return reposToProvision
}

func isValidRepoName(s string) bool {
	if len(s) > 0 && (s[0] == ' ' || s[len(s)-1] == ' ') {
		return false
	}
	pattern := "^[a-zA-Z0-9 -_]+$"
	regex := regexp.MustCompile(pattern)
	return regex.MatchString(s)
}
