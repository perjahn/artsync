package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
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
	dryRun bool) (bool, error) {

	if len(ldapSettings) == 0 {
		return false, fmt.Errorf("missing LDAP settings")
	}

	for _, ldapSettingsSingle := range ldapSettings {
		log.Printf("Creating user: '%s', ldap settings: %d, settings name: '%s'\n",
			username, len(ldapSettings), ldapSettingsSingle.Key)

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

			entries, err := queryldapCreateUserFn(
				ldapSettingsSingle.LdapUrl,
				basedn,
				filter,
				ldapUsername,
				ldapPassword,
				[]string{ldapSettingsSingle.EmailAttribute, "userPrincipalName"})
			if err != nil {
				return false, fmt.Errorf("query failed: %w", err)
			}

			log.Printf("%s: %s: %d\n", ldapSettingsSingle.Key, basednPart, len(entries))

			if len(entries) < 1 {
				continue
			}
			if len(entries) > 1 {
				return false, fmt.Errorf("error: multiple DNs found for user: '%s' in base dn: '%s'", username, basedn)
			}
			entry := entries[0]

			values := entry.GetAttributeValues(ldapSettingsSingle.EmailAttribute)
			emailaddress := ""
			if len(values) >= 1 {
				emailaddress = values[0]
			}
			if emailaddress == "" {
				values = entry.GetAttributeValues("userPrincipalName")
				if len(values) >= 1 {
					emailaddress = values[0]
				}
			}
			log.Printf("emailaddress: '%s'\n", emailaddress)

			err = createSingleUser(client, baseurl, token, username, emailaddress, dryRun)
			if err != nil {
				return false, fmt.Errorf("creating failed: %w", err)
			}

			return true, nil
		}
	}

	fmt.Printf("Didn't find user: '%s'\n", username)
	return false, nil
}

func createSingleUser(client *http.Client, baseurl, token, username, emailaddress string, dryRun bool) error {
	fmt.Printf("Creating user: '%s'\n", username)

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
			log.Printf("'%s': Created user successfully.\n", username)
		}
	}

	return nil
}
