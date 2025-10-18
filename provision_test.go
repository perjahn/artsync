package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/go-ldap/ldap/v3"
)

func mockHTTPClient(fn func(*http.Request) (*http.Response, error)) *http.Client {
	return &http.Client{
		Transport: roundTripperFunc(fn),
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type testcase struct {
	reposToProvision       []Repo
	repos                  []ArtifactoryRepoDetailsResponse
	users                  []ArtifactoryUser
	groups                 []ArtifactoryGroup
	permissiondetails      []ArtifactoryPermissionDetails
	allowPatterns          bool
	dryRun                 bool
	HTTPResponseUser       string
	HTTPResponseGroup      string
	HTTPResponseRepo       string
	HTTPResponsePermission string
}

func TestProvisionSimple(t *testing.T) {
	tests := []testcase{
		// Minimal
		{[]Repo{}, []ArtifactoryRepoDetailsResponse{}, []ArtifactoryUser{}, []ArtifactoryGroup{}, []ArtifactoryPermissionDetails{}, false, false, "", "", "", ""},
		// allowPatterns
		{[]Repo{}, []ArtifactoryRepoDetailsResponse{}, []ArtifactoryUser{}, []ArtifactoryGroup{}, []ArtifactoryPermissionDetails{}, true, false, "", "", "", ""},
		// dryRun
		{[]Repo{}, []ArtifactoryRepoDetailsResponse{}, []ArtifactoryUser{}, []ArtifactoryGroup{}, []ArtifactoryPermissionDetails{}, false, true, "", "", "", ""},
		// dryRun
		{[]Repo{}, []ArtifactoryRepoDetailsResponse{}, []ArtifactoryUser{}, []ArtifactoryGroup{}, []ArtifactoryPermissionDetails{}, true, true, "", "", "", ""},
	}
	for i, tc := range tests {
		var client *http.Client
		err := Provision(client, "", "", tc.reposToProvision, tc.repos, tc.users, tc.groups, tc.permissiondetails, false, tc.allowPatterns, LdapConfig{}, tc.dryRun)
		if err != nil {
			t.Errorf("ProvisionSimple (%d/%d): error = %v", i+1, len(tests), err)
		}
	}
}

// Provision existing repo/permission
func TestProvisionPermissions(t *testing.T) {
	tc := testcase{
		[]Repo{
			{
				Name:        "test-repo",
				Description: "Test repository new",
				Rclass:      "",
				PackageType: "",
				Layout:      "maven-2-default",
				Read:        []string{"test-user", "test-user"},
				Write:       []string{"test-user"},
				Manage:      []string{"test-user"},
				Scan:        []string{"test-user"},
			},
		},
		[]ArtifactoryRepoDetailsResponse{
			{
				Key:           "test-repo",
				Description:   "Test repository",
				Rclass:        "local",
				PackageType:   "generic",
				RepoLayoutRef: "simple-default",
			},
		},
		[]ArtifactoryUser{{Username: "test-user"}},
		[]ArtifactoryGroup{{GroupName: "test-group"}},
		[]ArtifactoryPermissionDetails{
			{
				Name: "test-repo",
				Resources: ArtifactoryPermissionDetailsResources{
					Artifact: ArtifactoryPermissionDetailsArtifact{
						Actions: ArtifactoryPermissionDetailsActions{
							Users:  map[string][]string{"test-user": {"READ", "WRITE", "OTHER"}},
							Groups: map[string][]string{"test-group": {"READ"}},
						},
						Targets: map[string]ArtifactoryPermissionDetailsTarget{
							"test-repo": {IncludePatterns: []string{"**"}, ExcludePatterns: []string{}},
						},
					},
				},
				JsonSource: `{"aaa":{"bbb":{"actions":{"groups":{},"users":{"test-user":["OTHER","READ","WRITE"]}},"targets":{"test-repo":{"exclude_patterns":[],"include_patterns":["**"]}}}}}`,
			},
		},
		false,
		false,
		"",
		"",
		`{"ok":true}`,
		`{"ok":true}`}

	var callCount int
	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		var reponse *http.Response
		if callCount == 0 {
			reponse = &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.HTTPResponseRepo)), Header: make(http.Header)}
		} else {
			reponse = &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.HTTPResponsePermission)), Header: make(http.Header)}
		}

		callCount++
		return reponse, nil
	})

	var out bytes.Buffer

	origGetWriterFn := GetWriterFn
	defer func() { GetWriterFn = origGetWriterFn }()

	GetWriterFn = func() io.Writer {
		return &out
	}

	err := Provision(client, "", "", tc.reposToProvision, tc.repos, tc.users, tc.groups, tc.permissiondetails, true, tc.allowPatterns, LdapConfig{}, tc.dryRun)
	if err != nil {
		t.Errorf("ProvisionPermissions: error = %v", err)
	}

	consoleDiffOutput := "+         \"MANAGE\",\n+         \"SCAN\",\n"
	got := out.String()
	if strings.TrimSpace(got) != strings.TrimSpace(consoleDiffOutput) {
		t.Errorf("ProvisionPermissions: console output mismatch\n got: %q\nwant: %q", got, consoleDiffOutput)
	}
}

// Create/import new ldap users/groups
func TestProvisionLdap(t *testing.T) {
	tc := testcase{
		[]Repo{
			{
				Name:        "test-repo",
				Description: "Test repository new",
				Rclass:      "",
				PackageType: "",
				Layout:      "maven-2-default",
				Read:        []string{"test-user", "test-user"},
				Write:       []string{"test-user", "test-group"},
				Manage:      []string{"test-user"},
				Scan:        []string{"test-user"},
			},
		},
		[]ArtifactoryRepoDetailsResponse{
			{
				Key:           "test-repo",
				Description:   "Test repository",
				Rclass:        "local",
				PackageType:   "generic",
				RepoLayoutRef: "simple-default",
			},
		},
		[]ArtifactoryUser{{Username: "test-user-not"}},
		[]ArtifactoryGroup{{GroupName: "test-group-not"}},
		[]ArtifactoryPermissionDetails{
			{
				Name: "test-repo",
				Resources: ArtifactoryPermissionDetailsResources{
					Artifact: ArtifactoryPermissionDetailsArtifact{
						Actions: ArtifactoryPermissionDetailsActions{
							Users:  map[string][]string{"test-user": {"READ", "WRITE", "OTHER"}},
							Groups: map[string][]string{"test-group": {"READ"}},
						},
						Targets: map[string]ArtifactoryPermissionDetailsTarget{
							"test-repo": {IncludePatterns: []string{"**"}, ExcludePatterns: []string{}},
						},
					},
				},
			},
		},
		false,
		false,
		`{"ok":true}`,
		`{"ok":true}`,
		`{"ok":true}`,
		`{"ok":true}`}

	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		var bodyStr string
		if req.Body != nil {
			b, _ := io.ReadAll(req.Body)
			bodyStr = string(b)
			req.Body = io.NopCloser(strings.NewReader(bodyStr))
		} else {
			bodyStr = "<nil>"
		}

		fmt.Printf("Mock HTTPClient called with Host='%s', Method='%s', Proto='%s', RequestURI='%s', URL='%s', Body='%s'\n",
			req.Host, req.Method, req.Proto, req.RequestURI, req.URL, bodyStr)

		if req.URL != nil && req.URL.Path == "/access/api/v2/users" && req.Method == "POST" {
			return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(tc.HTTPResponseUser)), Header: make(http.Header)}, nil
		}

		if req.URL != nil && req.URL.Path == "/ui/api/v1/access/auth/login" && req.Method == "POST" {
			resp := &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}

			resp.Header.Add("Set-Cookie", "ACCESSTOKEN=access-token; Path=/")
			resp.Header.Add("Set-Cookie", "REFRESHTOKEN=refresh-token; Path=/")

			return resp, nil
		}

		if req.URL != nil && req.URL.Path == "/ui/api/v1/access/api/ui/ldap/groups/import" && req.Method == "POST" {
			return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(tc.HTTPResponseGroup)), Header: make(http.Header)}, nil
		}

		if req.URL != nil && req.URL.Path == "/artifactory/api/repositories/test-repo" && req.Method == "POST" {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.HTTPResponseRepo)), Header: make(http.Header)}, nil
		}

		if req.URL != nil && req.URL.Path == "/access/api/v2/permissions/test-repo/artifact" && req.Method == "PUT" {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.HTTPResponsePermission)), Header: make(http.Header)}, nil
		}

		fmt.Printf("FAIL\n")

		return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})

	ldapConfig := LdapConfig{
		CreateUsers:         true,
		ImportGroups:        true,
		LdapUsername:        "ldapusername",
		LdapPassword:        "ldappassword",
		Groupsettingsname:   "ldapgroupsettingsname",
		ArtifactoryUsername: "artifactoryusername",
		ArtifactoryPassword: "artifactorypassword",
		Ldapsettings: []ArtifactoryLDAPSettings{
			{
				Key:     "ldapsettings",
				Enabled: true,
				LdapUrl: "ldap://ldap.example.com:389",
				Search: ArtifactoryLDAPSettingsSearch{
					SearchFilter:    "(&(objectClass=user)(cn={0}))",
					SearchBase:      "dc=example,dc=com",
					SearchSubtree:   false,
					ManagerDn:       "",
					ManagerPassword: "",
				},
				AutoCreateUser:           false,
				EmailAttribute:           "mail",
				LdapPositionProtection:   false,
				AllowUserToAccessProfile: false,
				PagingSupportEnabled:     false,
			},
		},
		Ldapgroupsettings: []ArtifactoryLDAPGroupSettings{
			{
				Name:                 "ldapgroupsettingsname",
				EnabledLdap:          "ldapsettings",
				GroupBaseDn:          "dc=example,dc=org",
				GroupNameAttribute:   "cn",
				GroupMemberAttribute: "",
				SubTree:              false,
				ForceAttributeSearch: false,
				Filter:               "(objectClass=group)",
				DescriptionAttribute: "description",
				Strategy:             "",
			},
		},
	}

	origQueryCreateUser := queryldapCreateUserFn
	defer func() { queryldapCreateUserFn = origQueryCreateUser }()

	origQueryImportGroup := queryldapImportGroupFn
	defer func() { queryldapImportGroupFn = origQueryImportGroup }()

	queryldapCreateUserFn = func(server, baseDN, filter, bindDN, bindPW string, attrs []string) ([]*ldap.Entry, error) {
		fmt.Printf("Mock queryldapFn called with server='%s', baseDN='%s', filter='%s', bindDN='%s', bindPW='%s', attrs=%v\n",
			server, baseDN, filter, bindDN, bindPW, attrs)

		if filter == "(&(objectClass=user)(cn=test-user))" && len(attrs) == 1 && attrs[0] == "mail" {
			entry := &ldap.Entry{DN: "cn=test-user,dc=example,dc=org"}
			entry.Attributes = []*ldap.EntryAttribute{{Name: "mail", Values: []string{"noreply@example.com"}}}
			return []*ldap.Entry{entry}, nil
		}

		if filter == "(&(objectClass=group)(cn=test-group))" && len(attrs) == 1 && attrs[0] == "description" {
			entry := &ldap.Entry{DN: "cn=test-group,dc=example,dc=org"}
			entry.Attributes = []*ldap.EntryAttribute{{Name: "description", Values: []string{"Test description"}}}
			return []*ldap.Entry{entry}, nil
		}

		fmt.Printf("FAIL\n")

		return []*ldap.Entry{}, nil
	}

	queryldapImportGroupFn = queryldapCreateUserFn

	err := Provision(client, "", "", tc.reposToProvision, tc.repos, tc.users, tc.groups, tc.permissiondetails, false, tc.allowPatterns, ldapConfig, tc.dryRun)
	if err != nil {
		t.Errorf("ProvisionLdap: unexpected error = %v", err)
	}
}

// Fail on create/import new ldap users/groups
func TestProvisionLdapFail(t *testing.T) {
	tc := testcase{
		[]Repo{
			{
				Name:        "test-repo",
				Description: "Test repository new",
				Rclass:      "",
				PackageType: "",
				Layout:      "maven-2-default",
				Read:        []string{"test-user", "test-user"},
				Write:       []string{"test-user", "test-group"},
				Manage:      []string{"test-user"},
				Scan:        []string{"test-user"},
			},
		},
		[]ArtifactoryRepoDetailsResponse{
			{
				Key:           "test-repo",
				Description:   "Test repository",
				Rclass:        "local",
				PackageType:   "generic",
				RepoLayoutRef: "simple-default",
			},
		},
		[]ArtifactoryUser{{Username: "test-user-not"}},
		[]ArtifactoryGroup{{GroupName: "test-group-not"}},
		[]ArtifactoryPermissionDetails{
			{
				Name: "test-repo",
				Resources: ArtifactoryPermissionDetailsResources{
					Artifact: ArtifactoryPermissionDetailsArtifact{
						Actions: ArtifactoryPermissionDetailsActions{
							Users:  map[string][]string{"test-user": {"READ", "WRITE", "OTHER"}},
							Groups: map[string][]string{"test-group": {"READ"}},
						},
						Targets: map[string]ArtifactoryPermissionDetailsTarget{
							"test-repo": {IncludePatterns: []string{"**"}, ExcludePatterns: []string{}},
						},
					},
				},
			},
		},
		false,
		false,
		`{"ok":true}`,
		`{"ok":true}`,
		`{"ok":true}`,
		`{"ok":true}`}

	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		var bodyStr string
		if req.Body != nil {
			b, _ := io.ReadAll(req.Body)
			bodyStr = string(b)
			req.Body = io.NopCloser(strings.NewReader(bodyStr))
		} else {
			bodyStr = "<nil>"
		}

		fmt.Printf("Mock HTTPClient called with Host='%s', Method='%s', Proto='%s', RequestURI='%s', URL='%s', Body='%s'\n",
			req.Host, req.Method, req.Proto, req.RequestURI, req.URL, bodyStr)

		if req.URL != nil && req.URL.Path == "/access/api/v2/users" && req.Method == "POST" {
			return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(tc.HTTPResponseUser)), Header: make(http.Header)}, nil
		}

		if req.URL != nil && req.URL.Path == "/ui/api/v1/access/auth/login" && req.Method == "POST" {
			resp := &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}

			resp.Header.Add("Set-Cookie", "ACCESSTOKEN=access-token; Path=/")
			resp.Header.Add("Set-Cookie", "REFRESHTOKEN=refresh-token; Path=/")

			return resp, nil
		}

		if req.URL != nil && req.URL.Path == "/ui/api/v1/access/api/ui/ldap/groups/import" && req.Method == "POST" {
			return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(tc.HTTPResponseGroup)), Header: make(http.Header)}, nil
		}

		if req.URL != nil && req.URL.Path == "/artifactory/api/repositories/test-repo" && req.Method == "POST" {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.HTTPResponseRepo)), Header: make(http.Header)}, nil
		}

		if req.URL != nil && req.URL.Path == "/access/api/v2/permissions/test-repo/artifact" && req.Method == "PUT" {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.HTTPResponsePermission)), Header: make(http.Header)}, nil
		}

		fmt.Printf("FAIL\n")

		return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})

	ldapConfig := LdapConfig{
		CreateUsers:         true,
		ImportGroups:        true,
		LdapUsername:        "ldapusername",
		LdapPassword:        "ldappassword",
		Groupsettingsname:   "ldapgroupsettingsname",
		ArtifactoryUsername: "artifactoryusername",
		ArtifactoryPassword: "artifactorypassword",
		Ldapsettings: []ArtifactoryLDAPSettings{
			{
				Key:     "ldapsettings",
				Enabled: true,
				LdapUrl: "ldap://ldap.example.com:389",
				Search: ArtifactoryLDAPSettingsSearch{
					SearchFilter:    "(&(objectClass=user)(cn={0}))",
					SearchBase:      "dc=example,dc=com",
					SearchSubtree:   false,
					ManagerDn:       "",
					ManagerPassword: "",
				},
				AutoCreateUser:           false,
				EmailAttribute:           "mail",
				LdapPositionProtection:   false,
				AllowUserToAccessProfile: false,
				PagingSupportEnabled:     false,
			},
		},
		Ldapgroupsettings: []ArtifactoryLDAPGroupSettings{
			{
				Name:                 "ldapgroupsettingsname",
				EnabledLdap:          "ldapsettings",
				GroupBaseDn:          "dc=example,dc=org",
				GroupNameAttribute:   "cn",
				GroupMemberAttribute: "",
				SubTree:              false,
				ForceAttributeSearch: false,
				Filter:               "(objectClass=group)",
				DescriptionAttribute: "description",
				Strategy:             "",
			},
		},
	}

	origQueryCreateUser := queryldapCreateUserFn
	defer func() { queryldapCreateUserFn = origQueryCreateUser }()

	origQueryImportGroup := queryldapImportGroupFn
	defer func() { queryldapImportGroupFn = origQueryImportGroup }()

	queryldapCreateUserFn = func(server, baseDN, filter, bindDN, bindPW string, attrs []string) ([]*ldap.Entry, error) {
		fmt.Printf("Mock queryldapFn called with server='%s', baseDN='%s', filter='%s', bindDN='%s', bindPW='%s', attrs=%v\n",
			server, baseDN, filter, bindDN, bindPW, attrs)

		fmt.Printf("Simulating locked account for the provided credentials.\n")

		return []*ldap.Entry{}, nil
	}

	queryldapImportGroupFn = queryldapCreateUserFn

	err := Provision(client, "", "", tc.reposToProvision, tc.repos, tc.users, tc.groups, tc.permissiondetails, false, tc.allowPatterns, ldapConfig, tc.dryRun)
	if err != nil {
		t.Errorf("ProvisionLdap: unexpected error = %v", err)
	}
}
