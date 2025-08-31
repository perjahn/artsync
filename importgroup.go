package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

func ImportGroup(
	client *http.Client,
	baseurl string,
	ldapUsername string,
	ldapPassword string,
	groupname string,
	ldapsettings []ArtifactoryLDAPSettings,
	ldapgroupsettings []ArtifactoryLDAPGroupSettings,
	ldapgroupsettingsName string,
	artifactoryUsername string,
	artifactoryPassword string,
	dryRun bool) (bool, error) {

	index := -1
	for i := range ldapgroupsettings {
		if ldapgroupsettings[i].Name == ldapgroupsettingsName {
			index = i
			break
		}
	}
	if index == -1 {
		return false, fmt.Errorf("LDAP group settings named '%s' not found", ldapgroupsettingsName)
	}
	ldapgroupsettingsSingle := ldapgroupsettings[index]

	index = -1
	for i := range ldapsettings {
		if ldapsettings[i].Key == ldapgroupsettingsSingle.EnabledLdap {
			index = i
			break
		}
	}
	if index == -1 {
		return false, fmt.Errorf("LDAP settings named '%s' not found", ldapgroupsettingsSingle.EnabledLdap)
	}
	ldapsettingsSingle := ldapsettings[index]

	var basedn string
	if strings.Count(ldapsettingsSingle.LdapUrl, "/") >= 3 {
		parts := strings.SplitN(ldapsettingsSingle.LdapUrl, "/", 4)
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

	entries, err := queryldap(
		ldapsettingsSingle.LdapUrl,
		basedn,
		filter,
		ldapUsername,
		ldapPassword,
		[]string{ldapgroupsettingsSingle.DescriptionAttribute},
		false,
		false)
	if err != nil {
		return false, fmt.Errorf("query failed: %w", err)
	}

	if len(entries) < 1 {
		fmt.Printf("Didn't find group: '%s'", groupname)
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

	return true, nil
}

func queryldap(server, baseDN, filter, bindDN, bindPW string, attrs []string, startTLS, insecure bool) ([]*ldap.Entry, error) {
	var l *ldap.Conn
	var err error

	if strings.HasPrefix(strings.ToLower(server), "ldaps://") {
		l, err = ldap.DialURL(server, ldap.DialWithTLSConfig(&tls.Config{InsecureSkipVerify: insecure}))
		if err != nil {
			return nil, fmt.Errorf("failed to dial LDAPS: %w", err)
		}
	} else {
		l, err = ldap.DialURL(server)
		if err != nil {
			return nil, fmt.Errorf("failed to dial LDAP: %w", err)
		}
		if startTLS {
			if err = l.StartTLS(&tls.Config{InsecureSkipVerify: insecure}); err != nil {
				l.Close()
				return nil, fmt.Errorf("StartTLS failed: %w", err)
			}
		}
	}
	defer l.Close()

	if bindDN != "" {
		if err = l.Bind(bindDN, bindPW); err != nil {
			return nil, fmt.Errorf("bind failed: %w", err)
		}
	}

	searchReq := ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		filter,
		attrs,
		nil,
	)

	sr, err := l.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	fmt.Printf("Got %d entries\n", len(sr.Entries))
	fmt.Printf("Got %d controls\n", len(sr.Controls))
	fmt.Printf("Got %d referrals\n", len(sr.Referrals))

	return sr.Entries, nil
}

func importSingleGroup(
	client *http.Client,
	baseurl string,
	username string,
	password string,
	groupname string,
	groupimport ArtifactoryGroupImport,
	dryRun bool) error {

	fmt.Printf("Importing group: %s\n", groupname)

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

		fmt.Printf("Created group '%s' via '%s' (status %s)\n", groupname, url, resp.Status)
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
