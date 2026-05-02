package main

import (
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
		err := Provision(client, "", "", tc.reposToProvision, tc.repos, tc.users, tc.groups, tc.permissiondetails, false, tc.allowPatterns, LdapConfig{}, PropertiesConfig{}, tc.dryRun)
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

	err := Provision(client, "", "", tc.reposToProvision, tc.repos, tc.users, tc.groups, tc.permissiondetails, true, tc.allowPatterns, LdapConfig{}, PropertiesConfig{}, tc.dryRun)
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

	err := Provision(client, "", "", tc.reposToProvision, tc.repos, tc.users, tc.groups, tc.permissiondetails, true, tc.allowPatterns, LdapConfig{}, PropertiesConfig{}, tc.dryRun)
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

		fmt.Printf("FAIL1\n")

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

		fmt.Printf("FAIL2\n")

		return []*ldap.Entry{}, nil
	}

	queryldapImportGroupFn = queryldapCreateUserFn

	err := Provision(client, "", "", tc.reposToProvision, tc.repos, tc.users, tc.groups, tc.permissiondetails, false, tc.allowPatterns, ldapConfig, PropertiesConfig{}, tc.dryRun)
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

		fmt.Printf("FAIL3\n")

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

	err := Provision(client, "", "", tc.reposToProvision, tc.repos, tc.users, tc.groups, tc.permissiondetails, false, tc.allowPatterns, ldapConfig, PropertiesConfig{}, tc.dryRun)
	if err != nil {
		t.Errorf("ProvisionLdap: unexpected error = %v", err)
	}
}

// Test setRepoProperties function
func TestSetRepoProperties(t *testing.T) {
	repo := Repo{
		Name:        "test-repo",
		Description: "Test repository",
		Rclass:      "local",
		PackageType: "docker",
	}

	propertiesConfig := PropertiesConfig{
		Prefix: "mycorp.",
		Url:    "https://example.com/docs",
	}

	var capturedURL string
	var capturedMethod string

	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		capturedURL = req.URL.String()
		capturedMethod = req.Method

		// Verify authorization header is set
		authHeader := req.Header.Get("Authorization")
		if authHeader != "Bearer test-token" {
			t.Errorf("SetRepoProperties: expected Authorization header 'Bearer test-token', got '%s'", authHeader)
		}

		return &http.Response{
			StatusCode: 204,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})

	err := setRepoProperties(client, "https://artifactory.example.com", "test-token", repo, propertiesConfig, false)
	if err != nil {
		t.Errorf("SetRepoProperties: unexpected error = %v", err)
	}

	// Verify the request method
	if capturedMethod != "PUT" {
		t.Errorf("SetRepoProperties: expected method PUT, got %s", capturedMethod)
	}

	// Verify the URL contains expected properties
	if !strings.Contains(capturedURL, "https://artifactory.example.com/artifactory/api/storage/test-repo?properties=") {
		t.Errorf("SetRepoProperties: URL format incorrect: %s", capturedURL)
	}

	// Check for specific properties that should be included
	// Note: propertiesConfig.Url is added with capital U, but JSON marshaled repo fields are lowercase
	if !strings.Contains(capturedURL, "mycorp.Url=https://example.com/docs") {
		t.Errorf("SetRepoProperties: missing Url property in %s", capturedURL)
	}

	if !strings.Contains(capturedURL, "mycorp.name=test-repo") {
		t.Errorf("SetRepoProperties: missing name property in %s", capturedURL)
	}

	if !strings.Contains(capturedURL, "mycorp.description=Test repository") {
		t.Errorf("SetRepoProperties: missing description property in %s", capturedURL)
	}

	if !strings.Contains(capturedURL, "mycorp.packageType=docker") {
		t.Errorf("SetRepoProperties: missing packageType property in %s", capturedURL)
	}

	// Rclass is set to "local" so it SHOULD be included (it's not empty)
	if !strings.Contains(capturedURL, "mycorp.rclass=local") {
		t.Errorf("SetRepoProperties: missing rclass property in %s", capturedURL)
	}
}

// Test setRepoProperties with empty properties (should not fail, just return nil)
func TestSetRepoPropertiesEmpty(t *testing.T) {
	repo := Repo{
		Name: "test-repo",
		// All other fields are empty/zero
	}

	propertiesConfig := PropertiesConfig{
		Prefix: "mycorp.",
		Url:    "",
	}

	var httpCalled bool

	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		httpCalled = true
		return &http.Response{
			StatusCode: 204,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})

	err := setRepoProperties(client, "https://artifactory.example.com", "test-token", repo, propertiesConfig, false)
	if err != nil {
		t.Errorf("SetRepoPropertiesEmpty: unexpected error = %v", err)
	}

	// Even though most fields are empty, Name is set to "test-repo",
	// so there will be at least one property (mycorp.name=test-repo)
	// and an HTTP call will be made.
	if !httpCalled {
		t.Errorf("SetRepoPropertiesEmpty: HTTP call should be made when Name property exists")
	}
}
