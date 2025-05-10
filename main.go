package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type ArtifactoryRepo struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Url         string `json:"url"`
	PackageType string `json:"packageType"`
}

type ArtifactoryPermissions struct {
	Permissions []ArtifactoryPermission `json:"permissions"`
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
	Users []ArtifactoryUser `json:"users"`
}

type ArtifactoryUser struct {
	Username string `json:"username"`
	Uri      string `json:"uri"`
	Realm    string `json:"realm"`
	Status   string `json:"status"`
}

type ArtifactoryGroups struct {
	Groups []ArtifactoryGroup `json:"groups"`
}

type ArtifactoryGroup struct {
	GroupName string `json:"group_name"`
	Uri       string `json:"uri"`
}

type Repo struct {
	Key                 string `json:"key"`
	Description         string `json:"description"`
	Rclass              string `json:"rclass"`
	PackageType         string `json:"packageType"`
	PermissionRead      string `json:"permissionRead"`
	PermissionReadwrite string `json:"permissionReadwrite"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: artsync <baseurl>")
		os.Exit(1)
	}

	baseurl := os.Args[1]

	data, err := os.ReadFile("token.txt")
	if err != nil {
		fmt.Println("Error reading file:", err)
		os.Exit(1)
	}
	token := string(data)

	repos, err := getRepos(baseurl, token)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	fmt.Println("Repo count:", len(repos))

	//for _, repo := range repos {
	//	fmt.Printf("Key: %s, Description: %s, Type: %s, Url: %s, PackageType: %s\n",
	//		repo.Key, repo.Description, repo.Type, repo.Url, repo.PackageType)
	//}

	permissions, err := getPermissions(baseurl, token)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	fmt.Println("Permissions count:", len(permissions))

	//for _, permission := range permissions {
	//	fmt.Printf("Name: %s, Uri: %s\n",
	//		permission.Name, permission.Uri)
	//}

	users, err := getUsers(baseurl, token)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	fmt.Println("User count:", len(users))

	//for _, user := range users {
	//	fmt.Printf("Username: %s, Uri: %s, Realm: %s, Status: %s\n",
	//		user.Username, user.Uri, user.Realm, user.Status)
	//}

	groups, err := getGroups(baseurl, token)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	fmt.Println("Group count:", len(groups))

	//for _, group := range groups {
	//	fmt.Printf("Name: %s, Uri: %s\n",
	//		group.GroupName, group.Uri)
	//}

	//permissions := []ArtifactoryPermission{
	//	{Name: "permission1", Uri: "/access/api/v2/permissions/permission1"},
	//	{Name: "permission2", Uri: "/access/api/v2/permissions/permission2"},
	//	{Name: "permission3", Uri: "/access/api/v2/permissions/permission3"},
	//}

	permissiondetails, err := getPermissionDetails(baseurl, token, permissions)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	fmt.Println("Permission details count:", len(permissiondetails))

	jsonData, err := json.Marshal(permissiondetails)
	if err != nil {
		fmt.Println("Error generating json:", err)
		os.Exit(1)
	}

	err = os.WriteFile("allpermissiondetails.json", []byte(string(jsonData)), 0600)
	if err != nil {
		fmt.Println("Error saving permission details:", err)
		os.Exit(1)
	}
}

func getUsers(baseurl, token string) ([]ArtifactoryUser, error) {
	url := baseurl + "/access/api/v2/users"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	fmt.Printf("Response status: %s\n", resp.Status)
	//fmt.Printf("Response body: %s\n", body)

	err = os.WriteFile("allusers.json", []byte(body), 0600)
	if err != nil {
		return nil, fmt.Errorf("error saving response body: %w", err)
	}

	var users ArtifactoryUsers

	err = json.Unmarshal(body, &users)
	if err != nil {
		return nil, fmt.Errorf("error parsing response body: %w", err)
	}

	return users.Users, nil
}

func getGroups(baseurl, token string) ([]ArtifactoryGroup, error) {
	url := baseurl + "/access/api/v2/groups"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	fmt.Printf("Response status: %s\n", resp.Status)
	//fmt.Printf("Response body: %s\n", body)

	err = os.WriteFile("allgroups.json", []byte(body), 0600)
	if err != nil {
		return nil, fmt.Errorf("error saving response body: %w", err)
	}

	var groups ArtifactoryGroups

	err = json.Unmarshal(body, &groups)
	if err != nil {
		return nil, fmt.Errorf("error parsing response body: %w", err)
	}

	return groups.Groups, nil
}

func getRepos(baseurl string, token string) ([]ArtifactoryRepo, error) {
	url := baseurl + "/artifactory/api/repositories"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	fmt.Printf("Response status: %s\n", resp.Status)
	//fmt.Printf("Response body: %s\n", body)

	err = os.WriteFile("allrepos.json", []byte(body), 0600)
	if err != nil {
		return nil, fmt.Errorf("error saving response body: %w", err)
	}

	var repos []ArtifactoryRepo

	err = json.Unmarshal(body, &repos)
	if err != nil {
		return nil, fmt.Errorf("error parsing response body: %w", err)
	}

	return repos, nil
}

func getPermissions(baseurl string, token string) ([]ArtifactoryPermission, error) {
	url := baseurl + "/access/api/v2/permissions"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	fmt.Printf("Response status: %s\n", resp.Status)

	err = os.WriteFile("allpermissions.json", []byte(body), 0600)
	if err != nil {
		return nil, fmt.Errorf("error saving response body: %w", err)
	}

	var permissions ArtifactoryPermissions

	err = json.Unmarshal(body, &permissions)
	if err != nil {
		return nil, fmt.Errorf("error parsing response body: %w", err)
	}

	return permissions.Permissions, nil
}

func getPermissionDetails(baseurl string, token string, permissions []ArtifactoryPermission) ([]ArtifactoryPermissionDetails, error) {
	var allpermissiondetails []ArtifactoryPermissionDetails

	client := &http.Client{}

	for _, permission := range permissions {
		fmt.Print(".")

		url := baseurl + "/access/api/v2/permissions/" + strings.ReplaceAll(url.PathEscape(permission.Name), "%2F", "%252F")
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
			fmt.Printf("Url1: '%s'\n", url)
			fmt.Printf("Url2: '%s'\n", permission.Uri)
			fmt.Printf("Unexpected status: '%s'\n", resp.Status)
			fmt.Printf("Response body: '%s'\n", body)
		}

		err = os.WriteFile("allpermissiondetails1.json", []byte(body), 0600)
		if err != nil {
			return nil, fmt.Errorf("error saving response body: %w", err)
		}

		//body, err := os.ReadFile("perm.json")
		//if err != nil {
		//	return nil, fmt.Errorf("error reading perm.json: %w", err)
		//}

		var permissiondetails ArtifactoryPermissionDetails

		err = json.Unmarshal(body, &permissiondetails)
		if err != nil {
			return nil, fmt.Errorf("error parsing response body: %w", err)
		}

		allpermissiondetails = append(allpermissiondetails, permissiondetails)
	}

	return allpermissiondetails, nil
}
