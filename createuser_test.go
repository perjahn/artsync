package main

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/go-ldap/ldap/v3"
)

func TestCreateUser(t *testing.T) {
	tests := []struct {
		userName             string
		groupSettingsName    string
		ldapSettings         []ArtifactoryLDAPSettings
		ldapGroupSettings    []ArtifactoryLDAPGroupSettings
		dryRun               bool
		artifactoryCallCount int
		ldapCallCount        int
		shouldErr            bool
	}{
		// Minimal
		{"", "", nil, nil, false, 0, 0, true},
		// Create user
		{
			"test-user",
			"test-ldapgroupsettings",
			[]ArtifactoryLDAPSettings{
				{
					Key:     "test-ldapsettings",
					Enabled: true,
					LdapUrl: "",
					Search: ArtifactoryLDAPSettingsSearch{
						SearchFilter:    "",
						SearchBase:      "",
						SearchSubtree:   false,
						ManagerDn:       "",
						ManagerPassword: "",
					},
					AutoCreateUser:           false,
					EmailAttribute:           "",
					LdapPositionProtection:   false,
					AllowUserToAccessProfile: false,
					PagingSupportEnabled:     false,
				},
			},
			[]ArtifactoryLDAPGroupSettings{
				{
					Name:                 "test-ldapgroupsettings",
					EnabledLdap:          "test-ldapsettings",
					GroupBaseDn:          "",
					GroupNameAttribute:   "",
					GroupMemberAttribute: "",
					SubTree:              false,
					ForceAttributeSearch: false,
					Filter:               "",
					DescriptionAttribute: "",
					Strategy:             "",
				},
			},
			false,
			1,
			1,
			false},
	}

	origQuery := queryldapCreateUserFn
	defer func() { queryldapCreateUserFn = origQuery }()

	for i, tc := range tests {
		artifactoryCallCount := 0

		var client *http.Client
		client = mockHTTPClient(func(req *http.Request) (*http.Response, error) {
			var reponse *http.Response
			reponse = &http.Response{
				StatusCode: 201,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
				Status:     "201 OK",
			}

			artifactoryCallCount++
			return reponse, nil
		})

		ldapCallCount := 0
		queryldapCreateUserFn = func(server, baseDN, filter, bindDN, bindPW string, attrs []string) ([]*ldap.Entry, error) {
			entry := &ldap.Entry{DN: "cn=test-ldapgroupsettings,dc=example,dc=org"}
			entry.Attributes = []*ldap.EntryAttribute{{Name: attrs[0], Values: []string{"noreply@example.com"}}}

			ldapCallCount++
			return []*ldap.Entry{entry}, nil
		}

		_, err := CreateUser(client, "", "", "", "", tc.userName, tc.ldapSettings, tc.ldapGroupSettings, tc.dryRun)
		if (tc.shouldErr && err == nil) || (!tc.shouldErr && err != nil ||
			(tc.artifactoryCallCount != artifactoryCallCount) || (tc.ldapCallCount != ldapCallCount)) {
			t.Errorf("CreateUser (%d/%d): shouldErr: %t/%t, ldap backend calls: %d/%d, artifactory backend calls: %d/%d, error = %v",
				i+1, len(tests), err == nil, tc.shouldErr, ldapCallCount, tc.ldapCallCount, artifactoryCallCount, tc.artifactoryCallCount, err)
		}
	}
}
