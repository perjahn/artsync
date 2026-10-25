package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

var stats Statistics

var artifactoryPropertySpecialChars = strings.NewReplacer(
	"%5C", "%5C%5C",
	"%2C", "%5C,",
	"%3B", "%5C;",
	"%7C", "%5C|",
	"%3D", "%5C=",
)

func escapeArtifactoryProperty(value string) string {
	return artifactoryPropertySpecialChars.Replace(url.QueryEscape(value))
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
	allproperties []repoProperties,
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

	reposWithDiffs := findReposWithDiffs(reposToProvision, allrepos, allpermissiondetails, allusers, allproperties, allowpatterns, propertiesConfig)

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

		if diffRepo.hasPropDiff {
			err := setRepoProperties(client, baseurl, token, diffRepo.repo, diffRepo.existingProperties.Properties, propertiesConfig, dryRun)
			if err != nil {
				fmt.Printf("'%s': Warning: Error setting properties: %v\n", diffRepo.repo.Name, err)
				stats.IgnoredInvalidPropertiesCount++
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
	fmt.Printf("  Ignored no diff properties: %d\n", stats.IgnoredNoDiffPropertiesCount)
	fmt.Printf("  Ignored invalid properties: %d\n", stats.IgnoredInvalidPropertiesCount)

	fmt.Printf("  Created users: %d\n", stats.CreatedUserCount)
	fmt.Printf("  Imported groups: %d\n", stats.ImportedGroupCount)

	fmt.Printf("  Created repos: %d\n", stats.CreatedRepoCount)
	fmt.Printf("  Updated repos: %d\n", stats.UpdatedRepoCount)
	fmt.Printf("  Created permission targets: %d\n", stats.CreatedPermissionCount)
	fmt.Printf("  Updated permission targets: %d\n", stats.UpdatedPermissionCount)
	fmt.Printf("  Created properties: %d\n", stats.CreatedPropertyCount)
	fmt.Printf("  Updated properties: %d\n", stats.UpdatedPropertyCount)
	fmt.Printf("  Deleted properties: %d\n", stats.DeletedPropertyCount)
	fmt.Printf("  Remote cache repos not yet created: %d\n", stats.RemoteRepoMissingCount)

	return nil
}

func findReposWithDiffs(
	reposToProvision []Repo,
	allrepos []ArtifactoryRepoDetailsResponse,
	allpermissiondetails []ArtifactoryPermissionDetails,
	allusers []ArtifactoryUser,
	allproperties []repoProperties,
	allowpatterns bool,
	propertiesConfig PropertiesConfig) []repoDiff {

	var reposWithDiffs []repoDiff

	for _, repo := range reposToProvision {
		repo = addUrlToRepoProperties(repo, propertiesConfig)

		hasRepoDiff, existingRepo := hasRepoDiff(repo, allrepos)
		hasPermDiff, permDiffInfo := hasPermissionTargetDiff(repo, allpermissiondetails, allusers, allowpatterns)
		hasPropDiff, existingProperties := hasPropertiesDiff(repo, allproperties, propertiesConfig)

		if !hasRepoDiff {
			stats.IgnoredNoDiffRepoCount++
		}
		if !hasPermDiff {
			stats.IgnoredNoDiffPermissionCount++
		}
		if !hasPropDiff {
			stats.IgnoredNoDiffPropertiesCount++
		}

		if hasRepoDiff || hasPermDiff || hasPropDiff {
			reposWithDiffs = append(reposWithDiffs, repoDiff{
				repo:               repo,
				hasRepoDiff:        hasRepoDiff,
				hasPermDiff:        hasPermDiff,
				hasPropDiff:        hasPropDiff,
				existingRepo:       existingRepo,
				existingPermission: permDiffInfo.Existing,
				permUsers:          permDiffInfo.Users,
				permGroups:         permDiffInfo.Groups,
				existingProperties: existingProperties,
			})
		}
	}

	return reposWithDiffs
}

func addUrlToRepoProperties(repo Repo, propertiesConfig PropertiesConfig) Repo {
	if !propertiesConfig.SetProperties || propertiesConfig.Url == "" {
		return repo
	}

	extraFields := make(map[string][]string, len(repo.ExtraFields)+1)
	maps.Copy(extraFields, repo.ExtraFields)

	extraFields["url"] = []string{propertiesConfig.Url}
	repo.ExtraFields = extraFields
	return repo
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
		if repo.Rclass == "remote" && existingRepo.Offline != repo.Offline {
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

func hasPropertiesDiff(repo Repo, allproperties []repoProperties, propertiesConfig PropertiesConfig) (bool, repoProperties) {
	if repo.Rclass == "virtual" {
		return false, repoProperties{}
	}

	repoName := repo.Name
	if repo.Rclass == "remote" {
		repoName = repo.Name + "-cache"
	}

	var existingProperties repoProperties

	for _, existingProp := range allproperties {
		if existingProp.Repo == repoName {
			existingProperties = existingProp
			break
		}
	}

	if !propertiesConfig.SetProperties {
		return false, existingProperties
	}

	desiredProperties := desiredRepoProperties(repo, propertiesConfig.Prefix)
	return propertyMapsDiffer(desiredProperties, existingProperties.Properties, propertiesConfig.Prefix), existingProperties
}

func desiredRepoProperties(repo Repo, prefix string) map[string][]string {
	desiredProperties := make(map[string][]string)

	for key, value := range repo.ExtraFields {
		if len(value) == 0 {
			continue
		}
		normalizedValues := make([]string, 0, len(value))
		for _, item := range value {
			normalizedValues = append(normalizedValues, strings.Split(item, ";")...)
		}
		desiredProperties[prefix+key] = normalizedValues
	}
	return desiredProperties
}

func propertyMapsDiffer(desiredProperties, existingProperties map[string][]string, prefix string) bool {
	for key, desiredValue := range desiredProperties {
		if existingValue, ok := existingProperties[key]; !ok || !equalStringSlices(existingValue, desiredValue) {
			return true
		}
	}

	if prefix != "" {
		for key := range existingProperties {
			if strings.HasPrefix(key, prefix) {
				if _, ok := desiredProperties[key]; !ok {
					return true
				}
			}
		}
	}
	return false
}

func printRepoPropertiesDiff(existingProperties, desiredProperties map[string][]string, prefix string) (string, error) {
	ownedProperties := make(map[string][]string)
	for key, value := range existingProperties {
		if prefix == "" {
			if _, isDesiredProperty := desiredProperties[key]; isDesiredProperty {
				ownedProperties[key] = value
			}
		} else if strings.HasPrefix(key, prefix) {
			ownedProperties[key] = value
		}
	}

	oldJson, err := json.Marshal(ownedProperties)
	if err != nil {
		return "", fmt.Errorf("error encoding existing repo properties: %w", err)
	}
	newJson, err := json.Marshal(desiredProperties)
	if err != nil {
		return "", fmt.Errorf("error encoding desired repo properties: %w", err)
	}
	return PrintDiff(string(oldJson), string(newJson), false)
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
	if repo.Rclass == "remote" && existingRepo.Offline != repo.Offline {
		fmt.Printf("'%s': Offline diff: '%t' -> '%t'\n", repo.Name, existingRepo.Offline, repo.Offline)
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
		artifactoryrepo.Offline = repo.Offline
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
		artifactoryrepo.Offline = repo.Offline
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
	if repo.Rclass == "remote" {
		fields = append(fields, fmt.Sprintf("Offline: '%t'", repo.Offline))
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
	existingProperties map[string][]string,
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

	desiredProps := desiredRepoProperties(repo, propertiesConfig.Prefix)
	prefix := propertiesConfig.Prefix
	needsUpdate := propertyMapsDiffer(desiredProps, existingProperties, prefix)

	if needsUpdate {
		diffText, err := printRepoPropertiesDiff(existingProperties, desiredProps, prefix)
		if err != nil {
			return fmt.Errorf("error generating repo properties diff: %w", err)
		}
		if diffText != "" {
			log.Printf("Repo properties diff: '%s'\n%s", repoName, diffText)
		}
	}

	if needsUpdate && len(desiredProps) > 0 {
		createdProperties := 0
		updatedProperties := 0
		desiredKeys := make([]string, 0, len(desiredProps))
		for key := range desiredProps {
			desiredKeys = append(desiredKeys, key)
		}
		slices.Sort(desiredKeys)

		var properties []string
		for _, key := range desiredKeys {
			value := desiredProps[key]
			if existingValue, ok := existingProperties[key]; !ok {
				createdProperties++
			} else if !equalStringSlices(existingValue, value) {
				updatedProperties++
			}

			encodedValues := make([]string, len(value))
			for i, item := range value {
				encodedValues[i] = escapeArtifactoryProperty(item)
			}
			properties = append(properties, fmt.Sprintf("%s=%s", escapeArtifactoryProperty(key), strings.Join(encodedValues, ",")))
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

			if repo.Rclass == "remote" && resp.StatusCode == 404 {
				stats.RemoteRepoMissingCount++
				return nil
			} else if resp.StatusCode != 204 && resp.StatusCode != 200 {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("error setting repo properties, unexpected status: %s, body: %s", resp.Status, body)
			}
		}

		stats.CreatedPropertyCount += createdProperties
		stats.UpdatedPropertyCount += updatedProperties
		fmt.Printf("'%s': Set properties: %s.\n", repoName, strings.Join(properties, ", "))
	}

	var unused []string
	if prefix != "" {
		for key := range existingProperties {
			if strings.HasPrefix(key, prefix) {
				if _, ok := desiredProps[key]; !ok {
					unused = append(unused, key)
				}
			}
		}
	}
	slices.Sort(unused)
	if len(unused) > 0 {
		encodedUnused := make([]string, len(unused))
		for i, key := range unused {
			encodedUnused[i] = escapeArtifactoryProperty(key)
		}
		deleteUrl := fmt.Sprintf("%s/artifactory/api/storage/%s?properties=%s&recursive=0", baseurl, repoName, strings.Join(encodedUnused, ","))
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
			if repo.Rclass == "remote" && resp.StatusCode == 404 {
				stats.RemoteRepoMissingCount++
				return nil
			} else if resp.StatusCode != 204 && resp.StatusCode != 200 {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("error deleting unused properties, unexpected status: %s, body: %s", resp.Status, body)
			}
		}
		stats.DeletedPropertyCount += len(unused)
		fmt.Printf("'%s': Deleted unused properties: %s.\n", repoName, strings.Join(unused, ", "))
	}

	return nil
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

	if showDiff {
		artifactjsonsource := GetArtifactJson(existingPermission.JsonSource)
		difftext, _ := PrintDiff(artifactjsonsource, string(json), false)
		log.Printf("Permission (existing) diff: '%s'\n%s", permissionName, difftext)
	}

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
