package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
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
	properties             []repoProperties
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
		{[]Repo{}, []ArtifactoryRepoDetailsResponse{}, []ArtifactoryUser{}, []ArtifactoryGroup{}, []ArtifactoryPermissionDetails{}, []repoProperties{}, false, false, "", "", "", ""},
		// allowPatterns
		{[]Repo{}, []ArtifactoryRepoDetailsResponse{}, []ArtifactoryUser{}, []ArtifactoryGroup{}, []ArtifactoryPermissionDetails{}, []repoProperties{}, true, false, "", "", "", ""},
		// dryRun
		{[]Repo{}, []ArtifactoryRepoDetailsResponse{}, []ArtifactoryUser{}, []ArtifactoryGroup{}, []ArtifactoryPermissionDetails{}, []repoProperties{}, false, true, "", "", "", ""},
		// dryRun
		{[]Repo{}, []ArtifactoryRepoDetailsResponse{}, []ArtifactoryUser{}, []ArtifactoryGroup{}, []ArtifactoryPermissionDetails{}, []repoProperties{}, true, true, "", "", "", ""},
	}
	for i, tc := range tests {
		var client *http.Client
		err := Provision(client, "", "", tc.reposToProvision, tc.repos, tc.users, tc.groups, tc.permissiondetails, tc.properties, false, tc.allowPatterns, LdapConfig{}, PropertiesConfig{}, tc.dryRun)
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
		[]repoProperties{},
		false,
		false,
		"",
		"",
		`{"ok":true}`,
		`{"ok":true}`}

	var callCount int
	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		var response *http.Response
		if callCount == 0 {
			response = &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.HTTPResponseRepo)), Header: make(http.Header)}
		} else {
			response = &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.HTTPResponsePermission)), Header: make(http.Header)}
		}

		callCount++
		return response, nil
	})

	err := Provision(client, "", "", tc.reposToProvision, tc.repos, tc.users, tc.groups, tc.permissiondetails, tc.properties, true, tc.allowPatterns, LdapConfig{}, PropertiesConfig{}, tc.dryRun)
	if err != nil {
		t.Errorf("ProvisionPermissions: error = %v", err)
	}

	oldJson := `{"aaa":{"bbb":{"actions":{"groups":{},"users":{"test-user":["OTHER","READ","WRITE"]}},"targets":{"test-repo":{"exclude_patterns":[],"include_patterns":["**"]}}}}}`
	newJson := `{"aaa":{"bbb":{"actions":{"groups":{},"users":{"test-user":["MANAGE","OTHER","READ","SCAN","WRITE"]}},"targets":{"test-repo":{"exclude_patterns":[],"include_patterns":["**"]}}}}}`

	got, err := PrintDiff(oldJson, newJson, true)
	if err != nil {
		t.Errorf("ProvisionPermissions: PrintDiff error = %v", err)
	}

	consoleDiffOutput := "MANAGE"
	if !strings.Contains(got, consoleDiffOutput) {
		t.Errorf("ProvisionPermissions: console output mismatch - missing MANAGE\n got: %q", got)
	}
	if !strings.Contains(got, "SCAN") {
		t.Errorf("ProvisionPermissions: console output mismatch - missing SCAN\n got: %q", got)
	}
}

func TestProvisionRenamedPermissions(t *testing.T) {
	tc := testcase{
		[]Repo{
			{
				Name:           "test-repo",
				Description:    "Test repository new",
				Rclass:         "",
				PackageType:    "",
				Layout:         "maven-2-default",
				PermissionName: "test-permission",
				Read:           []string{"test-user", "test-user"},
				Write:          []string{"test-user"},
				Manage:         []string{"test-user"},
				Scan:           []string{"test-user"},
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
				Name: "test-permission",
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
		[]repoProperties{},
		false,
		false,
		"",
		"",
		`{"ok":true}`,
		`{"ok":true}`}

	var callCount int
	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		var response *http.Response
		if callCount == 0 {
			response = &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.HTTPResponseRepo)), Header: make(http.Header)}
		} else {
			response = &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.HTTPResponsePermission)), Header: make(http.Header)}
		}

		callCount++
		return response, nil
	})

	err := Provision(client, "", "", tc.reposToProvision, tc.repos, tc.users, tc.groups, tc.permissiondetails, tc.properties, true, tc.allowPatterns, LdapConfig{}, PropertiesConfig{}, tc.dryRun)
	if err != nil {
		t.Errorf("ProvisionRenamedPermissions: error = %v", err)
	}

	oldJson := `{"aaa":{"bbb":{"actions":{"groups":{},"users":{"test-user":["OTHER","READ","WRITE"]}},"targets":{"test-repo":{"exclude_patterns":[],"include_patterns":["**"]}}}}}`
	newJson := `{"aaa":{"bbb":{"actions":{"groups":{},"users":{"test-user":["MANAGE","OTHER","READ","SCAN","WRITE"]}},"targets":{"test-repo":{"exclude_patterns":[],"include_patterns":["**"]}}}}}`

	got, err := PrintDiff(oldJson, newJson, true)
	if err != nil {
		t.Errorf("ProvisionRenamedPermissions: PrintDiff error = %v", err)
	}

	consoleDiffOutput := "MANAGE"
	if !strings.Contains(got, consoleDiffOutput) {
		t.Errorf("ProvisionRenamedPermissions: console output mismatch - missing MANAGE\n got: %q", got)
	}
	if !strings.Contains(got, "SCAN") {
		t.Errorf("ProvisionRenamedPermissions: console output mismatch - missing SCAN\n got: %q", got)
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
		[]repoProperties{},
		false,
		false,
		`{"ok":true}`,
		`{"ok":true}`,
		`{"ok":true}`,
		`{"ok":true}`}

	var unexpectedHttpRequest bool

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

		unexpectedHttpRequest = true

		return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})

	ldapConfig := LdapConfig{
		ImportUsersAndGroups: true,
		LdapUsername:         "ldapusername",
		LdapPassword:         "ldappassword",
		ArtifactoryUsername:  "artifactoryusername",
		ArtifactoryPassword:  "artifactorypassword",
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

	var unexpectedLdapQuery bool

	origQueryCreateUser := queryldapCreateUserFn
	defer func() { queryldapCreateUserFn = origQueryCreateUser }()

	origQueryImportGroup := queryldapImportGroupFn
	defer func() { queryldapImportGroupFn = origQueryImportGroup }()

	queryldapCreateUserFn = func(server, baseDN, filter, bindDN, bindPW string, attrs []string) ([]*ldap.Entry, error) {
		fmt.Printf("Mock queryldapFn called with server='%s', baseDN='%s', filter='%s', bindDN='%s', bindPW='%s', attrs=%v\n",
			server, baseDN, filter, bindDN, bindPW, attrs)

		if filter == "(&(objectClass=user)(cn=test-user))" && len(attrs) == 2 && attrs[0] == "mail" && attrs[1] == "userPrincipalName" {
			entry := &ldap.Entry{DN: "cn=test-user,dc=example,dc=org"}
			entry.Attributes = []*ldap.EntryAttribute{{Name: "mail", Values: []string{"noreply@example.com"}}, {Name: "userPrincipalName", Values: []string{"test-user@example.com"}}}
			return []*ldap.Entry{entry}, nil
		}

		if filter == "(&(objectClass=group)(cn=test-group))" && len(attrs) == 1 && attrs[0] == "description" {
			entry := &ldap.Entry{DN: "cn=test-group,dc=example,dc=org"}
			entry.Attributes = []*ldap.EntryAttribute{{Name: "description", Values: []string{"Test description"}}}
			return []*ldap.Entry{entry}, nil
		}

		if filter == "(&(objectClass=group)(cn=test-user))" && len(attrs) == 1 && attrs[0] == "description" {
			return []*ldap.Entry{}, nil
		}

		unexpectedLdapQuery = true

		return []*ldap.Entry{}, nil
	}

	queryldapImportGroupFn = queryldapCreateUserFn

	err := Provision(client, "", "", tc.reposToProvision, tc.repos, tc.users, tc.groups, tc.permissiondetails, tc.properties, false, tc.allowPatterns, ldapConfig, PropertiesConfig{}, tc.dryRun)
	if err != nil {
		t.Errorf("ProvisionLdap: unexpected error = %v", err)
	}

	if unexpectedHttpRequest {
		t.Errorf("ProvisionLdap: unexpected http request received")
	}

	if unexpectedLdapQuery {
		t.Errorf("ProvisionLdap: unexpected ldap query received")
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
		[]repoProperties{},
		false,
		false,
		`{"ok":true}`,
		`{"ok":true}`,
		`{"ok":true}`,
		`{"ok":true}`}

	var unexpectedHttpRequest bool

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

		unexpectedHttpRequest = true

		return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})

	ldapConfig := LdapConfig{
		ImportUsersAndGroups: true,
		LdapUsername:         "ldapusername",
		LdapPassword:         "ldappassword",
		ArtifactoryUsername:  "artifactoryusername",
		ArtifactoryPassword:  "artifactorypassword",
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

	err := Provision(client, "", "", tc.reposToProvision, tc.repos, tc.users, tc.groups, tc.permissiondetails, tc.properties, false, tc.allowPatterns, ldapConfig, PropertiesConfig{}, tc.dryRun)
	if err != nil {
		t.Errorf("ProvisionLdapFail: unexpected error = %v", err)
	}

	if unexpectedHttpRequest {
		t.Errorf("ProvisionLdapFail: unexpected http request received")
	}
}

// Test setRepoProperties function
func TestSetRepoProperties(t *testing.T) {
	ClearStats()
	repo := Repo{
		Name:        "test-repo",
		Description: "Test repository",
		Rclass:      "local",
		PackageType: "docker",
		ExtraFields: map[string][]string{
			"owner":       {"admin"},
			"semicolon":   {"left;right"},
			"version":     {"1.0", "2.0"},
			"special,key": {"left,right", `back\slash`, "pipe|value", "equals=value"},
		},
	}

	propertiesConfig := PropertiesConfig{
		SetProperties: true,
		Prefix:        "mycorp",
		Url:           "https://example.com/docs",
	}
	repo = addUrlToRepoProperties(repo, propertiesConfig)

	var capturedUrl string
	var capturedMethod string
	var requestCount int

	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		capturedUrl = req.URL.String()
		capturedMethod = req.Method

		authHeader := req.Header.Get("Authorization")
		if authHeader != "Bearer test-token" {
			t.Errorf("SetRepoProperties: expected Authorization header 'Bearer test-token', got '%s'", authHeader)
		}

		if req.Method == "GET" {
			requestCount++
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"properties":{}}`)),
				Header:     make(http.Header),
			}, nil
		}

		requestCount++
		return &http.Response{
			StatusCode: 204,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})

	err := setRepoProperties(client, "https://artifactory.example.com", "test-token", repo, map[string][]string{}, propertiesConfig, false)
	if err != nil {
		t.Errorf("SetRepoProperties: unexpected error = %v", err)
	}

	if capturedMethod != "PUT" {
		t.Errorf("SetRepoProperties: expected last method PUT, got %s", capturedMethod)
	}

	if !strings.Contains(capturedUrl, "https://artifactory.example.com/artifactory/api/storage/test-repo?properties=") {
		t.Errorf("SetRepoProperties: url format incorrect: %s", capturedUrl)
	}

	if !strings.Contains(capturedUrl, "mycorp.url=https%3A%2F%2Fexample.com%2Fdocs") {
		t.Errorf("SetRepoProperties: missing url property in %s", capturedUrl)
	}

	if !strings.Contains(capturedUrl, "mycorp.owner=admin") {
		t.Errorf("SetRepoProperties: missing owner property in %s", capturedUrl)
	}

	if !strings.Contains(capturedUrl, "mycorp.version=1.0,2.0") {
		t.Errorf("SetRepoProperties: missing comma-separated multi-value property in %s", capturedUrl)
	}
	if !strings.Contains(capturedUrl, "mycorp.semicolon=left,right") {
		t.Errorf("SetRepoProperties: semicolon-separated values should use a comma in %s", capturedUrl)
	}
	if !strings.Contains(capturedUrl, "mycorp.special%5C,key=left%5C,right,back%5C%5Cslash,pipe%5C|value,equals%5C=value") {
		t.Errorf("SetRepoProperties: special characters not encoded with Artifactory escaping in %s", capturedUrl)
	}

	orderedProperties := []string{
		"mycorp.owner=admin",
		"mycorp.semicolon=left,right",
		"mycorp.special%5C,key=",
		"mycorp.url=https%3A%2F%2Fexample.com%2Fdocs",
		"mycorp.version=1.0,2.0",
	}
	previousPosition := -1
	for _, property := range orderedProperties {
		position := strings.Index(capturedUrl, property)
		if position <= previousPosition {
			t.Errorf("SetRepoProperties: property %q is missing or out of order in %s", property, capturedUrl)
		}
		previousPosition = position
	}
}

func TestSetRepoPropertiesCountsCreatedAndUpdated(t *testing.T) {
	ClearStats()
	repo := Repo{
		Name: "test-repo",
		ExtraFields: map[string][]string{
			"new":     {"value"},
			"changed": {"new-value"},
			"same":    {"same-value"},
		},
	}
	propertiesConfig := PropertiesConfig{SetProperties: true, Prefix: "mycorp"}
	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 204,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})

	err := setRepoProperties(client, "https://artifactory.example.com", "test-token", repo, map[string][]string{
		"mycorp.changed": {"old-value"},
		"mycorp.same":    {"same-value"},
	}, propertiesConfig, false)
	if err != nil {
		t.Fatalf("setRepoProperties() error = %v", err)
	}
	if stats.CreatedPropertyCount != 1 {
		t.Errorf("CreatedPropertyCount = %d, want 1", stats.CreatedPropertyCount)
	}
	if stats.UpdatedPropertyCount != 1 {
		t.Errorf("UpdatedPropertyCount = %d, want 1", stats.UpdatedPropertyCount)
	}
}

func TestSetRepoPropertiesRemoteRepoUsesCache(t *testing.T) {
	ClearStats()
	var capturedMethods []string
	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		capturedMethods = append(capturedMethods, req.Method)
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})

	repo := Repo{
		Name:   "remote-repo",
		Rclass: "remote",
		Url:    "https://remote.example.com/repo",
		ExtraFields: map[string][]string{
			"owner": {"admin"},
		},
	}
	propertiesConfig := PropertiesConfig{SetProperties: true, Prefix: "mycorp"}

	err := setRepoProperties(client, "https://artifactory.example.com", "test-token", repo, map[string][]string{
		"mycorp.old": {"value"},
	}, propertiesConfig, false)
	if err != nil {
		t.Fatalf("setRepoProperties() error = %v", err)
	}
	if !slices.Equal(capturedMethods, []string{"PUT"}) {
		t.Fatalf("request methods = %v, want [PUT] after cache-missing response", capturedMethods)
	}
	if stats.CreatedPropertyCount != 0 || stats.UpdatedPropertyCount != 0 {
		t.Errorf("property counts = created %d, updated %d; want both 0 when cache is missing", stats.CreatedPropertyCount, stats.UpdatedPropertyCount)
	}
	if stats.RemoteRepoMissingCount != 1 {
		t.Errorf("RemoteRepoMissingCount = %d, want 1", stats.RemoteRepoMissingCount)
	}
}

func TestSetRepoPropertiesVirtualRepoIsSkipped(t *testing.T) {
	requestCount := 0
	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		requestCount++
		return &http.Response{
			StatusCode: 204,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})

	repo := Repo{
		Name:   "virtual-repo",
		Rclass: "virtual",
		ExtraFields: map[string][]string{
			"owner": {"admin"},
		},
	}
	propertiesConfig := PropertiesConfig{SetProperties: true, Prefix: "mycorp"}

	err := setRepoProperties(client, "https://artifactory.example.com", "test-token", repo, nil, propertiesConfig, false)
	if err != nil {
		t.Fatalf("setRepoProperties() error = %v", err)
	}
	if requestCount != 0 {
		t.Fatalf("HTTP requests = %d, want 0 for virtual repo", requestCount)
	}
}

// Test setRepoProperties with empty ExtraFields (should not fail, just return nil)
func TestSetRepoPropertiesEmpty(t *testing.T) {
	repo := Repo{
		Name:        "test-repo",
		ExtraFields: map[string][]string{},
	}

	propertiesConfig := PropertiesConfig{
		SetProperties: true,
		Prefix:        "mycorp",
		Url:           "abc123",
	}
	repo = addUrlToRepoProperties(repo, propertiesConfig)

	var requestCount int

	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		requestCount++

		if req.Method == "GET" {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		}

		return &http.Response{
			StatusCode: 204,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})

	err := setRepoProperties(client, "https://artifactory.example.com", "test-token", repo, map[string][]string{}, propertiesConfig, false)
	if err != nil {
		t.Errorf("SetRepoPropertiesEmpty: unexpected error = %v", err)
	}

	if requestCount == 0 {
		t.Errorf("SetRepoPropertiesEmpty: http call should be made when url property exists")
	}
}

// Test setRepoProperties deletes unused properties
func TestSetRepoPropertiesDeleteUnused(t *testing.T) {
	ClearStats()
	repo := Repo{
		Name:        "test-repo",
		Description: "Test repository",
		Rclass:      "local",
		PackageType: "docker",
		ExtraFields: map[string][]string{
			"owner": {"admin"},
		},
	}

	propertiesConfig := PropertiesConfig{
		SetProperties: true,
		Prefix:        "mycorp",
		Url:           "https://example.com/docs",
	}
	repo = addUrlToRepoProperties(repo, propertiesConfig)

	var requests []string

	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, fmt.Sprintf("%s %s", req.Method, req.URL.String()))

		if req.Method == "GET" {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"properties":{"mycorp.oldprop":["oldvalue"],"mycorp.owner":["admin"],"other.key":["value"]}}`)),
				Header:     make(http.Header),
			}, nil
		}

		return &http.Response{
			StatusCode: 204,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})

	err := setRepoProperties(client, "https://artifactory.example.com", "test-token", repo, map[string][]string{
		"mycorp.oldprop": {"oldvalue"},
		"mycorp.old,key": {"oldvalue"},
		"mycorp.owner":   {"admin"},
		"mycorp.url":     {"https://example.com/docs"},
		"other.key":      {"value"},
	}, propertiesConfig, false)
	if err != nil {
		t.Errorf("SetRepoPropertiesDeleteUnused: unexpected error = %v", err)
	}

	var deleteCalled bool
	var deleteUrl string
	for _, req := range requests {
		if strings.HasPrefix(req, "DELETE") {
			deleteCalled = true
			deleteUrl = req
			break
		}
	}

	if !deleteCalled {
		t.Errorf("SetRepoPropertiesDeleteUnused: expected DELETE request for unused properties")
	}

	if !strings.Contains(deleteUrl, "mycorp.oldprop") {
		t.Errorf("SetRepoPropertiesDeleteUnused: DELETE should include mycorp.oldprop, got %s", deleteUrl)
	}
	if !strings.Contains(deleteUrl, "mycorp.old%5C,key") {
		t.Errorf("SetRepoPropertiesDeleteUnused: DELETE should use Artifactory escaping for commas, got %s", deleteUrl)
	}
	if !strings.Contains(deleteUrl, "properties=mycorp.old%5C,key,mycorp.oldprop&recursive=0") {
		t.Errorf("SetRepoPropertiesDeleteUnused: DELETE properties should be alphabetical, got %s", deleteUrl)
	}
	if strings.Contains(deleteUrl, ";") {
		t.Errorf("SetRepoPropertiesDeleteUnused: DELETE property keys should be comma-separated, got %s", deleteUrl)
	}

	if strings.Contains(deleteUrl, "other.key") {
		t.Errorf("SetRepoPropertiesDeleteUnused: DELETE should NOT include other.key (different prefix), got %s", deleteUrl)
	}
	if stats.DeletedPropertyCount != 2 {
		t.Errorf("DeletedPropertyCount = %d, want 2", stats.DeletedPropertyCount)
	}
}

func TestSetRepoPropertiesRemoteDeleteMissingCache(t *testing.T) {
	ClearStats()
	var capturedPath string
	var capturedMethods []string
	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		capturedMethods = append(capturedMethods, req.Method)
		if req.Method == "DELETE" {
			capturedPath = req.URL.Path
		}
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})

	repo := Repo{
		Name:   "remote-repo",
		Rclass: "remote",
	}
	propertiesConfig := PropertiesConfig{SetProperties: true, Prefix: "mycorp"}
	err := setRepoProperties(client, "https://artifactory.example.com", "test-token", repo, map[string][]string{
		"mycorp.old": {"value"},
	}, propertiesConfig, false)
	if err != nil {
		t.Fatalf("setRepoProperties() error = %v", err)
	}
	if capturedPath != "/artifactory/api/storage/remote-repo-cache" {
		t.Errorf("request path = %q, want remote cache path", capturedPath)
	}
	if !slices.Equal(capturedMethods, []string{"DELETE"}) {
		t.Errorf("request methods = %v, want [DELETE]", capturedMethods)
	}
	if stats.DeletedPropertyCount != 0 {
		t.Errorf("DeletedPropertyCount = %d, want 0 for missing cache", stats.DeletedPropertyCount)
	}
	if stats.RemoteRepoMissingCount != 1 {
		t.Errorf("RemoteRepoMissingCount = %d, want 1", stats.RemoteRepoMissingCount)
	}
}

// Test setRepoProperties only updates when needed
func TestSetRepoPropertiesNoUpdate(t *testing.T) {
	repo := Repo{
		Name:        "test-repo",
		Description: "Test repository",
		Rclass:      "local",
		PackageType: "docker",
		ExtraFields: map[string][]string{
			"owner": {"admin"},
		},
	}

	propertiesConfig := PropertiesConfig{
		SetProperties: true,
		Prefix:        "mycorp",
		Url:           "https://example.com/docs",
	}
	repo = addUrlToRepoProperties(repo, propertiesConfig)

	var requests []string

	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, fmt.Sprintf("%s %s", req.Method, req.URL.String()))

		if req.Method == "GET" {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"properties":{"mycorp.url":["https://example.com/docs"],"mycorp.owner":["admin"]}}`)),
				Header:     make(http.Header),
			}, nil
		}

		return &http.Response{
			StatusCode: 204,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})

	err := setRepoProperties(client, "https://artifactory.example.com", "test-token", repo, map[string][]string{
		"mycorp.url":   {"https://example.com/docs"},
		"mycorp.owner": {"admin"},
		"other.key":    {"preserve me"},
	}, propertiesConfig, false)
	if err != nil {
		t.Errorf("SetRepoPropertiesNoUpdate: unexpected error = %v", err)
	}

	var putCalled, deleteCalled bool
	for _, req := range requests {
		if strings.HasPrefix(req, "PUT") {
			putCalled = true
		}
		if strings.HasPrefix(req, "DELETE") {
			deleteCalled = true
		}
	}

	if putCalled {
		t.Errorf("SetRepoPropertiesNoUpdate: should not call PUT when properties match")
	}

	if deleteCalled {
		t.Errorf("SetRepoPropertiesNoUpdate: should not call DELETE when no unused properties exist")
	}
}

func TestRepoPropertiesDiff(t *testing.T) {
	diffText, err := repoPropertiesDiff(
		map[string][]string{
			"mycorp.owner":  {"old"},
			"mycorp.legacy": {"remove me"},
			"other.key":     {"leave unchanged"},
		},
		map[string][]string{
			"mycorp.owner":   {"new"},
			"mycorp.version": {"1.0"},
		},
		"mycorp.",
	)
	if err != nil {
		t.Fatalf("repoPropertiesDiff() error = %v", err)
	}

	var removedOld, addedNew, removedLegacy, addedVersion bool
	for _, line := range strings.Split(diffText, "\n") {
		removedOld = removedOld || (strings.HasPrefix(line, "-") && strings.Contains(line, `"old"`))
		addedNew = addedNew || (strings.HasPrefix(line, "+") && strings.Contains(line, `"new"`))
		removedLegacy = removedLegacy || (strings.HasPrefix(line, "-") && strings.Contains(line, `"mycorp.legacy"`))
		addedVersion = addedVersion || (strings.HasPrefix(line, "+") && strings.Contains(line, `"mycorp.version"`))
	}
	if !removedOld || !addedNew || !removedLegacy || !addedVersion {
		t.Fatalf("diff missing expected additions or removals:\n%s", diffText)
	}
	if strings.Contains(diffText, "other.key") || strings.Contains(diffText, "leave unchanged") {
		t.Fatalf("diff included a property outside the configured prefix:\n%s", diffText)
	}
}

func TestHasPropertiesDiff(t *testing.T) {
	repo := Repo{
		Name: "test-repo",
		ExtraFields: map[string][]string{
			"owners": {"admin", "release"},
		},
	}
	propertiesConfig := PropertiesConfig{SetProperties: true, Prefix: "mycorp"}

	tests := []struct {
		name       string
		properties map[string][]string
		wantDiff   bool
	}{
		{
			name: "matching values in the same order",
			properties: map[string][]string{
				"mycorp.owners": {"admin", "release"},
			},
			wantDiff: false,
		},
		{
			name: "different value order",
			properties: map[string][]string{
				"mycorp.owners": {"release", "admin"},
			},
			wantDiff: true,
		},
		{
			name: "different values",
			properties: map[string][]string{
				"mycorp.owners": {"admin", "viewer"},
			},
			wantDiff: true,
		},
		{
			name: "stale prefixed property",
			properties: map[string][]string{
				"mycorp.owners": {"admin", "release"},
				"mycorp.legacy": {"old"},
			},
			wantDiff: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			properties := []repoProperties{{Repo: repo.Name, Properties: test.properties}}
			gotDiff, _ := hasPropertiesDiff(repo, properties, propertiesConfig)
			if gotDiff != test.wantDiff {
				t.Errorf("hasPropertiesDiff() = %t, want %t", gotDiff, test.wantDiff)
			}
		})
	}
}

func TestHasPropertiesDiffRemoteRepoUsesCacheProperties(t *testing.T) {
	repo := Repo{
		Name:   "test-remote",
		Rclass: "remote",
		ExtraFields: map[string][]string{
			"owner": {"admin"},
		},
	}
	properties := []repoProperties{{
		Repo: "test-remote-cache",
		Properties: map[string][]string{
			"mycorp.owner": {"admin"},
		},
	}}
	propertiesConfig := PropertiesConfig{SetProperties: true, Prefix: "mycorp"}

	gotDiff, existingProperties := hasPropertiesDiff(repo, properties, propertiesConfig)
	if gotDiff {
		t.Fatal("hasPropertiesDiff() = true for matching properties on remote cache")
	}
	if existingProperties.Repo != "test-remote-cache" {
		t.Fatalf("existing properties repo = %q, want %q", existingProperties.Repo, "test-remote-cache")
	}
}

func TestHasPropertiesDiffSemicolonSeparatedValues(t *testing.T) {
	repo := Repo{
		Name: "test-repo",
		ExtraFields: map[string][]string{
			"colors": {"red;blue"},
		},
	}
	properties := []repoProperties{{
		Repo: repo.Name,
		Properties: map[string][]string{
			"mycorp.colors": {"red", "blue"},
		},
	}}
	propertiesConfig := PropertiesConfig{SetProperties: true, Prefix: "mycorp"}

	gotDiff, _ := hasPropertiesDiff(repo, properties, propertiesConfig)
	if gotDiff {
		t.Fatal("hasPropertiesDiff() = true for semicolon-separated values already stored as an array")
	}
}

func TestFindReposWithDiffsAddsConfiguredURLProperty(t *testing.T) {
	repo := Repo{Name: "test-repo"}
	propertiesConfig := PropertiesConfig{
		SetProperties: true,
		Prefix:        "mycorp",
		Url:           "https://example.com/docs",
	}

	reposWithDiffs := findReposWithDiffs(
		[]Repo{repo},
		nil,
		nil,
		nil,
		[]repoProperties{{
			Repo:       repo.Name,
			Properties: map[string][]string{"mycorp.url": {"old-url"}},
		}},
		false,
		propertiesConfig,
	)
	if len(reposWithDiffs) != 1 {
		t.Fatalf("expected one repo with diffs, got %d", len(reposWithDiffs))
	}
	if !reposWithDiffs[0].hasPropDiff {
		t.Fatal("expected configured URL change to produce a property diff")
	}
	if got := reposWithDiffs[0].repo.ExtraFields["url"]; !slices.Equal(got, []string{propertiesConfig.Url}) {
		t.Fatalf("configured URL in ExtraFields = %v, want [%s]", got, propertiesConfig.Url)
	}
	if repo.ExtraFields != nil {
		t.Fatal("findReposWithDiffs should not mutate the input repo")
	}
}

func TestSetRepoPropertiesDeletesAllWhenDesiredIsEmpty(t *testing.T) {
	var methods []string
	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		methods = append(methods, req.Method)
		return &http.Response{
			StatusCode: 204,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})

	err := setRepoProperties(
		client,
		"https://artifactory.example.com",
		"test-token",
		Repo{Name: "test-repo", ExtraFields: map[string][]string{}},
		map[string][]string{"mycorp.old": {"value"}},
		PropertiesConfig{SetProperties: true, Prefix: "mycorp"},
		false,
	)
	if err != nil {
		t.Fatalf("setRepoProperties() error = %v", err)
	}
	if !slices.Equal(methods, []string{"DELETE"}) {
		t.Fatalf("HTTP methods = %v, want [DELETE]", methods)
	}
}

func TestProvisionCreateVirtualRepo(t *testing.T) {
	// capture fmt.Println to assert
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	tc := testcase{
		[]Repo{
			{
				Name:         "test-repo",
				Description:  "Test repository",
				Rclass:       "virtual",
				PackageType:  "docker",
				Layout:       "",
				Repositories: []string{"test1", "test2"},
			},
		},
		[]ArtifactoryRepoDetailsResponse{},
		[]ArtifactoryUser{},
		[]ArtifactoryGroup{},
		[]ArtifactoryPermissionDetails{},
		[]repoProperties{},
		false,
		false,
		"",
		"",
		`{"ok":true}`,
		`{"ok":true}`}

	var callCount int
	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		var response *http.Response
		if callCount == 0 {
			response = &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.HTTPResponseRepo)), Header: make(http.Header)}
		} else {
			t.Error("ProvisionCreateVirtualRepo: Unexpected second call for permissions targets when handling virtual repo")
		}
		callCount++

		body, _ := io.ReadAll(req.Body)
		if !strings.Contains(string(body), `"repositories":["test1","test2"]`) {
			t.Error("ProvisionCreateVirtualRepo: Repositories to add not passed to API")
		}
		return response, nil
	})

	ClearStats()
	err = Provision(client, "", "", tc.reposToProvision, tc.repos, tc.users, tc.groups, tc.permissiondetails, tc.properties, true, tc.allowPatterns, LdapConfig{}, PropertiesConfig{}, tc.dryRun)
	if err != nil {
		t.Errorf("ProvisionCreateVirtualRepo: error = %v", err)
	}

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)

	output := buf.String()
	if !strings.Contains(output, "Created repos: 1") {
		t.Error("ProvisionCreateVirtualRepo: Expected one repo to be created. \"Created repos: 1\" not found in fmt.print")
	}
}

func TestProvisionUpdateVirtualRepo(t *testing.T) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	tc := testcase{
		[]Repo{
			{
				Name:         "test-repo",
				Description:  "Test repository",
				Rclass:       "virtual",
				PackageType:  "docker",
				Layout:       "",
				Repositories: []string{"test1", "test2"},
			},
		},
		[]ArtifactoryRepoDetailsResponse{
			{
				Key:           "test-repo",
				Description:   "Test repository",
				Rclass:        "virtual",
				PackageType:   "docker",
				RepoLayoutRef: "simple-default",
			},
		},
		[]ArtifactoryUser{},
		[]ArtifactoryGroup{},
		[]ArtifactoryPermissionDetails{},
		[]repoProperties{},
		false,
		false,
		"",
		"",
		`{"ok":true}`,
		`{"ok":true}`}

	var callCount int
	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		var response *http.Response
		if callCount == 0 {
			response = &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.HTTPResponseRepo)), Header: make(http.Header)}
		} else {
			t.Error("ProvisionUpdateVirtualRepo: Unexpected second call for permissions targets when handling virtual repo")
		}
		callCount++

		body, _ := io.ReadAll(req.Body)
		if !strings.Contains(string(body), "\"repositories\":[\"test1\",\"test2\"]") {
			t.Error("ProvisionUpdateVirtualRepo: Repositories to add not passed to API")
		}
		return response, nil
	})

	ClearStats()
	err = Provision(client, "", "", tc.reposToProvision, tc.repos, tc.users, tc.groups, tc.permissiondetails, tc.properties, true, tc.allowPatterns, LdapConfig{}, PropertiesConfig{}, tc.dryRun)
	if err != nil {
		t.Errorf("ProvisionUpdateVirtualRepo: error = %v", err)
	}

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)

	output := buf.String()

	fmt.Println("###")
	fmt.Println(tc.reposToProvision)
	fmt.Println(output)
	fmt.Println("###")

	if !strings.Contains(output, "Updated repos: 1") {
		t.Error("ProvisionUpdateVirtualRepo: Expected one repo to be updated. \"Updated repos: 1\" not found in fmt.print")
	}
}

func TestProvisionVirtualRepoMissingRepoList(t *testing.T) {
	tc := testcase{
		reposToProvision: []Repo{
			{
				Name:        "test-repo",
				Description: "Test repository",
				Rclass:      "virtual",
				PackageType: "docker",
				Layout:      "",
			},
		},
		repos: []ArtifactoryRepoDetailsResponse{
			{
				Key:           "test-repo",
				Description:   "Test repository",
				Rclass:        "virtual",
				PackageType:   "docker",
				RepoLayoutRef: "simple-default",
				Repositories:  []string{"test1", "test2"},
			},
		},
		users:             []ArtifactoryUser{},
		groups:            []ArtifactoryGroup{},
		permissiondetails: []ArtifactoryPermissionDetails{},
		allowPatterns:     false,
		dryRun:            false}

	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		t.Error("ProvisionVirtualRepoMissingRepoList: Unexpected call to API when nothing was changed.")
		return nil, nil
	})

	err := Provision(client, "", "", tc.reposToProvision, tc.repos, tc.users, tc.groups, tc.permissiondetails, tc.properties, true, tc.allowPatterns, LdapConfig{}, PropertiesConfig{}, tc.dryRun)
	if err != nil {
		t.Errorf("ProvisionVirtualRepoMissingRepoList: error = %v", err)
	}
}

func TestProvisionVirtualRepoMissingRepoListTriggerChange(t *testing.T) {
	tc := testcase{
		[]Repo{
			{
				Name:        "test-repo",
				Description: "Test repository",
				Rclass:      "virtual",
				PackageType: "docker",
				Layout:      "",
			},
		},
		[]ArtifactoryRepoDetailsResponse{
			{
				Key:           "test-repo",
				Description:   "Test repository - new", // trigger change
				Rclass:        "virtual",
				PackageType:   "docker",
				RepoLayoutRef: "simple-default",
				Repositories:  []string{"test1", "test2"},
			},
		},
		[]ArtifactoryUser{},
		[]ArtifactoryGroup{},
		[]ArtifactoryPermissionDetails{},
		[]repoProperties{},
		false,
		false,
		"",
		"",
		`{"ok":true}`,
		`{"ok":true}`}

	var callCount int
	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		var response *http.Response
		if callCount == 0 {
			response = &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.HTTPResponseRepo)), Header: make(http.Header)}
		} else {
			t.Error("ProvisionVirtualRepoMissingRepoListTriggerChange: Unexpected second call for permissions targets when handling virtual repo")
		}
		callCount++

		body, _ := io.ReadAll(req.Body)
		if strings.Contains(string(body), "\"repositories\":") {
			t.Error("ProvisionVirtualRepoMissingRepoListTriggerChange: List of repositories should not be passed when empty")
		}
		return response, nil
	})

	err := Provision(client, "", "", tc.reposToProvision, tc.repos, tc.users, tc.groups, tc.permissiondetails, tc.properties, true, tc.allowPatterns, LdapConfig{}, PropertiesConfig{}, tc.dryRun)
	if err != nil {
		t.Errorf("ProvisionVirtualRepoMissingRepoListTriggerChange: error = %v", err)
	}
}
