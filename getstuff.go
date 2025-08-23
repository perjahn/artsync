package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

func GetStuff(
	client *http.Client,
	baseurl string,
	token string,
	retrieveldapsettings bool) (
	[]ArtifactoryRepoDetailsResponse,
	[]ArtifactoryUser,
	[]ArtifactoryGroup,
	[]ArtifactoryPermissionDetails,
	[]ArtifactoryLDAPSettings,
	[]ArtifactoryLDAPGroupSettings,
	error) {
	repos, err := getRepos(client, baseurl, token)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	fmt.Printf("Repo count: %d\n", len(repos))

	repodetails, err := getRepoDetails(client, baseurl, token, repos)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	fmt.Printf("Repo details count: %d\n", len(repodetails))

	permissions, err := getPermissions(client, baseurl, token)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	fmt.Printf("Permissions count: %d\n", len(permissions))

	users, err := getUsers(client, baseurl, token)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	fmt.Printf("User count: %d\n", len(users))

	groups, err := getGroups(client, baseurl, token)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	fmt.Printf("Group count: %d\n", len(groups))

	permissiondetails, err := getPermissionDetails(client, baseurl, token, permissions)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	fmt.Printf("Permission details count: %d\n", len(permissiondetails))

	var ldapsettings []ArtifactoryLDAPSettings
	var ldapgroupsettings []ArtifactoryLDAPGroupSettings
	if retrieveldapsettings {
		ldapsettings, err = getLDAPSettings(client, baseurl, token)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}

		fmt.Printf("LDAP settings count: %d\n", len(ldapsettings))

		ldapgroupsettings, err = getLDAPGroupSettings(client, baseurl, token)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}

		fmt.Printf("LDAP group settings count: %d\n", len(ldapgroupsettings))
	}

	return repodetails, users, groups, permissiondetails, ldapsettings, ldapgroupsettings, nil
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

func getRepoDetails(client *http.Client, baseurl string, token string, repos []ArtifactoryRepoResponse) ([]ArtifactoryRepoDetailsResponse, error) {
	var allrepodetails []ArtifactoryRepoDetailsResponse

	fmt.Println("Getting Repo details...")

	for _, repo := range repos {
		fmt.Print(".")

		url := fmt.Sprintf("%s/artifactory/api/repositories/%s", baseurl, url.PathEscape(repo.Key))
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

		err = os.WriteFile("allrepodetails1.json", []byte(body), 0600)
		if err != nil {
			return nil, fmt.Errorf("error saving response body: %w", err)
		}

		var repodetails ArtifactoryRepoDetailsResponse

		err = json.Unmarshal(body, &repodetails)
		if err != nil {
			return nil, fmt.Errorf("error parsing response body: %w", err)
		}

		allrepodetails = append(allrepodetails, repodetails)
	}

	fmt.Println()

	json, err := json.MarshalIndent(allrepodetails, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("error generating json: %w", err)
	}

	err = os.WriteFile("allrepodetails.json", []byte(string(json)), 0600)
	if err != nil {
		return nil, fmt.Errorf("error saving repo details: %w", err)
	}

	return allrepodetails, nil
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

	fmt.Println("Getting Permission details...")

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

func getLDAPSettings(client *http.Client, baseurl string, token string) ([]ArtifactoryLDAPSettings, error) {
	var settings []ArtifactoryLDAPSettings

	cachefilename := "ldap_settings.json"
	if _, err := os.Stat(cachefilename); err == nil {
		fmt.Printf("Using cached ldap settings from file: '%s'\n", cachefilename)

		data, err := os.ReadFile(cachefilename)
		if err != nil {
			return nil, fmt.Errorf("error saving groups: %w", err)
		}

		err = json.Unmarshal(data, &settings)
		if err != nil {
			return nil, fmt.Errorf("error parsing response body: %w", err)
		}

		return settings, nil
	}

	url := fmt.Sprintf("%s/access/api/v1/ldap/settings", baseurl)

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

	err = json.Unmarshal(body, &settings)
	if err != nil {
		return nil, fmt.Errorf("error parsing response body: %w", err)
	}

	json, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("error generating json: %w", err)
	}

	err = os.WriteFile("allldapsettings.json", []byte(json), 0600)
	if err != nil {
		return nil, fmt.Errorf("error saving groups: %w", err)
	}

	return settings, nil
}

func getLDAPGroupSettings(client *http.Client, baseurl string, token string) ([]ArtifactoryLDAPGroupSettings, error) {
	var groups []ArtifactoryLDAPGroupSettings

	cachefilename := "ldap_groups.json"
	if _, err := os.Stat(cachefilename); err == nil {
		fmt.Printf("Using cached ldap groups from file: '%s'\n", cachefilename)

		data, err := os.ReadFile(cachefilename)
		if err != nil {
			return nil, fmt.Errorf("error saving groups: %w", err)
		}

		err = json.Unmarshal(data, &groups)
		if err != nil {
			return nil, fmt.Errorf("error parsing response body: %w", err)
		}

		return groups, nil
	}

	url := fmt.Sprintf("%s/access/api/v1/ldap/groups", baseurl)

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

	err = json.Unmarshal(body, &groups)
	if err != nil {
		return nil, fmt.Errorf("error parsing response body: %w", err)
	}

	json, err := json.MarshalIndent(groups, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("error generating json: %w", err)
	}

	err = os.WriteFile("allldapgroupsettings.json", []byte(json), 0600)
	if err != nil {
		return nil, fmt.Errorf("error saving groups: %w", err)
	}

	return groups, nil
}
