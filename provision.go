package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
)

var ignoredInvalidRepoFilesCount int
var ignoredDuplicated_RepoCount int
var ignoredInvalidRepoCount int
var ignoredNoDiffRepoCount int
var ignoredInvalidPermissionCount int
var ignoredNoDiffPermissionCount int

func LoadRepoFiles(repofile []string) ([]Repo, error) {
	var allrepos []Repo

	for _, repofile := range repofile {
		repos, err := loadRepoFile(repofile)
		if err != nil {
			return nil, err
		}

		allrepos = append(allrepos, repos...)
	}

	allrepos = removeDups(allrepos)

	return allrepos, nil
}

func loadRepoFile(repofile string) ([]Repo, error) {
	data, err := os.ReadFile(repofile)
	if err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	var repos []Repo

	decoder := json.NewDecoder(strings.NewReader(string(data)))
	errjson := decoder.Decode(&repos)
	if errjson != nil {
		erryaml := yaml.Unmarshal(data, &repos)
		if erryaml != nil {
			return nil, fmt.Errorf("error parsing json/yaml file: %w %w", errjson, erryaml)
		} else {
			if len(repos) == 0 {
				fmt.Printf("Warning: Ignoring empty yaml file: '%s'\n", repofile)
				ignoredInvalidRepoFilesCount++
				return repos, nil
			}

			var node ast.Node
			if err := yaml.Unmarshal(data, &node); err != nil {
				return nil, fmt.Errorf("error parsing yaml file: %w", erryaml)
			}

			type position struct {
				offset int
				line   int
			}
			positions := []position{}
			t := node.GetToken()
			for {
				if t.Value == "-" && t.Position.IndentLevel == 0 {
					positions = append(positions, position{offset: t.Position.Offset, line: t.Position.Line})
				}
				t = t.Next
				if t == nil {
					break
				}
			}
			if len(positions) != len(repos) {
				fmt.Printf("Warning: Ignoring repo file (%s): Number of repos (%d) does not match number of yaml objects (%d)\n",
					repofile, len(repos), len(positions))
				ignoredInvalidRepoFilesCount++
			}
			for i := range repos {
				repos[i].SourceFile = repofile
				repos[i].SourceOffset = positions[i].offset
				repos[i].SourceLine = positions[i].line
			}
		}
	} else {
		if len(repos) == 0 {
			fmt.Printf("Warning: Ignoring empty json file: '%s'\n", repofile)
			ignoredInvalidRepoFilesCount++
			return repos, nil
		}

		decoder = json.NewDecoder(strings.NewReader(string(data)))
		offsets := []int{}

		for {
			t, err := decoder.Token()
			if err != nil {
				break
			}
			if t == json.Delim('{') {
				offsets = append(offsets, int(decoder.InputOffset()-1))
			}
		}
		if len(offsets) != len(repos) {
			fmt.Printf("Warning: Ignoring repo file (%s): Number of repos (%d) does not match number of json objects (%d)\n",
				repofile, len(repos), len(offsets))
			ignoredInvalidRepoFilesCount++
		}
		for i := range repos {
			repos[i].SourceFile = repofile
			repos[i].SourceOffset = offsets[i]
			line := 1
			for j := range data {
				if data[j] == '\n' {
					line++
				}
				if offsets[i] == j {
					break
				}
			}
			repos[i].SourceLine = line
		}
	}

	repos = expandRepos(repos)

	return repos, nil
}

func expandRepos(repos []Repo) []Repo {
	var expandedRepos []Repo

	for i := range repos {
		if repos[i].Name == "" && len(repos[i].Names) == 0 {
			fmt.Printf("Ignoring repo: Repo must have either a name or names\n")
			continue
		}
		if repos[i].Name != "" && len(repos[i].Names) > 0 {
			fmt.Printf("Ignoring repo: Repo must not have both a name (%s) and names (%s)\n", repos[i].Name, strings.Join(repos[i].Names, ", "))
			continue
		}

		if repos[i].Name != "" {
			expandedRepos = append(expandedRepos, repos[i])
		} else {
			names := repos[i].Names
			repos[i].Names = []string{}
			for _, name := range names {
				fmt.Printf("Expanding: '%s' -> '%s'\n", strings.Join(names, "', '"), name)
				repos[i].Name = name
				expandedRepos = append(expandedRepos, repos[i])
			}
		}
	}

	return expandedRepos
}

func removeDups(repos []Repo) []Repo {
	type jsonobject struct {
		Index      int
		SourceFile string
		SourceLine int
	}

	reposToDelete := make(map[string][]jsonobject)

	for i := range repos {
		name := repos[i].Name
		for j := i + 1; j < len(repos); j++ {
			if name == repos[j].Name {
				indices, ok := reposToDelete[name]
				if !ok {
					jo := jsonobject{Index: i, SourceFile: repos[i].SourceFile, SourceLine: repos[i].SourceLine}
					reposToDelete[name] = []jsonobject{jo}
				}
				found := false
				for _, index := range indices {
					if index.Index == j {
						found = true
						break
					}
				}
				if !found {
					jo := jsonobject{Index: j, SourceFile: repos[j].SourceFile, SourceLine: repos[j].SourceLine}
					reposToDelete[name] = append(reposToDelete[name], jo)
				}
			}
		}
	}

	keys := make([]string, 0, len(reposToDelete))
	for key := range reposToDelete {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		positions := make([]string, len(reposToDelete[key]))
		for i, jo := range reposToDelete[key] {
			positions[i] = fmt.Sprintf("%s:%d", jo.SourceFile, jo.SourceLine)
		}
		fmt.Printf("Warning: Ignoring %d repos due to duplicate name. Name: '%s', objects (file:line): %s\n", len(reposToDelete[key]), key, strings.Join(positions, ", "))
	}

	repoIndicesToDelete := []int{}
	sort.Ints(repoIndicesToDelete)
	for _, value := range reposToDelete {
		for _, jo := range value {
			repoIndicesToDelete = append(repoIndicesToDelete, jo.Index)
		}
	}
	sort.Ints(repoIndicesToDelete)

	ignoredDuplicated_RepoCount = len(repoIndicesToDelete)

	for i := len(repoIndicesToDelete) - 1; i >= 0; i-- {
		repos = slices.Delete(repos, repoIndicesToDelete[i], repoIndicesToDelete[i]+1)
	}

	return repos
}

func Provision(
	reposToProvision []Repo,
	allrepos []ArtifactoryRepoDetailsResponse,
	allusers []ArtifactoryUser,
	allgroups []ArtifactoryGroup,
	allpermissiondetails []ArtifactoryPermissionDetails,
	client *http.Client,
	baseurl string,
	token string,
	allowpatterns bool,
	dryRun bool) error {

	fmt.Printf("Repos to provision: %d\n", len(reposToProvision))
	for _, repo := range reposToProvision {
		err := validateRepo(repo, allusers, allgroups)
		if err != nil {
			fmt.Printf("'%s': Warning: Ignoring repo: %v\n", repo.Name, err)
			ignoredInvalidRepoCount++
		} else {
			err := provisionRepo(repo, allrepos, client, baseurl, token, dryRun)
			if err != nil {
				fmt.Printf("'%s': Warning: Ignoring repo: %v\n", repo.Name, err)
				ignoredInvalidRepoCount++
			} else {
				err = provisionPermissionTarget(repo, allusers, allpermissiondetails, client, baseurl, token, allowpatterns, dryRun)
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

	return nil
}

func validateRepo(repo Repo, allusers []ArtifactoryUser, allgroups []ArtifactoryGroup) error {
	if repo.Name == "" {
		return fmt.Errorf("missing name for repo")
	}

	if !isValidRepoName(repo.Name) {
		return fmt.Errorf("invalid name for repo")
	}

	errors := checkUsersAndGroups(repo.Read, allusers, allgroups)
	if len(errors) > 0 {
		for _, err := range errors {
			fmt.Printf("'%s': Permission read: %v\n", repo.Name, err)
		}
		return fmt.Errorf("see errors above for details")
	}
	errors = checkUsersAndGroups(repo.Annotate, allusers, allgroups)
	if len(errors) > 0 {
		for _, err := range errors {
			fmt.Printf("'%s': Permission annotate: %v\n", repo.Name, err)
		}
		return fmt.Errorf("see errors above for details")
	}
	errors = checkUsersAndGroups(repo.Write, allusers, allgroups)
	if len(errors) > 0 {
		for _, err := range errors {
			fmt.Printf("'%s': Permission write: %v\n", repo.Name, err)
		}
		return fmt.Errorf("see errors above for details")
	}
	errors = checkUsersAndGroups(repo.Delete, allusers, allgroups)
	if len(errors) > 0 {
		for _, err := range errors {
			fmt.Printf("'%s': Permission delete: %v\n", repo.Name, err)
		}
		return fmt.Errorf("see errors above for details")
	}
	errors = checkUsersAndGroups(repo.Manage, allusers, allgroups)
	if len(errors) > 0 {
		for _, err := range errors {
			fmt.Printf("'%s': Permission manage: %v\n", repo.Name, err)
		}
		return fmt.Errorf("see errors above for details")
	}

	return nil
}

func provisionRepo(
	repo Repo,
	allrepos []ArtifactoryRepoDetailsResponse,
	client *http.Client,
	baseurl string,
	token string,
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
			//fmt.Printf("'%s': No diff, skipping update...\n", repo.Name)
			ignoredNoDiffRepoCount++
		} else {
			fmt.Printf("'%s': Repo already exists, updating...\n", repo.Name)

			err := updateExistingRepo(repo, existingRepo, client, baseurl, token, dryRun)
			if err != nil {
				return err
			}
		}
	} else {
		fmt.Printf("'%s': Repo does not exist, creating...\n", repo.Name)

		err := createNewRepo(repo, client, baseurl, token, dryRun)
		if err != nil {
			return err
		}
	}

	return nil
}

func updateExistingRepo(
	repo Repo,
	existingRepo *ArtifactoryRepoDetailsResponse,
	client *http.Client,
	baseurl string,
	token string,
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
	repo Repo,
	client *http.Client,
	baseurl string,
	token string,
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
	repo Repo,
	allusers []ArtifactoryUser,
	allpermissiondetails []ArtifactoryPermissionDetails,
	client *http.Client,
	baseurl,
	token string,
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
			//fmt.Printf("'%s': No diff, skipping update...\n", repo.Name)
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

			updateExistingPermission(repo, users, groups, existingPermission, client, baseurl, token, dryRun)
		}
	} else {
		fmt.Printf("'%s': Permission target does not exist, creating...\n", repo.Name)

		createNewPermission(repo, users, groups, client, baseurl, token, dryRun)
	}

	return nil
}

func updateExistingPermission(
	repo Repo,
	users map[string][]string,
	groups map[string][]string,
	existingPermission *ArtifactoryPermissionDetails,
	client *http.Client,
	baseurl,
	token string,
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
		return fmt.Errorf("error updating permission target, error updating request: %w", err)
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
	repo Repo,
	users map[string][]string,
	groups map[string][]string,
	client *http.Client,
	baseurl, token string,
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

			fmt.Printf("'%s': %s diff: '%s': %s -> removed.\n", repo.Name, kind, name, strings.Join(permissionsOld, ", "))
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

	getUsersAndGroupsPermission(repo.Read, "READ", users, groups, alluserstrings)
	getUsersAndGroupsPermission(repo.Annotate, "ANNOTATE", users, groups, alluserstrings)
	getUsersAndGroupsPermission(repo.Write, "WRITE", users, groups, alluserstrings)
	getUsersAndGroupsPermission(repo.Delete, "DELETE", users, groups, alluserstrings)
	getUsersAndGroupsPermission(repo.Manage, "MANAGE", users, groups, alluserstrings)

	if existingPermission != nil {
		addUnknownPermissions(users, (*existingPermission).Resources.Artifact.Actions.Users, repo.Name, "user")
		addUnknownPermissions(groups, (*existingPermission).Resources.Artifact.Actions.Groups, repo.Name, "group")
	}

	return users, groups
}

func addUnknownPermissions(ugsNew map[string][]string, ugsExisting map[string][]string, reponame string, typename string) {
	knownPermissions := []string{"READ", "ANNOTATE", "WRITE", "DELETE", "MANAGE"}

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
	alluserstrings []string) {

	for _, ug := range ugs {
		if slices.Contains(alluserstrings, ug) {
			if users[ug] != nil {
				users[ug] = append(users[ug], permission)
			} else {
				users[ug] = []string{permission}
			}
		} else {
			if groups[ug] != nil {
				groups[ug] = append(groups[ug], permission)
			} else {
				groups[ug] = []string{permission}
			}
		}
	}
}

func checkUsersAndGroups(usersAndGroups []string, users []ArtifactoryUser, groups []ArtifactoryGroup) []error {
	var errors []error

	for _, ug := range usersAndGroups {
		userExists := false
		for _, u := range users {
			if u.Username == ug {
				userExists = true
				break
			}
		}
		groupExists := false
		for _, g := range groups {
			if g.GroupName == ug {
				groupExists = true
				break
			}
		}

		if userExists && groupExists {
			errors = append(errors, fmt.Errorf("both user and group exists with the name: '%s'", ug))
		}

		if !userExists && !groupExists {
			errors = append(errors, fmt.Errorf("no user or group exists with the name: '%s'", ug))
		}
	}

	return errors
}

func isValidRepoName(s string) bool {
	if len(s) > 0 && (s[0] == ' ' || s[len(s)-1] == ' ') {
		return false
	}
	pattern := "^[a-zA-Z0-9 -_]+$"
	regex := regexp.MustCompile(pattern)
	return regex.MatchString(s)
}
