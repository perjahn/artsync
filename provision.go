package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
)

var ignoredInvalidRepoFilesCount int
var ignoredDuplicated_RepoCount int
var ignoredInvalidRepoCount int
var ignoredNoDiffRepoCount int
var ignoredInvalidPermissionCount int
var ignoredNoDiffPermissionCount int
var ignoredDuplicatePermissionCount int
var importGroupsCount int
var createUsersCount int

func Provision(
	client *http.Client,
	baseurl string,
	token string,
	reposToProvision []Repo,
	allrepos []ArtifactoryRepoDetailsResponse,
	allusers []ArtifactoryUser,
	allgroups []ArtifactoryGroup,
	allpermissiondetails []ArtifactoryPermissionDetails,
	allowpatterns bool,
	createUsers bool,
	ldapConfig LdapConfig,
	dryRun bool) error {

	fmt.Printf("Repos to provision: %d\n", len(reposToProvision))
	for _, repo := range reposToProvision {
		var err error
		allusers, allgroups, err = validateRepo(client, baseurl, token, repo, allusers, allgroups, createUsers, ldapConfig, dryRun)
		if err != nil {
			fmt.Printf("'%s': Warning: Ignoring repo: %v\n", repo.Name, err)
			ignoredInvalidRepoCount++
		} else {
			err := provisionRepo(client, baseurl, token, repo, allrepos, dryRun)
			if err != nil {
				fmt.Printf("'%s': Warning: Ignoring repo: %v\n", repo.Name, err)
				ignoredInvalidRepoCount++
			} else {
				err = provisionPermissionTarget(client, baseurl, token, repo, allusers, allpermissiondetails, allowpatterns, dryRun)
				if err != nil {
					fmt.Printf("'%s': Warning: Ignoring repo's permission target: %v\n", repo.Name, err)
					ignoredInvalidPermissionCount++
				}
			}
		}
	}

	fmt.Printf("Ignored invalid repo files: %d\n", ignoredInvalidRepoFilesCount)
	fmt.Printf("Ignored duplicated repos: %d\n", ignoredDuplicated_RepoCount)
	fmt.Printf("Ignored invalid repos: %d\n", ignoredInvalidRepoCount)
	fmt.Printf("Ignored no diff repos: %d\n", ignoredNoDiffRepoCount)
	fmt.Printf("Ignored invalid permission targets: %d\n", ignoredInvalidPermissionCount)
	fmt.Printf("Ignored no diff permission targets: %d\n", ignoredNoDiffPermissionCount)
	fmt.Printf("Ignored duplicate permissions: %d\n", ignoredDuplicatePermissionCount)
	fmt.Printf("Imported groups: %d\n", importGroupsCount)
	fmt.Printf("Created users: %d\n", createUsersCount)

	return nil
}

func validateRepo(
	client *http.Client,
	baseurl string,
	token string,
	repo Repo,
	allusers []ArtifactoryUser,
	allgroups []ArtifactoryGroup,
	createUsers bool,
	ldapConfig LdapConfig,
	dryRun bool) ([]ArtifactoryUser, []ArtifactoryGroup, error) {

	if repo.Name == "" {
		return nil, nil, fmt.Errorf("missing name for repo")
	}

	if !isValidRepoName(repo.Name) {
		return nil, nil, fmt.Errorf("invalid name for repo")
	}

	hasErrors := false

	for _, check := range []struct {
		values []string
		perm   string
	}{
		{repo.Read, "read"},
		{repo.Annotate, "annotate"},
		{repo.Write, "write"},
		{repo.Delete, "delete"},
		{repo.Manage, "manage"},
		{repo.Scan, "scan"},
	} {
		var errs []error
		allusers, allgroups, errs = checkUsersAndGroups(client, baseurl, token, check.values, allusers, allgroups, createUsers, ldapConfig, dryRun)
		if len(errs) > 0 {
			for _, err := range errs {
				fmt.Printf("'%s': Permission %s: %v\n", repo.Name, check.perm, err)
			}
			hasErrors = true
		}
	}

	if hasErrors {
		return nil, nil, fmt.Errorf("see errors above for details")
	}

	return allusers, allgroups, nil
}

func provisionRepo(
	client *http.Client,
	baseurl string,
	token string,
	repo Repo,
	allrepos []ArtifactoryRepoDetailsResponse,
	dryRun bool) error {

	if repo.Rclass == "" {
		repo.Rclass = "local"
	}
	if repo.PackageType == "" {
		repo.PackageType = "generic"
	}
	if repo.Layout == "" {
		repo.Layout = "simple-default"
	}

	var existingRepo *ArtifactoryRepoDetailsResponse
	for _, r := range allrepos {
		if r.Key == repo.Name {
			existingRepo = &r
			break
		}
	}

	if existingRepo != nil {
		diff := false
		ignore := false
		if existingRepo.Description != repo.Description {
			diff = true
		}
		if !strings.EqualFold(existingRepo.Rclass, repo.Rclass) {
			fmt.Printf("'%s': Ignoring repo, cannot update rclass/type: diff: '%s' -> '%s'\n", repo.Name, existingRepo.Rclass, repo.Rclass)
			ignore = true
		}
		if !strings.EqualFold(existingRepo.PackageType, repo.PackageType) {
			fmt.Printf("'%s': Ignoring repo, cannot update package type: diff: '%s' -> '%s'\n", repo.Name, existingRepo.PackageType, repo.PackageType)
			ignore = true
		}
		if !strings.EqualFold(existingRepo.RepoLayoutRef, repo.Layout) {
			diff = true
		}
		if ignore {
			return fmt.Errorf("see errors above for details")
		}
		if !diff {
			ignoredNoDiffRepoCount++
		} else {
			fmt.Printf("'%s': Repo already exists, updating...\n", repo.Name)

			err := updateExistingRepo(client, baseurl, token, repo, existingRepo, dryRun)
			if err != nil {
				return err
			}
		}
	} else {
		fmt.Printf("'%s': Repo does not exist, creating...\n", repo.Name)

		err := createNewRepo(client, baseurl, token, repo, dryRun)
		if err != nil {
			return err
		}
	}

	return nil
}

func updateExistingRepo(
	client *http.Client,
	baseurl string,
	token string,
	repo Repo,
	existingRepo *ArtifactoryRepoDetailsResponse,
	dryRun bool,
) error {

	if existingRepo.Description != repo.Description {
		fmt.Printf("'%s': Description diff: '%s' -> '%s'\n", repo.Name, existingRepo.Description, repo.Description)
	}
	if !strings.EqualFold(existingRepo.RepoLayoutRef, repo.Layout) {
		fmt.Printf("'%s': Layout diff: '%s' -> '%s'\n", repo.Name, existingRepo.RepoLayoutRef, repo.Layout)
	}

	url := fmt.Sprintf("%s/artifactory/api/repositories/%s", baseurl, repo.Name)

	artifactoryrepo := ArtifactoryRepoRequest{
		Key:           repo.Name,
		Description:   repo.Description,
		Rclass:        repo.Rclass,
		PackageType:   repo.PackageType,
		RepoLayoutRef: repo.Layout,
	}

	json, err := json.Marshal(artifactoryrepo)
	if err != nil {
		return fmt.Errorf("error updating repo, error generating json: %w", err)
	}
	req, err := http.NewRequest("POST", url, strings.NewReader(string(json)))
	if err != nil {
		return fmt.Errorf("error updating repo, error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	if !dryRun {
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("error updating repo: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			fmt.Printf("Key: '%s'\n", repo.Name)
			fmt.Printf("Url: '%s'\n", url)
			fmt.Printf("Unexpected status: '%s'\n", resp.Status)
			fmt.Printf("Request body: '%s'\n", req.Body)
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("Response body: '%s'\n", body)
			return fmt.Errorf("error updating repo")
		} else {
			fmt.Printf("'%s': Updated repo successfully.\n", repo.Name)
		}
	}

	return nil
}

func createNewRepo(
	client *http.Client,
	baseurl string,
	token string,
	repo Repo,
	dryRun bool,
) error {

	url := fmt.Sprintf("%s/artifactory/api/repositories/%s", baseurl, repo.Name)

	artifactoryrepo := ArtifactoryRepoRequest{
		Key:           repo.Name,
		Description:   repo.Description,
		Rclass:        repo.Rclass,
		PackageType:   repo.PackageType,
		RepoLayoutRef: repo.Layout,
	}

	json, err := json.Marshal(artifactoryrepo)
	if err != nil {
		return fmt.Errorf("error creating repo, error generating json: %w", err)
	}
	req, err := http.NewRequest("PUT", url, strings.NewReader(string(json)))
	if err != nil {
		return fmt.Errorf("error creating repo, error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	fields := []string{}
	if repo.Description != "" {
		fields = append(fields, fmt.Sprintf("Description: '%s'", repo.Description))
	}
	if repo.Rclass != "" {
		fields = append(fields, fmt.Sprintf("Rclass: '%s'", repo.Rclass))
	}
	if repo.PackageType != "" {
		fields = append(fields, fmt.Sprintf("PackageType: '%s'", repo.PackageType))
	}
	if repo.Layout != "" {
		fields = append(fields, fmt.Sprintf("Layout: '%s'", repo.Layout))
	}
	fmt.Printf("'%s': %s\n", repo.Name, strings.Join(fields, ", "))

	if !dryRun {
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("error creating repo: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			fmt.Printf("Key: '%s'\n", repo.Name)
			fmt.Printf("Url: '%s'\n", url)
			fmt.Printf("Unexpected status: '%s'\n", resp.Status)
			fmt.Printf("Request body: '%s'\n", req.Body)
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("Response body: '%s'\n", body)
			return fmt.Errorf("error creating repo")
		} else {
			fmt.Printf("'%s': Created repo successfully.\n", repo.Name)
		}
	}

	return nil
}

func provisionPermissionTarget(
	client *http.Client,
	baseurl string,
	token string,
	repo Repo,
	allusers []ArtifactoryUser,
	allpermissiondetails []ArtifactoryPermissionDetails,
	allowpatterns bool,
	dryRun bool) error {

	var existingPermission *ArtifactoryPermissionDetails
	for _, p := range allpermissiondetails {
		if p.Name == repo.Name {
			existingPermission = &p
			break
		}
	}

	users, groups := convertUsersAndGroups(repo, allusers, existingPermission)

	if existingPermission != nil {
		diff := false
		if !equalStringSliceMaps(existingPermission.Resources.Artifact.Actions.Users, users) {
			diff = true
		}
		if !equalStringSliceMaps(existingPermission.Resources.Artifact.Actions.Groups, groups) {
			diff = true
		}
		if !diff {
			ignoredNoDiffPermissionCount++
		} else {
			for _, target := range existingPermission.Resources.Artifact.Targets {
				include := target.IncludePatterns
				exclude := target.ExcludePatterns
				if !allowpatterns && (!slices.Equal(include, []string{"**"}) || (len(exclude) != 0 && !slices.Equal(exclude, []string{""}))) {
					return fmt.Errorf("'%s': Ignoring permission target due to existing non-default include/exclude patterns: permission target: '%s', include: '%s', exclude: '%s' %d",
						repo.Name, existingPermission.Name, include, exclude, len(exclude))
				}
			}

			fmt.Printf("'%s': Permission target already exists, updating...\n", repo.Name)

			updateExistingPermission(client, baseurl, token, repo, users, groups, existingPermission, dryRun)
		}
	} else {
		fmt.Printf("'%s': Permission target does not exist, creating...\n", repo.Name)

		createNewPermission(client, baseurl, token, repo, users, groups, dryRun)
	}

	return nil
}

func updateExistingPermission(
	client *http.Client,
	baseurl string,
	token string,
	repo Repo,
	users map[string][]string,
	groups map[string][]string,
	existingPermission *ArtifactoryPermissionDetails,
	dryRun bool) error {

	if !equalStringSliceMaps(existingPermission.Resources.Artifact.Actions.Users, users) {
		printDiff(repo, existingPermission.Resources.Artifact.Actions.Users, users, "Users")
	}
	if !equalStringSliceMaps(existingPermission.Resources.Artifact.Actions.Groups, groups) {
		printDiff(repo, existingPermission.Resources.Artifact.Actions.Groups, groups, "Groups")
	}

	url := fmt.Sprintf("%s/access/api/v2/permissions/%s/artifact", baseurl, repo.Name)

	targets := make(map[string]ArtifactoryPermissionDetailsTarget)
	targets[repo.Name] = ArtifactoryPermissionDetailsTarget{}

	artifactorypermissiontarget := ArtifactoryPermissionDetailsArtifact{
		Actions: ArtifactoryPermissionDetailsActions{
			Users:  users,
			Groups: groups,
		},
		Targets: targets,
	}

	json, err := json.Marshal(artifactorypermissiontarget)

	if err != nil {
		return fmt.Errorf("error updating permission target, error generating json: %w", err)
	}
	req, err := http.NewRequest("PUT", url, strings.NewReader(string(json)))
	if err != nil {
		return fmt.Errorf("error updating permission target, error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	if !dryRun {
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("error updating permission target: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			fmt.Printf("Key: '%s'\n", repo.Name)
			fmt.Printf("Url: '%s'\n", url)
			fmt.Printf("Unexpected status: '%s'\n", resp.Status)
			fmt.Printf("Request body: '%s'\n", req.Body)
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("Response body: '%s'\n", body)
			return fmt.Errorf("error updating permission target")
		} else {
			fmt.Printf("'%s': Updated permission target successfully.\n", repo.Name)
		}
	}

	return nil
}

func createNewPermission(
	client *http.Client,
	baseurl string,
	token string,
	repo Repo,
	users map[string][]string,
	groups map[string][]string,
	dryRun bool) error {

	printDiff(repo, map[string][]string{}, users, "Users")
	printDiff(repo, map[string][]string{}, groups, "Groups")

	url := fmt.Sprintf("%s/access/api/v2/permissions", baseurl)

	targets := make(map[string]ArtifactoryPermissionDetailsTarget)
	targets[repo.Name] = ArtifactoryPermissionDetailsTarget{
		IncludePatterns: []string{"**"},
		ExcludePatterns: []string{},
	}

	artifactorypermissiontarget := ArtifactoryPermissionDetails{
		Name: repo.Name,
		Resources: ArtifactoryPermissionDetailsResources{
			Artifact: ArtifactoryPermissionDetailsArtifact{
				Actions: ArtifactoryPermissionDetailsActions{
					Users:  users,
					Groups: groups,
				},
				Targets: targets,
			},
		},
	}

	json, err := json.Marshal(artifactorypermissiontarget)

	if err != nil {
		return fmt.Errorf("error creating permission target, error generating json: %w", err)
	}
	req, err := http.NewRequest("POST", url, strings.NewReader(string(json)))
	if err != nil {
		return fmt.Errorf("error creating permission target, error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	if !dryRun {
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("error creating permission target: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 201 {
			fmt.Printf("Key: '%s'\n", repo.Name)
			fmt.Printf("Url: '%s'\n", url)
			fmt.Printf("Unexpected status: '%s'\n", resp.Status)
			fmt.Printf("Request body: '%s'\n", req.Body)
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("Response body: '%s'\n", body)
			return fmt.Errorf("error creating permission target")
		} else {
			fmt.Printf("'%s': Created permission target successfully.\n", repo.Name)
		}
	}

	return nil
}

func printDiff(repo Repo, old map[string][]string, new map[string][]string, kind string) {
	var names []string
	for name := range old {
		names = append(names, name)
	}
	for name := range new {
		names = append(names, name)
	}
	names = uniqueStrings(names)
	slices.Sort(names)

	for _, name := range names {
		foundOld := false
		if _, ok := old[name]; ok {
			foundOld = true
		}
		foundNew := false
		if _, ok := new[name]; ok {
			foundNew = true
		}

		if foundOld && foundNew {
			permissionsOld := old[name]
			slices.Sort(permissionsOld)
			permissionsNew := new[name]
			slices.Sort(permissionsNew)

			if !slices.Equal(permissionsOld, permissionsNew) {
				fmt.Printf("'%s': %s diff: '%s': %s -> %s\n", repo.Name, kind, name, strings.Join(permissionsOld, ", "), strings.Join(permissionsNew, ", "))
			}
		} else if foundOld && !foundNew {
			permissionsOld := old[name]
			slices.Sort(permissionsOld)

			fmt.Printf("'%s': %s diff: '%s': %s -> removed\n", repo.Name, kind, name, strings.Join(permissionsOld, ", "))
		} else if !foundOld && foundNew {
			permissionsNew := new[name]
			slices.Sort(permissionsNew)

			fmt.Printf("'%s': %s diff: '%s': notexist -> %s\n", repo.Name, kind, name, strings.Join(permissionsNew, ", "))
		}
	}
}

func equalStringSliceMaps(a map[string][]string, b map[string][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, vA := range a {
		vB, ok := b[k]
		if !ok {
			return false
		}
		slices.Sort(vA)
		slices.Sort(vB)
		if !slices.Equal(vA, vB) {
			return false
		}
	}
	return true
}

func uniqueStrings(input []string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, v := range input {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}

func convertUsersAndGroups(
	repo Repo,
	allusers []ArtifactoryUser,
	existingPermission *ArtifactoryPermissionDetails) (map[string][]string, map[string][]string) {

	alluserstrings := make([]string, len(allusers))
	for i, user := range allusers {
		alluserstrings[i] = user.Username
	}

	users := make(map[string][]string)
	groups := make(map[string][]string)

	getUsersAndGroupsPermission(repo.Read, "READ", users, groups, alluserstrings, repo.Name)
	getUsersAndGroupsPermission(repo.Annotate, "ANNOTATE", users, groups, alluserstrings, repo.Name)
	getUsersAndGroupsPermission(repo.Write, "WRITE", users, groups, alluserstrings, repo.Name)
	getUsersAndGroupsPermission(repo.Delete, "DELETE", users, groups, alluserstrings, repo.Name)
	getUsersAndGroupsPermission(repo.Manage, "MANAGE", users, groups, alluserstrings, repo.Name)
	getUsersAndGroupsPermission(repo.Scan, "SCAN", users, groups, alluserstrings, repo.Name)

	if existingPermission != nil {
		addUnknownPermissions(users, (*existingPermission).Resources.Artifact.Actions.Users, repo.Name, "user")
		addUnknownPermissions(groups, (*existingPermission).Resources.Artifact.Actions.Groups, repo.Name, "group")
	}

	return users, groups
}

func addUnknownPermissions(ugsNew map[string][]string, ugsExisting map[string][]string, reponame string, typename string) {
	knownPermissions := []string{"READ", "ANNOTATE", "WRITE", "DELETE", "MANAGE", "SCAN"}

	for ug, permissions := range ugsExisting {
		for _, permission := range permissions {
			if !slices.Contains(knownPermissions, permission) {
				fmt.Printf("'%s': Keeping unknown permission '%s' for %s '%s'\n", reponame, permission, typename, ug)
				if ugsNew[ug] != nil {
					ugsNew[ug] = append(ugsNew[ug], permission)
				} else {
					ugsNew[ug] = []string{permission}
				}
			}
		}
	}
}

func getUsersAndGroupsPermission(
	ugs []string,
	permission string,
	users map[string][]string,
	groups map[string][]string,
	alluserstrings []string,
	reponame string) {

	for _, ug := range ugs {
		if slices.Contains(alluserstrings, ug) {
			if users[ug] != nil {
				if slices.Contains(users[ug], permission) {
					fmt.Printf("'%s': Ignoring duplicate permission '%s' for user '%s'\n", reponame, permission, ug)
					ignoredDuplicatePermissionCount++
				} else {
					users[ug] = append(users[ug], permission)
				}
			} else {
				users[ug] = []string{permission}
			}
		} else {
			if groups[ug] != nil {
				if slices.Contains(groups[ug], permission) {
					fmt.Printf("'%s': Ignoring duplicate permission '%s' for group '%s'\n", reponame, permission, ug)
					ignoredDuplicatePermissionCount++
				} else {
					groups[ug] = append(groups[ug], permission)
				}
			} else {
				groups[ug] = []string{permission}
			}
		}
	}
}

func checkUsersAndGroups(
	client *http.Client,
	baseurl string,
	token string,
	usersAndGroups []string,
	allusers []ArtifactoryUser,
	allgroups []ArtifactoryGroup,
	createUsers bool,
	ldapConfig LdapConfig,
	dryRun bool) ([]ArtifactoryUser, []ArtifactoryGroup, []error) {

	var errs []error
	importGroups := ldapConfig.Importgroups

	for _, ug := range usersAndGroups {
		userExists := false
		for _, u := range allusers {
			if u.Username == ug {
				userExists = true
				break
			}
		}
		groupExists := false
		for _, g := range allgroups {
			if g.GroupName == ug {
				groupExists = true
				break
			}
		}

		if userExists && groupExists {
			errs = append(errs, fmt.Errorf("both user and group exists with the name: '%s'", ug))
			continue
		}

		if !userExists && !groupExists {
			var errGroup error
			var errUser error

			if importGroups {
				fmt.Printf("Importing group: '%s'\n", ug)
				errGroup = ImportGroup(
					client,
					baseurl,
					ldapConfig.Username,
					ldapConfig.Password,
					ug,
					ldapConfig.ldapsettings,
					ldapConfig.ldapgroupsettings,
					ldapConfig.Ldapgroupsettingsname,
					ldapConfig.Ldapusername,
					ldapConfig.Ldappassword,
					dryRun)
				if errGroup == nil {
					allgroups = append(allgroups, ArtifactoryGroup{GroupName: ug})
					importGroupsCount++
				}
			}

			if (createUsers && importGroups && errGroup != nil) || (createUsers && !importGroups) {
				fmt.Printf("Creating user: '%s'\n", ug)
				errUser = createUser(client, baseurl, token, ug, dryRun)
				if errUser == nil {
					allusers = append(allusers, ArtifactoryUser{Username: ug})
					errGroup = nil
					createUsersCount++
				}
			}

			if errGroup != nil || errUser != nil {
				joined := errors.Join(errGroup, errUser)
				errs = append(errs, fmt.Errorf("no user or group exists with the name: '%s': %w", ug, joined))
			}
		}
	}

	return allusers, allgroups, errs
}

func createUser(
	client *http.Client,
	baseurl string,
	token string,
	username string,
	dryRun bool) error {

	url := fmt.Sprintf("%s/access/api/v2/users", baseurl)

	var email string
	if at := strings.Index(username, "@"); at != -1 {
		email = username
	} else {
		email = fmt.Sprintf("%s@example.com", username)
	}

	artifactoryUserRequest := ArtifactoryUserRequest{
		Username:                 username,
		Email:                    email,
		InternalPasswordDisabled: true,
	}

	json, err := json.Marshal(artifactoryUserRequest)

	if err != nil {
		return fmt.Errorf("error creating user, error generating json: %w", err)
	}
	req, err := http.NewRequest("POST", url, strings.NewReader(string(json)))
	if err != nil {
		return fmt.Errorf("error creating user, error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	if !dryRun {
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("error creating user: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 201 {
			fmt.Printf("Username: '%s'\n", username)
			fmt.Printf("Url: '%s'\n", url)
			fmt.Printf("Unexpected status: '%s'\n", resp.Status)
			fmt.Printf("Request body: '%s'\n", req.Body)
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("Response body: '%s'\n", body)
			return fmt.Errorf("error creating user")
		} else {
			fmt.Printf("'%s': Created user successfully.\n", username)
		}
	}

	return nil
}

func isValidRepoName(s string) bool {
	if len(s) > 0 && (s[0] == ' ' || s[len(s)-1] == ' ') {
		return false
	}
	pattern := "^[a-zA-Z0-9 -_]+$"
	regex := regexp.MustCompile(pattern)
	return regex.MatchString(s)
}
