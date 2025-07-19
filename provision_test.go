package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
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

func TestProvision(t *testing.T) {
	tests := []struct {
		reposToProvision       []Repo
		repos                  []ArtifactoryRepoDetailsResponse
		users                  []ArtifactoryUser
		groups                 []ArtifactoryGroup
		permissiondetails      []ArtifactoryPermissionDetails
		allowPatterns          bool
		dryRun                 bool
		HTTPResponseRepo       string
		HTTPResponsePermission string
	}{
		// Minimal
		{[]Repo{}, []ArtifactoryRepoDetailsResponse{}, []ArtifactoryUser{}, []ArtifactoryGroup{}, []ArtifactoryPermissionDetails{}, false, false, "", ""},
		// allowPatterns
		{[]Repo{}, []ArtifactoryRepoDetailsResponse{}, []ArtifactoryUser{}, []ArtifactoryGroup{}, []ArtifactoryPermissionDetails{}, true, false, "", ""},
		// dryRun
		{[]Repo{}, []ArtifactoryRepoDetailsResponse{}, []ArtifactoryUser{}, []ArtifactoryGroup{}, []ArtifactoryPermissionDetails{}, false, true, "", ""},
		// Provision existing repo/permission
		{
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
			[]ArtifactoryUser{
				{
					Username: "test-user",
				},
			},
			[]ArtifactoryGroup{
				{
					GroupName: "test-group",
				}},
			[]ArtifactoryPermissionDetails{
				{
					Name: "test-repo",
					Resources: ArtifactoryPermissionDetailsResources{
						Artifact: ArtifactoryPermissionDetailsArtifact{
							Actions: ArtifactoryPermissionDetailsActions{
								Users: map[string][]string{
									"test-user": {"READ", "WRITE", "OTHER"},
								},
								Groups: map[string][]string{
									"test-group": {"READ"},
								},
							},
							Targets: map[string]ArtifactoryPermissionDetailsTarget{
								"test-repo": {
									IncludePatterns: []string{"**"},
									ExcludePatterns: []string{},
								},
							},
						},
					},
				},
			},
			false,
			false,
			`{"ok":true}`,
			`{"ok":true}`},
	}
	for i, tc := range tests {
		var client *http.Client
		var callCount int
		if tc.HTTPResponseRepo != "" && tc.HTTPResponsePermission != "" {
			client = mockHTTPClient(func(req *http.Request) (*http.Response, error) {
				var reponse *http.Response
				if callCount == 0 {
					reponse = &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(strings.NewReader(tc.HTTPResponseRepo)),
						Header:     make(http.Header),
					}
				} else {
					reponse = &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(strings.NewReader(tc.HTTPResponsePermission)),
						Header:     make(http.Header),
					}
				}

				callCount++
				return reponse, nil
			})
		}
		err := Provision(tc.reposToProvision, tc.repos, tc.users, tc.groups, tc.permissiondetails, client, "", "", tc.allowPatterns, tc.dryRun)
		if err != nil {
			t.Errorf("Provision (%d/%d): error = %v",
				i, len(tests), err)
		}
	}
}
