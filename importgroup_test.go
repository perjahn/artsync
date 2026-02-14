package main

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/go-ldap/ldap/v3"
)

func TestImportGroup(t *testing.T) {
	tests := []struct {
		groupName         string
		ldapSettings      []ArtifactoryLDAPSettings
		ldapGroupSettings []ArtifactoryLDAPGroupSettings
		dryRun            bool
		shouldErr         bool
	}{
		// Minimal
		{"", []ArtifactoryLDAPSettings{}, []ArtifactoryLDAPGroupSettings{}, false, true},
		// Import group
		{
			"test-ldapsettings",
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
			false},
	}

	origQuery := queryldapImportGroupFn
	defer func() { queryldapImportGroupFn = origQuery }()

	for i, tc := range tests {
		var client *http.Client
		client = mockHTTPClient(func(req *http.Request) (*http.Response, error) {
			var reponse *http.Response
			reponse = &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
				Status:     "200 OK",
			}
			reponse.Header.Add("Set-Cookie", "ACCESSTOKEN=access-token; Path=/")
			reponse.Header.Add("Set-Cookie", "REFRESHTOKEN=refresh-token; Path=/")

			return reponse, nil
		})
		queryldapImportGroupFn = func(server, baseDN, filter, bindDN, bindPW string, attrs []string) ([]*ldap.Entry, error) {
			entry := &ldap.Entry{DN: "cn=test-ldapgroupsettings,dc=example,dc=org"}
			entry.Attributes = []*ldap.EntryAttribute{{Name: attrs[0], Values: []string{"Test description"}}}
			return []*ldap.Entry{entry}, nil
		}

		_, err := ImportGroup(client, "", "", "", tc.groupName, tc.ldapSettings, tc.ldapGroupSettings, "", "", tc.dryRun)
		if (tc.shouldErr && err == nil) || (!tc.shouldErr && err != nil) {
			t.Errorf("ImportGroup (%d/%d): error = %v", i+1, len(tests), err)
		}
	}
}
