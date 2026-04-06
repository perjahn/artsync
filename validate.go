package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

func Validate(reposToProvision []Repo, existingRepos []ArtifactoryRepoDetailsResponse, existingPermissions []ArtifactoryPermissionDetails) (repos []Repo, err error) {
	reposToProvision = validateSharedPermissions(reposToProvision, existingPermissions)

	reposToProvision = validateRepoNames(reposToProvision)

	reposToProvision = validateCasePermissions(reposToProvision)

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

		if repo1.Rclass == "remote" {
			permissionName1 = permissionName1 + "-cache"
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
			fmt.Printf("Warning: Ignoring repo '%s', due to missing name for repo.\n", repo.Name)
			ignoredInvalidRepoCount++
			reposToProvision = slices.Delete(reposToProvision, i, i+1)
			i--
		}

		if !isValidRepoName(repo.Name) {
			fmt.Printf("Warning: Ignoring repo '%s', due to invalid name for repo.\n", repo.Name)
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

func validateCasePermissions(reposToProvision []Repo) []Repo {
	for i := range reposToProvision {
		repo := reposToProvision[i]
		var offendingValues []string

		permSlices := [][]string{repo.Read, repo.Annotate, repo.Write, repo.Delete, repo.Manage, repo.Scan}
		for _, permSlice := range permSlices {
			for _, value := range permSlice {
				if strings.ToLower(value) != value {
					offendingValues = append(offendingValues, value)
				}
			}
		}

		if len(offendingValues) > 0 {
			fmt.Printf("Warning: Converting permissions for repo '%s' to lowercase: %v -> %v\n", repo.Name, offendingValues, toLowerSlice(offendingValues))
			repo.Read = toLowerSlice(repo.Read)
			repo.Annotate = toLowerSlice(repo.Annotate)
			repo.Write = toLowerSlice(repo.Write)
			repo.Delete = toLowerSlice(repo.Delete)
			repo.Manage = toLowerSlice(repo.Manage)
			repo.Scan = toLowerSlice(repo.Scan)
			reposToProvision[i] = repo
		}
	}
	return reposToProvision
}

func toLowerSlice(slice []string) []string {
	result := make([]string, len(slice))
	for i, v := range slice {
		result[i] = strings.ToLower(v)
	}
	return result
}
