package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

type ArtifactoryRepoResponse struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Url         string `json:"url"`
	PackageType string `json:"packageType"`
}

type ArtifactoryRepoRequest struct {
	Key         string `json:"key,omitempty"`
	Description string `json:"description,omitempty"`
	Rclass      string `json:"rclass"`
	PackageType string `json:"packageType,omitempty"`
}

type ArtifactoryPermissions struct {
	Permissions []ArtifactoryPermission `json:"permissions"`
	Cursor      string                  `json:"cursor"`
}

type ArtifactoryPermission struct {
	Name string `json:"name"`
	Uri  string `json:"uri"`
}

type ArtifactoryPermissionDetails struct {
	Name      string                                `json:"name"`
	Resources ArtifactoryPermissionDetailsResources `json:"resources"`
}

type ArtifactoryPermissionDetailsResources struct {
	Artifact ArtifactoryPermissionDetailsArtifact `json:"artifact"`
}

type ArtifactoryPermissionDetailsArtifact struct {
	Actions ArtifactoryPermissionDetailsActions            `json:"actions"`
	Targets map[string]ArtifactoryPermissionDetailsTargets `json:"targets"`
}

type ArtifactoryPermissionDetailsActions struct {
	Users  map[string][]string `json:"users"`
	Groups map[string][]string `json:"groups"`
}

type ArtifactoryPermissionDetailsTargets struct {
	IncludePatterns []string `json:"include_patterns"`
	ExcludePatterns []string `json:"exclude_patterns"`
}

type ArtifactoryUsers struct {
	Users  []ArtifactoryUser `json:"users"`
	Cursor string            `json:"cursor"`
}

type ArtifactoryUser struct {
	Username string `json:"username"`
	Uri      string `json:"uri"`
	Realm    string `json:"realm"`
	Status   string `json:"status"`
}

type ArtifactoryGroups struct {
	Groups []ArtifactoryGroup `json:"groups"`
	Cursor string             `json:"cursor"`
}

type ArtifactoryGroup struct {
	GroupName string `json:"group_name"`
	Uri       string `json:"uri"`
}

type Repo struct {
	Name        string   `json:"name"`
	PackageType string   `json:"packageType,omitempty"`
	Description string   `json:"description,omitempty"`
	Rclass      string   `json:"rclass,omitempty"`
	Read        []string `json:"read,omitempty"`
	Write       []string `json:"write,omitempty"`
	Annotate    []string `json:"annotate,omitempty"`
	Delete      []string `json:"delete,omitempty"`
	Manage      []string `json:"manage,omitempty"`
}

func main() {
	dryRun := flag.Bool("d", false, "Enable dry run mode (read-only, no changes will be made).")
	generate := flag.Bool("g", false, "Generate repo file.")
	overwrite := flag.Bool("o", false, "Allow overwriting of existing repo file.")

	flag.Parse()
	args := flag.Args()
	if len(args) != 3 || args[0] == "" || args[1] == "" || args[2] == "" {
		fmt.Println("Usage: artsync [-d] [-g] [-o] <baseurl> <repofile> <tokenfile>")
		fmt.Println()
		flag.Usage()
		fmt.Println("baseurl:    Base URL of Artifactory instance, like https://artifactory.example.com")
		fmt.Println("repofile:   Input file with repo definitions (output file when using -g flag).")
		fmt.Println("tokenfile:  File with access token (aka bearer token).")
		os.Exit(1)
	}

	baseurl := args[0]
	repofile := args[1]
	tokenfile := args[2]

	if !*generate {
		if _, err := os.Stat(repofile); os.IsNotExist(err) {
			fmt.Printf("File not found: '%s'\n", repofile)
			os.Exit(1)
		}
	}

	if *generate && !*overwrite {
		if _, err := os.Stat(repofile); err == nil {
			fmt.Printf("File already exists, will not overwrite: '%s'\n", repofile)
			os.Exit(1)
		}
	}

	data, err := os.ReadFile(tokenfile)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}
	token := string(data)

	client := &http.Client{}

	var reposToProvision []Repo

	if !*generate {
		reposToProvision, err = validateRepoFile(repofile)
		if err != nil {
			fmt.Printf("Error validating repo file: %v\n", err)
			os.Exit(1)
		}
	}

	repos, users, groups, permissiondetails, err := getStuff(client, baseurl, token)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if *generate {
		generete(repos, permissiondetails, repofile)
		if err != nil {
			fmt.Printf("Error generating: %v\n", err)
			os.Exit(1)
		}
	} else {
		err = provision(reposToProvision, repos, users, groups, permissiondetails, client, baseurl, token, *dryRun)
		if err != nil {
			fmt.Printf("Error provisioning: %v\n", err)
			os.Exit(1)
		}
	}
}

func validateRepoFile(repofile string) ([]Repo, error) {
	file, err := os.Open(repofile)
	if err != nil {
		return nil, fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()

	var reposToProvision []Repo
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&reposToProvision)
	if err != nil {
		return nil, fmt.Errorf("error parsing json file: %w", err)
	}

	reposToProvision = removeDups(reposToProvision)

	return reposToProvision, nil
}

func removeDups(reposToProvision []Repo) []Repo {
	reposToDelete := make(map[string][]int)

	for i := range reposToProvision {
		name := reposToProvision[i].Name
		for j := i + 1; j < len(reposToProvision); j++ {
			if name == reposToProvision[j].Name {
				indices, ok := reposToDelete[name]
				if !ok {
					reposToDelete[name] = []int{i}
				}
				if !slices.Contains(indices, j) {
					reposToDelete[name] = append(reposToDelete[name], j)
				}
			}
		}
	}
	for key, value := range reposToDelete {
		stringslice := make([]string, len(value))
		for i, num := range value {
			stringslice[i] = strconv.Itoa(num + 1)
		}
		fmt.Printf("Warning: Ignoring repos due to duplicate name. Name: '%s', Indices: %s\n", key, strings.Join(stringslice, ", "))
	}

	repoIndicesToDelete := []int{}
	for _, value := range reposToDelete {
		repoIndicesToDelete = append(repoIndicesToDelete, value...)
	}
	sort.Ints(repoIndicesToDelete)
	for i := len(repoIndicesToDelete) - 1; i >= 0; i-- {
		reposToProvision = slices.Delete(reposToProvision, i, i+1)
	}

	return reposToProvision
}

func generete(repos []ArtifactoryRepoResponse, permissiondetails []ArtifactoryPermissionDetails, repofile string) error {
	reposToSave := make([]Repo, len(repos))

	for i, repo := range repos {
		reposToSave[i] = Repo{
			Name:        repo.Key,
			Description: repo.Description,
			Rclass:      repo.Type,
			PackageType: repo.PackageType,
		}
		if repo.Type == "LOCAL" {
			reposToSave[i].Rclass = ""
		}
		if repo.PackageType == "Generic" {
			reposToSave[i].PackageType = ""
		}

		for _, permission := range permissiondetails {
			if permission.Name == repo.Key {
				for key, permissions := range permission.Resources.Artifact.Actions.Users {
					if slices.Contains(permissions, "READ") {
						reposToSave[i].Read = append(reposToSave[i].Read, key)
					}
					if slices.Contains(permissions, "WRITE") {
						reposToSave[i].Write = append(reposToSave[i].Write, key)
					}
					if slices.Contains(permissions, "ANNOTATE") {
						reposToSave[i].Annotate = append(reposToSave[i].Annotate, key)
					}
					if slices.Contains(permissions, "DELETE") {
						reposToSave[i].Delete = append(reposToSave[i].Delete, key)
					}
					if slices.Contains(permissions, "MANAGE") {
						reposToSave[i].Manage = append(reposToSave[i].Manage, key)
					}
				}
				for key, permissions := range permission.Resources.Artifact.Actions.Groups {
					if slices.Contains(permissions, "READ") {
						reposToSave[i].Read = append(reposToSave[i].Read, key)
					}
					if slices.Contains(permissions, "WRITE") {
						reposToSave[i].Write = append(reposToSave[i].Write, key)
					}
					if slices.Contains(permissions, "ANNOTATE") {
						reposToSave[i].Annotate = append(reposToSave[i].Annotate, key)
					}
					if slices.Contains(permissions, "DELETE") {
						reposToSave[i].Delete = append(reposToSave[i].Delete, key)
					}
					if slices.Contains(permissions, "MANAGE") {
						reposToSave[i].Manage = append(reposToSave[i].Manage, key)
					}
				}
			}
		}

		slices.Sort(reposToSave[i].Read)
		slices.Sort(reposToSave[i].Write)
		slices.Sort(reposToSave[i].Annotate)
		slices.Sort(reposToSave[i].Delete)
		slices.Sort(reposToSave[i].Manage)
	}

	sort.Slice(reposToSave, func(i, j int) bool {
		return reposToSave[i].Name < reposToSave[j].Name
	})

	json, err := json.MarshalIndent(reposToSave, "", "  ")
	if err != nil {
		return fmt.Errorf("error generating json: %w", err)
	}

	file, err := os.Create(repofile)
	if err != nil {
		return fmt.Errorf("error creating file: %w", err)
	}
	defer file.Close()

	_, err = file.Write(json)
	if err != nil {
		return fmt.Errorf("error saving file: %w", err)
	}

	return nil
}

func getStuff(client *http.Client, baseurl string, token string) ([]ArtifactoryRepoResponse, []ArtifactoryUser, []ArtifactoryGroup, []ArtifactoryPermissionDetails, error) {
	repos, err := getRepos(client, baseurl, token)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	fmt.Printf("Repo count: %d\n", len(repos))

	permissions, err := getPermissions(client, baseurl, token)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	fmt.Printf("Permissions count: %d\n", len(permissions))

	users, err := getUsers(client, baseurl, token)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	fmt.Printf("User count: %d\n", len(users))

	groups, err := getGroups(client, baseurl, token)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	fmt.Printf("Group count: %d\n", len(groups))

	permissiondetails, err := getPermissionDetails(client, baseurl, token, permissions)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	fmt.Printf("Permission details count: %d\n", len(permissiondetails))

	return repos, users, groups, permissiondetails, nil
}

func provision(
	reposToProvision []Repo,
	allrepos []ArtifactoryRepoResponse,
	allusers []ArtifactoryUser,
	allgroups []ArtifactoryGroup,
	allpermissiondetails []ArtifactoryPermissionDetails,
	client *http.Client,
	baseurl string,
	token string,
	dryrun bool) error {

	fmt.Printf("Repos to provision: %d\n", len(reposToProvision))
	for index, repo := range reposToProvision {
		err := provisionRepo(repo, allrepos, allusers, allgroups, allpermissiondetails, client, baseurl, token, dryrun)
		if err != nil {
			fmt.Printf("Warning: Ignoring repo %d '%s': %v\n", index+1, repo.Name, err)
		}
	}

	return nil
}

func provisionRepo(
	repo Repo,
	allrepos []ArtifactoryRepoResponse,
	allusers []ArtifactoryUser,
	allgroups []ArtifactoryGroup,
	allpermissiondetails []ArtifactoryPermissionDetails,
	client *http.Client,
	baseurl string,
	token string,
	dryrun bool) error {

	if repo.Name == "" {
		return fmt.Errorf("missing name for repo")
	}

	if !isValidRepoName(repo.Name) {
		return fmt.Errorf("invalid name for repo")
	}

	if repo.Rclass == "" {
		repo.Rclass = "local"
	}

	errors := checkUsersAndGroups(repo.Read, allusers, allgroups)
	if len(errors) > 0 {
		for _, err := range errors {
			fmt.Printf("Permission read, Repo '%s': %v\n", repo.Name, err)
		}
		return fmt.Errorf("")
	}
	errors = checkUsersAndGroups(repo.Write, allusers, allgroups)
	if len(errors) > 0 {
		for _, err := range errors {
			fmt.Printf("Permission write, Repo '%s': %v\n", repo.Name, err)
		}
		return fmt.Errorf("")
	}
	errors = checkUsersAndGroups(repo.Annotate, allusers, allgroups)
	if len(errors) > 0 {
		for _, err := range errors {
			fmt.Printf("Permission annotate, Repo '%s': %v\n", repo.Name, err)
		}
		return fmt.Errorf("")
	}
	errors = checkUsersAndGroups(repo.Delete, allusers, allgroups)
	if len(errors) > 0 {
		for _, err := range errors {
			fmt.Printf("Permission delete, Repo '%s': %v\n", repo.Name, err)
		}
		return fmt.Errorf("")
	}
	errors = checkUsersAndGroups(repo.Manage, allusers, allgroups)
	if len(errors) > 0 {
		for _, err := range errors {
			fmt.Printf("Permission manage, Repo '%s': %v\n", repo.Name, err)
		}
		return fmt.Errorf("")
	}

	repoExists := false
	for _, r := range allrepos {
		if r.Key == repo.Name {
			repoExists = true
			break
		}
	}

	if repoExists {
		fmt.Printf("'%s': Repo already exists, updating...\n", repo.Name)

		url := fmt.Sprintf("%s/artifactory/api/repositories/%s", baseurl, repo.Name)

		// Todo: Fields (not key/name) might be overwritten with new values, if so, print out a diff
		artifactoryrepo := ArtifactoryRepoRequest{
			Key:         repo.Name,
			Description: repo.Description,
			Rclass:      repo.Rclass,
			PackageType: repo.PackageType,
		}

		json, err := json.Marshal(artifactoryrepo)
		if err != nil {
			return fmt.Errorf("error updating repo, error generating json: %w", err)
		}
		req, err := http.NewRequest("PUT", url, strings.NewReader(string(json)))
		if err != nil {
			return fmt.Errorf("error updating repo, error creating request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		/*
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
				fmt.Printf("Updated repo successfully: '%s'\n", repo.Name)
			}
		*/
	} else {
		//fmt.Printf("Repo '%s' does not exist, creating...\n", repo.Name)

		url := fmt.Sprintf("%s/artifactory/api/repositories/%s", baseurl, repo.Name)
		artifactoryrepo := ArtifactoryRepoRequest{
			Key:         repo.Name,
			Description: repo.Description,
			Rclass:      repo.Rclass,
			PackageType: repo.PackageType,
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

		if !dryrun {
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
				//fmt.Printf("Created repo successfully: '%s'\n", repo.Name)
			}
		}
	}

	permissionTargetExists := false
	for _, p := range allpermissiondetails {
		if p.Name == repo.Name {
			permissionTargetExists = true
			break
		}
	}

	if permissionTargetExists {
		//fmt.Printf("Permission target '%s' already exists, updating...\n", repo.Name)
	} else {
		//fmt.Printf("Permission target '%s' doesn't exist, creating...\n", repo.Name)

		url := fmt.Sprintf("%s/access/api/v2/permissions", baseurl)

		users, groups := convertUsersAndGroups(repo, allusers, allgroups)

		targets := make(map[string]ArtifactoryPermissionDetailsTargets)
		targets[repo.Name] = ArtifactoryPermissionDetailsTargets{
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

		//fmt.Printf("json: '%s'\n", string(json))

		if err != nil {
			return fmt.Errorf("error creating permission target, error generating json: %w", err)
		}
		req, err := http.NewRequest("POST", url, strings.NewReader(string(json)))
		if err != nil {
			return fmt.Errorf("error creating permission target, error creating request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		if !dryrun {
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
				//fmt.Printf("Created permission target successfully: '%s'\n", repo.Name)
			}
		}
	}

	return nil
}

func convertUsersAndGroups(repo Repo, allusers []ArtifactoryUser, allgroups []ArtifactoryGroup) (map[string][]string, map[string][]string) {
	alluserstrings := make([]string, len(allusers))
	for i, user := range allusers {
		alluserstrings[i] = user.Username
	}
	allgroupstrings := make([]string, len(allgroups))
	for i, group := range allgroups {
		allgroupstrings[i] = group.GroupName
	}

	users := make(map[string][]string)
	groups := make(map[string][]string)

	getUsersAndGroupsPermission(repo.Read, "READ", users, groups, alluserstrings)
	getUsersAndGroupsPermission(repo.Write, "WRITE", users, groups, alluserstrings)
	getUsersAndGroupsPermission(repo.Annotate, "ANNOTATE", users, groups, alluserstrings)
	getUsersAndGroupsPermission(repo.Delete, "DELETE", users, groups, alluserstrings)
	getUsersAndGroupsPermission(repo.Manage, "MANAGE", users, groups, alluserstrings)

	return users, groups
}

func getUsersAndGroupsPermission(ugs []string, permission string, users map[string][]string, groups map[string][]string, alluserstrings []string) {
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

func getUsers(client *http.Client, baseurl string, token string) ([]ArtifactoryUser, error) {
	var cursor string
	var allusers []ArtifactoryUser

	for {
		var users []ArtifactoryUser
		var err error

		users, cursor, err = getUsersPage(client, baseurl, token, cursor)
		if err != nil {
			return nil, err
		}

		allusers = append(allusers, users...)

		if cursor == "" {
			json, err := json.MarshalIndent(allusers, "", "  ")
			if err != nil {
				return nil, fmt.Errorf("error generating json: %w", err)
			}

			err = os.WriteFile("allusers.json", []byte(json), 0600)
			if err != nil {
				return nil, fmt.Errorf("error saving users: %w", err)
			}

			return allusers, nil
		}
	}
}

func getUsersPage(client *http.Client, baseurl string, token string, cursor string) ([]ArtifactoryUser, string, error) {
	var url string
	if cursor == "" {
		url = fmt.Sprintf("%s/access/api/v2/users", baseurl)
	} else {
		url = fmt.Sprintf("%s/access/api/v2/users?cursor=%s", baseurl, cursor)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != 200 {
		fmt.Printf("Url: '%s'\n", url)
		fmt.Printf("Unexpected status: '%s'\n", resp.Status)
		fmt.Printf("Response body: '%s'\n", body)
	}

	var users ArtifactoryUsers
	err = json.Unmarshal(body, &users)
	if err != nil {
		return nil, "", fmt.Errorf("error parsing response body: %w", err)
	}

	return users.Users, users.Cursor, nil
}

func getGroups(client *http.Client, baseurl string, token string) ([]ArtifactoryGroup, error) {
	var cursor string
	var allgroups []ArtifactoryGroup

	for {
		var groups []ArtifactoryGroup
		var err error

		groups, cursor, err = getGroupsPage(client, baseurl, token, cursor)
		if err != nil {
			return nil, err
		}

		allgroups = append(allgroups, groups...)

		if cursor == "" {
			json, err := json.MarshalIndent(allgroups, "", "  ")
			if err != nil {
				return nil, fmt.Errorf("error generating json: %w", err)
			}

			err = os.WriteFile("allgroups.json", []byte(json), 0600)
			if err != nil {
				return nil, fmt.Errorf("error saving groups: %w", err)
			}

			return allgroups, nil
		}
	}
}

func getGroupsPage(client *http.Client, baseurl string, token string, cursor string) ([]ArtifactoryGroup, string, error) {
	var url string
	if cursor == "" {
		url = fmt.Sprintf("%s/access/api/v2/groups", baseurl)
	} else {
		url = fmt.Sprintf("%s/access/api/v2/groups?cursor=%s", baseurl, cursor)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != 200 {
		fmt.Printf("Url: '%s'\n", url)
		fmt.Printf("Unexpected status: '%s'\n", resp.Status)
		fmt.Printf("Response body: '%s'\n", body)
	}

	var groups ArtifactoryGroups
	err = json.Unmarshal(body, &groups)
	if err != nil {
		return nil, "", fmt.Errorf("error parsing response body: %w", err)
	}

	return groups.Groups, groups.Cursor, nil
}

func getRepos(client *http.Client, baseurl string, token string) ([]ArtifactoryRepoResponse, error) {
	url := fmt.Sprintf("%s/artifactory/api/repositories", baseurl)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != 200 {
		fmt.Printf("Url: '%s'\n", url)
		fmt.Printf("Unexpected status: '%s'\n", resp.Status)
		fmt.Printf("Response body: '%s'\n", body)
	}

	err = os.WriteFile("allrepos.json", []byte(body), 0600)
	if err != nil {
		return nil, fmt.Errorf("error saving response body: %w", err)
	}

	var repos []ArtifactoryRepoResponse
	err = json.Unmarshal(body, &repos)
	if err != nil {
		return nil, fmt.Errorf("error parsing response body: %w", err)
	}

	return repos, nil
}

func getPermissions(client *http.Client, baseurl string, token string) ([]ArtifactoryPermission, error) {
	var cursor string
	var allpermissions []ArtifactoryPermission

	for {
		var permissions []ArtifactoryPermission
		var err error

		permissions, cursor, err = getPermissionsPage(client, baseurl, token, cursor)
		if err != nil {
			return nil, err
		}

		allpermissions = append(allpermissions, permissions...)

		if cursor == "" {
			json, err := json.MarshalIndent(allpermissions, "", "  ")
			if err != nil {
				return nil, fmt.Errorf("error generating json: %w", err)
			}

			err = os.WriteFile("allpermissions.json", []byte(json), 0600)
			if err != nil {
				return nil, fmt.Errorf("error saving permissions: %w", err)
			}

			return allpermissions, nil
		}
	}
}

func getPermissionsPage(client *http.Client, baseurl string, token string, cursor string) ([]ArtifactoryPermission, string, error) {
	var url string
	if cursor == "" {
		url = fmt.Sprintf("%s/access/api/v2/permissions", baseurl)
	} else {
		url = fmt.Sprintf("%s/access/api/v2/permissions?cursor=%s", baseurl, cursor)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != 200 {
		fmt.Printf("Url: '%s'\n", url)
		fmt.Printf("Unexpected status: '%s'\n", resp.Status)
		fmt.Printf("Response body: '%s'\n", body)
	}

	var permissions ArtifactoryPermissions
	err = json.Unmarshal(body, &permissions)
	if err != nil {
		return nil, "", fmt.Errorf("error parsing response body: %w", err)
	}

	return permissions.Permissions, permissions.Cursor, nil
}

func getPermissionDetails(client *http.Client, baseurl string, token string, permissions []ArtifactoryPermission) ([]ArtifactoryPermissionDetails, error) {
	var allpermissiondetails []ArtifactoryPermissionDetails

	for _, permission := range permissions {
		fmt.Print(".")

		url := fmt.Sprintf("%s/access/api/v2/permissions/%s", baseurl, url.PathEscape(permission.Name))
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error sending request: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error reading response body: %w", err)
		}

		if resp.StatusCode != 200 {
			fmt.Printf("Url: '%s'\n", url)
			fmt.Printf("Url2: '%s'\n", permission.Uri)
			fmt.Printf("Unexpected status: '%s'\n", resp.Status)
			fmt.Printf("Response body: '%s'\n", body)
		}

		err = os.WriteFile("allpermissiondetails1.json", []byte(body), 0600)
		if err != nil {
			return nil, fmt.Errorf("error saving response body: %w", err)
		}

		var permissiondetails ArtifactoryPermissionDetails

		err = json.Unmarshal(body, &permissiondetails)
		if err != nil {
			return nil, fmt.Errorf("error parsing response body: %w", err)
		}

		allpermissiondetails = append(allpermissiondetails, permissiondetails)
	}

	fmt.Println()

	json, err := json.MarshalIndent(allpermissiondetails, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("error generating json: %w", err)
	}

	err = os.WriteFile("allpermissiondetails.json", []byte(string(json)), 0600)
	if err != nil {
		return nil, fmt.Errorf("error saving permission details: %w", err)
	}

	return allpermissiondetails, nil
}
