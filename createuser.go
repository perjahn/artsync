package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

// For mock testing.
var queryldapCreateUserFn = queryldap

func CreateUser(
	client *http.Client,
	baseurl string,
	token string,
	ldapUsername string,
	ldapPassword string,
	username string,
	ldapSettings []ArtifactoryLDAPSettings,
	ldapGroupSettings []ArtifactoryLDAPGroupSettings,
	ldapGroupSettingsName string,
	dryRun bool) (bool, error) {

	index := -1
	for i := range ldapGroupSettings {
		if ldapGroupSettings[i].Name == ldapGroupSettingsName {
			index = i
			break
		}
	}
	if index == -1 {
		return false, fmt.Errorf("LDAP group settings named '%s' not found", ldapGroupSettingsName)
	}
	ldapGroupSettingsSingle := ldapGroupSettings[index]

	index = -1
	for i := range ldapSettings {
		if ldapSettings[i].Key == ldapGroupSettingsSingle.EnabledLdap {
			index = i
			break
		}
	}
	if index == -1 {
		return false, fmt.Errorf("LDAP settings named '%s' not found", ldapGroupSettingsSingle.EnabledLdap)
	}
	ldapSettingsSingle := ldapSettings[index]

	found := false
	var entries []*ldap.Entry
	var err error

	basednParts := strings.SplitSeq(ldapSettingsSingle.Search.SearchBase, "|")
	for basednPart := range basednParts {
		var basedn string

		if strings.Count(ldapSettingsSingle.LdapUrl, "/") >= 3 {
			parts := strings.SplitN(ldapSettingsSingle.LdapUrl, "/", 4)
			if ldapSettingsSingle.Search.SearchBase != "" {
				basedn = fmt.Sprintf("%s,%s", basednPart, parts[3])
			} else {
				basedn = parts[3]
			}
		} else {
			basedn = basednPart
		}

		filter := strings.ReplaceAll(ldapSettingsSingle.Search.SearchFilter, "{0}", username)

		entries, err = queryldapCreateUserFn(
			ldapSettingsSingle.LdapUrl,
			basedn,
			filter,
			ldapUsername,
			ldapPassword,
			[]string{ldapSettingsSingle.EmailAttribute})
		if err != nil {
			return false, fmt.Errorf("query failed: %w", err)
		}

		if len(entries) > 1 {
			return false, fmt.Errorf("error: multiple DNs found for user: '%s' in base dn: '%s'", username, basedn)
		}

		if len(entries) == 1 {
			found = true
			break
		}
	}

	if !found {
		fmt.Printf("Didn't find user: '%s'\n", username)
		return false, nil
	}

	entry := entries[0]

	values := entry.GetAttributeValues(ldapSettingsSingle.EmailAttribute)
	emailaddress := ""
	if len(values) >= 1 {
		emailaddress = values[0]
	}
	fmt.Printf("emailaddress: '%s'\n", emailaddress)

	err = createSingleUser(client, baseurl, token, username, emailaddress, dryRun)
	if err != nil {
		return false, fmt.Errorf("creating failed: %w", err)
	}

	return true, nil
}

func createSingleUser(client *http.Client, baseurl, token, username, emailaddress string, dryRun bool) error {
	url := fmt.Sprintf("%s/access/api/v2/users", baseurl)

	artifactoryUserRequest := ArtifactoryUserRequest{
		Username:                 username,
		Email:                    emailaddress,
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
