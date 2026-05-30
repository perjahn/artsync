package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strings"
)

type Statistics struct {
	IgnoredInvalidRepoFilesCount    int
	IgnoredDuplicatedRepoCount      int
	IgnoredInvalidRepoCount         int
	IgnoredNoDiffRepoCount          int
	IgnoredInvalidPermissionCount   int
	IgnoredNoDiffPermissionCount    int
	IgnoredDuplicatePermissionCount int
	CreatedUserCount                int
	ImportedGroupCount              int
	CreatedRepoCount                int
	UpdatedRepoCount                int
	CreatedPermissionCount          int
	UpdatedPermissionCount          int
}

var stats Statistics

type repoDiff struct {
	repo               Repo
	hasRepoDiff        bool
	hasPermDiff        bool
	existingRepo       *ArtifactoryRepoDetailsResponse
	existingPermission *ArtifactoryPermissionDetails
	permUsers          map[string][]string
	permGroups         map[string][]string
}

// for testing, to be able to check output in a controlled manner
func ClearStats() {
	stats = Statistics{}
}

func Provision(
	client *http.Client,
	baseurl string,
	token string,
	reposToProvision []Repo,
	allrepos []ArtifactoryRepoDetailsResponse,
	allusers []ArtifactoryUser,
	allgroups []ArtifactoryGroup,
	allpermissiondetails []ArtifactoryPermissionDetails,
	showDiff bool,
	allowpatterns bool,
	ldapConfig LdapConfig,
	propertiesConfig PropertiesConfig,
	dryRun bool) error {

	if ldapConfig.ImportUsersAndGroups {
		accessToken, refreshToken, err := getUITokens(client, baseurl, ldapConfig.ArtifactoryUsername, ldapConfig.ArtifactoryPassword)
		if err != nil {
			return fmt.Errorf("unable to obtain UI tokens for Artifactory, cannot import ldap groups: %w", err)
		}

		reposToProvision, allusers, allgroups = provisionUsersAndGroups(client, baseurl, token, reposToProvision, allusers, allgroups, ldapConfig, accessToken, refreshToken, dryRun)
	}

	reposWithDiffs := findReposWithDiffs(reposToProvision, allrepos, allpermissiondetails, allusers, allowpatterns)

	fmt.Printf("Repos to provision: %d/%d\n", len(reposWithDiffs), len(reposToProvision))

	for _, diffRepo := range reposWithDiffs {
		if diffRepo.hasRepoDiff {
			err := provisionRepo(client, baseurl, token, diffRepo.repo, diffRepo.existingRepo, dryRun)
			if err != nil {
				fmt.Printf("'%s': Warning: Ignoring repo: %v\n", diffRepo.repo.Name, err)
				stats.IgnoredInvalidRepoCount++
			}
		}

		if diffRepo.hasPermDiff {
			err := provisionPermissionTarget(client, baseurl, token, diffRepo.repo, diffRepo.existingPermission, diffRepo.permUsers, diffRepo.permGroups, showDiff, dryRun)
			if err != nil {
				fmt.Printf("'%s': Warning: Ignoring repo's permission target: %v\n", diffRepo.repo.Name, err)
				stats.IgnoredInvalidPermissionCount++
			}
		}

		if diffRepo.hasRepoDiff || diffRepo.hasPermDiff {
			err := setRepoProperties(client, baseurl, token, diffRepo.repo, propertiesConfig, dryRun)
			if err != nil {
				fmt.Printf("'%s': Warning: Error setting properties: %v\n", diffRepo.repo.Name, err)
			}
		}
	}

	fmt.Printf("Results:\n")
	fmt.Printf("  Ignored invalid repo files: %d\n", stats.IgnoredInvalidRepoFilesCount)
	fmt.Printf("  Ignored duplicated repos: %d\n", stats.IgnoredDuplicatedRepoCount)
	fmt.Printf("  Ignored invalid repos: %d\n", stats.IgnoredInvalidRepoCount)
	fmt.Printf("  Ignored no diff repos: %d\n", stats.IgnoredNoDiffRepoCount)
	fmt.Printf("  Ignored invalid permission targets: %d\n", stats.IgnoredInvalidPermissionCount)
	fmt.Printf("  Ignored no diff permission targets: %d\n", stats.IgnoredNoDiffPermissionCount)
	fmt.Printf("  Ignored duplicate permissions: %d\n", stats.IgnoredDuplicatePermissionCount)

	fmt.Printf("  Created users: %d\n", stats.CreatedUserCount)
	fmt.Printf("  Imported groups: %d\n", stats.ImportedGroupCount)

	fmt.Printf("  Created repos: %d\n", stats.CreatedRepoCount)
	fmt.Printf("  Updated repos: %d\n", stats.UpdatedRepoCount)
	fmt.Printf("  Created permission targets: %d\n", stats.CreatedPermissionCount)
	fmt.Printf("  Updated permission targets: %d\n", stats.UpdatedPermissionCount)

	return nil
}

func findReposWithDiffs(
	reposToProvision []Repo,
	allrepos []ArtifactoryRepoDetailsResponse,
	allpermissiondetails []ArtifactoryPermissionDetails,
	allusers []ArtifactoryUser,
	allowpatterns bool) []repoDiff {

	var reposWithDiffs []repoDiff

	for _, repo := range reposToProvision {
		hasRepoDiff, existingRepo := hasRepoDiff(repo, allrepos)
		hasPermDiff, permDiffInfo := hasPermissionTargetDiff(repo, allpermissiondetails, allusers, allowpatterns)

		if !hasRepoDiff {
			stats.IgnoredNoDiffRepoCount++
		}
		if !hasPermDiff {
			stats.IgnoredNoDiffPermissionCount++
		}

		if hasRepoDiff || hasPermDiff {
			reposWithDiffs = append(reposWithDiffs, repoDiff{
				repo:               repo,
				hasRepoDiff:        hasRepoDiff,
				hasPermDiff:        hasPermDiff,
				existingRepo:       existingRepo,
				existingPermission: permDiffInfo.Existing,
				permUsers:          permDiffInfo.Users,
				permGroups:         permDiffInfo.Groups,
			})
		}
	}

	return reposWithDiffs
}

func provisionUsersAndGroups(
	client *http.Client,
	baseurl string,
	token string,
	reposToProvision []Repo,
	allusers []ArtifactoryUser,
	allgroups []ArtifactoryGroup,
	ldapConfig LdapConfig,
	accessToken string,
	refreshToken string,
	dryRun bool) ([]Repo, []ArtifactoryUser, []ArtifactoryGroup) {

	var usersAndGroups []string

	for _, repo := range reposToProvision {
		for _, repoUsersAndGroups := range [][]string{
			repo.Read,
			repo.Annotate,
			repo.Write,
			repo.Delete,
			repo.Manage,
			repo.Scan,
		} {
			usersAndGroups = append(usersAndGroups, repoUsersAndGroups...)
		}
	}

	slices.Sort(usersAndGroups)

	fmt.Printf("Got %d total user/group permissions to provision.\n", len(usersAndGroups))

	for i := 0; i < len(usersAndGroups); i++ {
		if i > 0 && strings.EqualFold(usersAndGroups[i], usersAndGroups[i-1]) {
			usersAndGroups = slices.Delete(usersAndGroups, i, i+1)
			i--
		}
	}

	fmt.Printf("Got %d unique users/groups to provision.\n", len(usersAndGroups))
	fmt.Printf("Got %d existing users.\n", len(allusers))
	fmt.Printf("Got %d existing groups.\n", len(allgroups))

	var newUsersAndGroups []string

	for _, ug := range usersAndGroups {
		userExists := slices.ContainsFunc(allusers, func(u ArtifactoryUser) bool {
			return u.Username == ug
		})
		groupExists := slices.ContainsFunc(allgroups, func(g ArtifactoryGroup) bool {
			return g.GroupName == ug
		})

		if !userExists && !groupExists {
			log.Printf("No existing user or group found, will attempt to import user/group: '%s'\n", ug)
			newUsersAndGroups = append(newUsersAndGroups, ug)
		} else if userExists && groupExists {
			var repos []string
			for i := 0; i < len(reposToProvision); i++ {
				repo := reposToProvision[i]
				if slices.Contains(repo.Read, ug) ||
					slices.Contains(repo.Annotate, ug) ||
					slices.Contains(repo.Write, ug) ||
					slices.Contains(repo.Delete, ug) ||
					slices.Contains(repo.Manage, ug) ||
					slices.Contains(repo.Scan, ug) {
					repos = append(repos, repo.Name)
					stats.IgnoredInvalidRepoCount++
					reposToProvision = slices.Delete(reposToProvision, i, i+1)
					i--
				}
			}
			fmt.Printf("Warning: Both user and group found, will ignore repos with user/group '%s': %v\n", ug, repos)
		}
	}

	for _, ug := range newUsersAndGroups {
		var errGroup, errUser error
		var importedGroup, createdUser bool

		importedGroup, errGroup = ImportGroup(
			client,
			baseurl,
			ldapConfig.LdapUsername,
			ldapConfig.LdapPassword,
			ug,
			ldapConfig.Ldapsettings,
			ldapConfig.Ldapgroupsettings,
			accessToken,
			refreshToken,
			dryRun)
		if errGroup != nil {
			fmt.Printf("Importing group '%s' failed: %v\n", ug, errGroup)
		}
		if errGroup == nil && importedGroup {
			fmt.Printf("Imported group '%s'\n", ug)
			allgroups = append(allgroups, ArtifactoryGroup{GroupName: ug})
			stats.ImportedGroupCount++
		}

		if !importedGroup && errGroup == nil {
			createdUser, errUser = CreateUser(
				client,
				baseurl,
				token,
				ldapConfig.LdapUsername,
				ldapConfig.LdapPassword,
				ug,
				ldapConfig.Ldapsettings,
				dryRun)
			if errUser != nil {
				fmt.Printf("Creating user '%s' failed: %v\n", ug, errUser)
			}
			if errUser == nil && createdUser {
				fmt.Printf("Created user '%s'\n", ug)
				allusers = append(allusers, ArtifactoryUser{Username: ug})
				stats.CreatedUserCount++
			}
		}

		if !importedGroup && !createdUser {
			for i := 0; i < len(reposToProvision); i++ {
				repo := reposToProvision[i]
				if slices.Contains(repo.Read, ug) ||
					slices.Contains(repo.Annotate, ug) ||
					slices.Contains(repo.Write, ug) ||
					slices.Contains(repo.Delete, ug) ||
					slices.Contains(repo.Manage, ug) ||
					slices.Contains(repo.Scan, ug) {
					fmt.Printf("'%s': Ignoring repo due to missing user/group: '%s'\n", repo.Name, ug)

					stats.IgnoredInvalidRepoCount++
					reposToProvision = slices.Delete(reposToProvision, i, i+1)
					i--
				}
			}
		}
	}

	fmt.Printf("Created users: %d, imported groups: %d. (%d/%d)\n", stats.CreatedUserCount, stats.ImportedGroupCount, stats.CreatedUserCount+stats.ImportedGroupCount, len(newUsersAndGroups))

	return reposToProvision, allusers, allgroups
}

func hasRepoDiff(repo Repo, allrepos []ArtifactoryRepoDetailsResponse) (bool, *ArtifactoryRepoDetailsResponse) {
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
		if existingRepo.Rclass != repo.Rclass {
			fmt.Printf("'%s': Ignoring repo, cannot update rclass/type: diff: '%s' -> '%s'\n", repo.Name, existingRepo.Rclass, repo.Rclass)
			ignore = true
		}
		if existingRepo.PackageType != repo.PackageType {
			fmt.Printf("'%s': Ignoring repo, cannot update package type: diff: '%s' -> '%s'\n", repo.Name, existingRepo.PackageType, repo.PackageType)
			ignore = true
		}
		if existingRepo.RepoLayoutRef != repo.Layout {
			diff = true
		}
		if repo.Rclass == "remote" && existingRepo.Url != repo.Url {
			diff = true
		}
		if repo.Rclass == "virtual" && repo.Repositories != nil && slices.Compare(existingRepo.Repositories, repo.Repositories) != 0 {
			diff = true
		}
		if ignore {
			return false, nil
		}
		if diff {
			return true, existingRepo
		}
		return false, existingRepo
	}

	return true, nil
}

func provisionRepo(
	client *http.Client,
	baseurl string,
	token string,
	repo Repo,
	existingRepo *ArtifactoryRepoDetailsResponse,
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

	if existingRepo == nil {
		fmt.Printf("'%s': Repo does not exist, creating...\n", repo.Name)

		err := createNewRepo(client, baseurl, token, repo, dryRun)
		if err != nil {
			return err
		}
	} else {
		fmt.Printf("'%s': Repo already exists, updating...\n", repo.Name)

		err := updateExistingRepo(client, baseurl, token, repo, *existingRepo, dryRun)
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
	existingRepo ArtifactoryRepoDetailsResponse,
	dryRun bool,
) error {

	if existingRepo.Description != repo.Description {
		fmt.Printf("'%s': Description diff: '%s' -> '%s'\n", repo.Name, existingRepo.Description, repo.Description)
	}
	if existingRepo.RepoLayoutRef != repo.Layout {
		fmt.Printf("'%s': Layout diff: '%s' -> '%s'\n", repo.Name, existingRepo.RepoLayoutRef, repo.Layout)
	}
	if repo.Rclass == "remote" && existingRepo.Url != repo.Url {
		fmt.Printf("'%s': Url diff: '%s' -> '%s'\n", repo.Name, existingRepo.Url, repo.Url)
	}

	url := fmt.Sprintf("%s/artifactory/api/repositories/%s", baseurl, repo.Name)

	artifactoryrepo := ArtifactoryRepoRequest{
		Key:           repo.Name,
		Description:   repo.Description,
		Rclass:        repo.Rclass,
		PackageType:   repo.PackageType,
		RepoLayoutRef: repo.Layout,
	}
	if repo.Rclass == "remote" {
		artifactoryrepo.Url = repo.Url
	}
	if repo.Rclass == "virtual" && repo.Repositories != nil {
		artifactoryrepo.Repositories = repo.Repositories
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
	stats.UpdatedRepoCount++

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
	if repo.Rclass == "remote" {
		artifactoryrepo.Url = repo.Url
	}
	if repo.Rclass == "virtual" && repo.Repositories != nil {
		artifactoryrepo.Repositories = repo.Repositories
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
	if repo.Rclass == "remote" && repo.Url != "" {
		fields = append(fields, fmt.Sprintf("Url: '%s'", repo.Url))
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
	stats.CreatedRepoCount++

	return nil
}

func setRepoProperties(
	client *http.Client,
	baseurl string,
	token string,
	repo Repo,
	propertiesConfig PropertiesConfig,
	dryRun bool,
) error {

	if !propertiesConfig.SetProperties {
		return nil
	}

	if repo.Rclass == "virtual" {
		return nil
	}

	repoName := repo.Name
	if repo.Rclass == "remote" {
		repoName = repo.Name + "-cache"
	}

	desiredProps := make(map[string]string)

	if propertiesConfig.Url != "" {
		desiredProps[fmt.Sprintf("%s.url", propertiesConfig.Prefix)] = propertiesConfig.Url
	}

	for key, value := range repo.ExtraFields {
		if value == nil || value == "" || value == false || value == 0 {
			continue
		}

		var valueStr string
		switch v := value.(type) {
		case []any:
			var items []string
			for _, item := range v {
				items = append(items, fmt.Sprintf("%v", item))
			}
			valueStr = strings.Join(items, ",")
		default:
			valueStr = fmt.Sprintf("%v", value)
		}

		desiredProps[fmt.Sprintf("%s.%s", propertiesConfig.Prefix, key)] = valueStr
	}

	if len(desiredProps) == 0 {
		return nil
	}

	currentProps, err := getRepoProperties(client, baseurl, token, repoName)
	if err != nil {
		return err
	}

	needsUpdate := false
	if len(desiredProps) != len(currentProps) {
		needsUpdate = true
	} else {
		for key, desiredVal := range desiredProps {
			if currentVal, ok := currentProps[key]; !ok || currentVal != desiredVal {
				needsUpdate = true
				break
			}
		}
	}

	if needsUpdate {
		var properties []string
		for key, value := range desiredProps {
			properties = append(properties, fmt.Sprintf("%s=%s", key, value))
		}

		url := fmt.Sprintf("%s/artifactory/api/storage/%s?properties=%s&recursive=0", baseurl, repoName, strings.Join(properties, ";"))

		req, err := http.NewRequest("PUT", url, strings.NewReader(""))
		if err != nil {
			return fmt.Errorf("error creating properties request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)

		if !dryRun {
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("error setting repo properties: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != 204 && resp.StatusCode != 200 {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("error setting repo properties, unexpected status: %s, body: %s", resp.Status, body)
			}
		}

		fmt.Printf("'%s': Set properties: %s.\n", repoName, strings.Join(properties, ", "))
	}

	prefix := propertiesConfig.Prefix + "."
	var unused []string
	for key := range currentProps {
		if strings.HasPrefix(key, prefix) {
			if _, ok := desiredProps[key]; !ok {
				unused = append(unused, key)
			}
		}
	}
	if len(unused) > 0 {
		deleteUrl := fmt.Sprintf("%s/artifactory/api/storage/%s?properties=%s&recursive=0", baseurl, repoName, strings.Join(unused, ";"))
		reqDel, err := http.NewRequest("DELETE", deleteUrl, nil)
		if err != nil {
			return fmt.Errorf("error creating delete properties request: %w", err)
		}
		reqDel.Header.Set("Authorization", "Bearer "+token)
		if !dryRun {
			resp, err := client.Do(reqDel)
			if err != nil {
				return fmt.Errorf("error deleting unused properties: %w", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 204 && resp.StatusCode != 200 {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("error deleting unused properties, unexpected status: %s, body: %s", resp.Status, body)
			}
		}
		fmt.Printf("'%s': Deleted unused properties: %s.\n", repoName, strings.Join(unused, ", "))
	}

	return nil
}

type ArtifactoryPropertiesResponse struct {
	Properties map[string][]string `json:"properties"`
}

func getRepoProperties(
	client *http.Client,
	baseurl string,
	token string,
	repoName string,
) (map[string]string, error) {
	url := fmt.Sprintf("%s/artifactory/api/storage/%s?properties", baseurl, repoName)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error getting properties request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error getting repo properties: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 404 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error getting repo properties, unexpected status: %s, body: %s", resp.Status, body)
	}

	if resp.StatusCode == 404 {
		return make(map[string]string), nil
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := strings.TrimSpace(string(body))

	if bodyStr == "" {
		return make(map[string]string), nil
	}

	var propsResponse ArtifactoryPropertiesResponse
	err = json.Unmarshal(body, &propsResponse)
	if err == nil && propsResponse.Properties != nil {
		props := make(map[string]string)
		for key, values := range propsResponse.Properties {
			if len(values) > 0 {
				props[key] = strings.Join(values, ";")
			}
		}
		return props, nil
	}

	return make(map[string]string), nil
}

type PermissionDiffInfo struct {
	Existing *ArtifactoryPermissionDetails
	Users    map[string][]string
	Groups   map[string][]string
}

func hasPermissionTargetDiff(repo Repo, allpermissiondetails []ArtifactoryPermissionDetails, allusers []ArtifactoryUser, allowpatterns bool) (bool, PermissionDiffInfo) {
	if repo.Rclass == "virtual" {
		return false, PermissionDiffInfo{}
	}

	var permissionName string
	if repo.PermissionName != "" {
		permissionName = repo.PermissionName
	} else {
		permissionName = repo.Name
	}

	var existingPermission *ArtifactoryPermissionDetails
	for _, p := range allpermissiondetails {
		if p.Name == permissionName {
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
			return false, PermissionDiffInfo{}
		}
		for _, target := range existingPermission.Resources.Artifact.Targets {
			include := target.IncludePatterns
			exclude := target.ExcludePatterns
			if !allowpatterns && (!slices.Equal(include, []string{"**"}) || (len(exclude) != 0 && !slices.Equal(exclude, []string{""}))) {
				return false, PermissionDiffInfo{}
			}
		}
		return true, PermissionDiffInfo{Existing: existingPermission, Users: users, Groups: groups}
	}

	return true, PermissionDiffInfo{Users: users, Groups: groups}
}

func provisionPermissionTarget(
	client *http.Client,
	baseurl string,
	token string,
	repo Repo,
	existingPermission *ArtifactoryPermissionDetails,
	permUsers map[string][]string,
	permGroups map[string][]string,
	showDiff bool,
	dryRun bool) error {

	if repo.Rclass == "virtual" {
		return nil
	}

	permissionName := repo.Name
	if repo.PermissionName != "" {
		permissionName = repo.PermissionName
	}

	repoName := repo.Name
	if repo.Rclass == "remote" {
		repoName = repo.Name + "-cache"
	}

	if existingPermission == nil {
		fmt.Printf("'%s': Permission target does not exist, creating...\n", permissionName)

		return createNewPermission(client, baseurl, token, repo, repoName, permissionName, permUsers, permGroups, dryRun)
	} else {
		fmt.Printf("'%s': Permission target already exists, updating...\n", permissionName)

		return updateExistingPermission(client, baseurl, token, repo, repoName, permissionName, permUsers, permGroups, *existingPermission, showDiff, dryRun)
	}
}

func updateExistingPermission(
	client *http.Client,
	baseurl string,
	token string,
	repo Repo,
	repoName string,
	permissionName string,
	users map[string][]string,
	groups map[string][]string,
	existingPermission ArtifactoryPermissionDetails,
	showDiff bool,
	dryRun bool) error {

	if !equalStringSliceMaps(existingPermission.Resources.Artifact.Actions.Users, users) {
		printDiffPermissions(repo, permissionName, existingPermission.Resources.Artifact.Actions.Users, users, "Users")
	}
	if !equalStringSliceMaps(existingPermission.Resources.Artifact.Actions.Groups, groups) {
		printDiffPermissions(repo, permissionName, existingPermission.Resources.Artifact.Actions.Groups, groups, "Groups")
	}

	url := fmt.Sprintf("%s/access/api/v2/permissions/%s/artifact", baseurl, permissionName)

	artifactorypermissiontarget := ArtifactoryPermissionDetailsArtifact{
		Actions: ArtifactoryPermissionDetailsActions{
			Users:  users,
			Groups: groups,
		},
		Targets: map[string]ArtifactoryPermissionDetailsTarget{
			repoName: {
				existingPermission.Resources.Artifact.Targets[repo.Name].IncludePatterns,
				existingPermission.Resources.Artifact.Targets[repo.Name].ExcludePatterns,
			},
		},
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

	artifactjsonsource := GetArtifactJson(existingPermission.JsonSource)
	if showDiff {
		difftext, _ := PrintDiff(artifactjsonsource, string(json), true)
		fmt.Printf("%s", difftext)
	}
	difftext, _ := PrintDiff(artifactjsonsource, string(json), false)
	log.Printf("Permission (existing) diff: '%s'\n%s", permissionName, difftext)

	if !dryRun {
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("error updating permission target: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			fmt.Printf("Key: '%s'\n", permissionName)
			fmt.Printf("Url: '%s'\n", url)
			fmt.Printf("Unexpected status: '%s'\n", resp.Status)
			fmt.Printf("Request body: '%s'\n", req.Body)
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("Response body: '%s'\n", body)
			return fmt.Errorf("error updating permission target")
		} else {
			fmt.Printf("'%s': Updated permission target successfully.\n", permissionName)
		}
	}
	stats.UpdatedPermissionCount++

	return nil
}

func createNewPermission(
	client *http.Client,
	baseurl string,
	token string,
	repo Repo,
	repoName string,
	permissionName string,
	users map[string][]string,
	groups map[string][]string,
	dryRun bool) error {

	printDiffPermissions(repo, permissionName, map[string][]string{}, users, "Users")
	printDiffPermissions(repo, permissionName, map[string][]string{}, groups, "Groups")

	url := fmt.Sprintf("%s/access/api/v2/permissions", baseurl)

	artifactorypermissiontarget := ArtifactoryPermissionDetails{
		Name: permissionName,
		Resources: ArtifactoryPermissionDetailsResources{
			Artifact: ArtifactoryPermissionDetailsArtifact{
				Actions: ArtifactoryPermissionDetailsActions{
					Users:  users,
					Groups: groups,
				},
				Targets: map[string]ArtifactoryPermissionDetailsTarget{
					repoName: {
						IncludePatterns: []string{"**"},
						ExcludePatterns: []string{},
					},
				},
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

	log.Printf("Permission (new): '%s'\n%s\n", permissionName, string(json))

	if !dryRun {
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("error creating permission target: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 201 {
			fmt.Printf("Key: '%s'\n", permissionName)
			fmt.Printf("Url: '%s'\n", url)
			fmt.Printf("Unexpected status: '%s'\n", resp.Status)
			fmt.Printf("Request body: '%s'\n", req.Body)
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("Response body: '%s'\n", body)
			return fmt.Errorf("error creating permission target")
		} else {
			fmt.Printf("'%s': Created permission target successfully.\n", permissionName)
		}
	}
	stats.CreatedPermissionCount++

	return nil
}

func printDiffPermissions(repo Repo, permissionName string, old map[string][]string, new map[string][]string, kind string) {
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
				fmt.Printf("'%s': permission: '%s': %s diff: '%s': %s -> %s\n", repo.Name, permissionName, kind, name, strings.Join(permissionsOld, ", "), strings.Join(permissionsNew, ", "))
			}
		} else if foundOld && !foundNew {
			permissionsOld := old[name]
			slices.Sort(permissionsOld)

			fmt.Printf("'%s': permission: '%s': %s diff: '%s': %s -> removed\n", repo.Name, permissionName, kind, name, strings.Join(permissionsOld, ", "))
		} else if !foundOld && foundNew {
			permissionsNew := new[name]
			slices.Sort(permissionsNew)

			fmt.Printf("'%s': permission: '%s': %s diff: '%s': notexist -> %s\n", repo.Name, permissionName, kind, name, strings.Join(permissionsNew, ", "))
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
					stats.IgnoredDuplicatePermissionCount++
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
					stats.IgnoredDuplicatePermissionCount++
				} else {
					groups[ug] = append(groups[ug], permission)
				}
			} else {
				groups[ug] = []string{permission}
			}
		}
	}
}
