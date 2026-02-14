package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// For mock testing.
var queryldapImportGroupFn = queryldap

func ImportGroup(
	client *http.Client,
	baseurl string,
	ldapUsername string,
	ldapPassword string,
	groupname string,
	ldapSettings []ArtifactoryLDAPSettings,
	ldapGroupSettings []ArtifactoryLDAPGroupSettings,
	artifactoryUsername string,
	artifactoryPassword string,
	dryRun bool) (bool, error) {

	if len(ldapSettings) == 0 {
		return false, fmt.Errorf("missing LDAP settings")
	}
	if len(ldapGroupSettings) == 0 {
		return false, fmt.Errorf("missing LDAP group settings")
	}

	for _, ldapgroupsettingsSingle := range ldapGroupSettings {
		settingsIndex := -1
		for i := range ldapSettings {
			if ldapSettings[i].Key == ldapgroupsettingsSingle.EnabledLdap {
				settingsIndex = i
				break
			}
		}
		if settingsIndex == -1 {
			return false, fmt.Errorf("LDAP settings named '%s' not found", ldapgroupsettingsSingle.EnabledLdap)
		}
		ldapSettingsSingle := ldapSettings[settingsIndex]

		var basedn string
		if strings.Count(ldapSettingsSingle.LdapUrl, "/") >= 3 {
			parts := strings.SplitN(ldapSettingsSingle.LdapUrl, "/", 4)
			if ldapgroupsettingsSingle.GroupBaseDn != "" {
				basedn = fmt.Sprintf("%s,%s", ldapgroupsettingsSingle.GroupBaseDn, parts[3])
			} else {
				basedn = parts[3]
			}
		} else {
			basedn = ldapgroupsettingsSingle.GroupBaseDn
		}

		var filter string
		if ldapgroupsettingsSingle.Filter != "" {
			filter = fmt.Sprintf("(&%s(%s=%s))", ldapgroupsettingsSingle.Filter, ldapgroupsettingsSingle.GroupNameAttribute, groupname)
		} else {
			filter = fmt.Sprintf("(%s=%s)", ldapgroupsettingsSingle.GroupNameAttribute, groupname)
		}

		entries, err := queryldapImportGroupFn(
			ldapSettingsSingle.LdapUrl,
			basedn,
			filter,
			ldapUsername,
			ldapPassword,
			[]string{ldapgroupsettingsSingle.DescriptionAttribute})
		if err != nil {
			return false, fmt.Errorf("query failed: %w", err)
		}

		fmt.Printf("Entries: %d\n", len(entries))

		if len(entries) < 1 {
			fmt.Printf("Didn't find group: '%s'\n", groupname)
			return false, nil
		}
		if len(entries) > 1 {
			return false, fmt.Errorf("error: multiple DNs found for group: '%s'", groupname)
		}
		entry := entries[0]

		groupdn := entry.DN
		fmt.Printf("groupdn: '%s'\n", groupdn)

		values := entry.GetAttributeValues(ldapgroupsettingsSingle.DescriptionAttribute)
		description := ""
		if len(values) >= 1 {
			description = values[0]
		}
		fmt.Printf("description: '%s'\n", description)

		importGroup := ArtifactoryGroupImport{
			ImportGroups: []ArtifactoryImportGroups{
				{
					GroupName:      groupname,
					Description:    description,
					GroupDn:        groupdn,
					RequiredUpdate: "DOES_NOT_EXIST",
				},
			},
			LdapGroupSettings: ldapgroupsettingsSingle,
		}

		err = importSingleGroup(client, baseurl, artifactoryUsername, artifactoryPassword, groupname, importGroup, dryRun)
		if err != nil {
			return false, fmt.Errorf("import failed: %w", err)
		}
	}

	return true, nil
}

func importSingleGroup(
	client *http.Client,
	baseurl string,
	username string,
	password string,
	groupname string,
	groupimport ArtifactoryGroupImport,
	dryRun bool) error {

	fmt.Printf("Importing group: '%s'\n", groupname)

	accessToken, refreshToken, err := getUITokens(client, baseurl, username, password)
	if err != nil {
		return fmt.Errorf("error: unable to obtain UI tokens: %v", err)
	}

	payload, err := json.Marshal(groupimport)
	if err != nil {
		return fmt.Errorf("error marshalling group: %w", err)
	}

	url := fmt.Sprintf("%s/ui/api/v1/access/api/ui/ldap/groups/import", baseurl)

	req, err := http.NewRequest("POST", url, strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("error creating request for '%s': %w", url, err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	req.AddCookie(&http.Cookie{Name: "ACCESSTOKEN", Value: accessToken})
	req.AddCookie(&http.Cookie{Name: "REFRESHTOKEN", Value: refreshToken})

	if !dryRun {
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("error sending request to '%s': %w", url, err)
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			return fmt.Errorf("unexpected response from '%s': %s - %s", url, resp.Status, string(body))
		}

		fmt.Printf("Created group '%s' via '%s' (status '%s', code %d)\n", groupname, url, resp.Status, resp.StatusCode)
	}

	return nil
}

func getUITokens(client *http.Client, baseurl string, username string, password string) (string, string, error) {
	url := fmt.Sprintf("%s/ui/api/v1/access/auth/login", baseurl)

	login := ArtifactoryLogin{
		Username: username,
		Password: password,
	}

	payload, err := json.Marshal(login)
	if err != nil {
		return "", "", fmt.Errorf("error marshalling login: %w", err)
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(payload)))
	if err != nil {
		return "", "", fmt.Errorf("error creating request for '%s': %w", url, err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("error sending request to '%s': %w", url, err)
	}

	fmt.Printf("Status: '%s' %d\n", resp.Status, resp.StatusCode)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		return "", "", fmt.Errorf("unexpected response from '%s': %s - %s", url, resp.Status, string(body))
	}

	accessToken := ""
	refreshToken := ""
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "ACCESSTOKEN" && cookie.Value != "" {
			accessToken = cookie.Value
		}
		if cookie.Name == "REFRESHTOKEN" && cookie.Value != "" {
			refreshToken = cookie.Value
		}
	}

	if accessToken != "" && refreshToken != "" {
		return accessToken, refreshToken, nil
	}

	return "", "", fmt.Errorf("error: unable to obtain UI tokens")
}
