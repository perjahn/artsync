package main

type ArtifactoryRepoResponse struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Type        string `json:"type"`
	PackageType string `json:"packageType"`
}

type ArtifactoryRepoDetailsResponse struct {
	Key           string `json:"key"`
	Description   string `json:"description"`
	Rclass        string `json:"rclass"`
	PackageType   string `json:"packageType"`
	RepoLayoutRef string `json:"repoLayoutRef"`
}

type ArtifactoryRepoRequest struct {
	Key           string `json:"key,omitempty"`
	Description   string `json:"description,omitempty"`
	Rclass        string `json:"rclass"`
	PackageType   string `json:"packageType,omitempty"`
	RepoLayoutRef string `json:"repoLayoutRef,omitempty"`
}

type ArtifactoryPermissions struct {
	Permissions []ArtifactoryPermission `json:"permissions"`
	Cursor      string                  `json:"cursor"`
}

type ArtifactoryPermission struct {
	Name string `json:"name"`
	Uri  string `json:"uri"`
}

type ArtifactoryPermissionDetails struct {
	Name      string                                `json:"name"`
	Resources ArtifactoryPermissionDetailsResources `json:"resources"`
}

type ArtifactoryPermissionDetailsResources struct {
	Artifact ArtifactoryPermissionDetailsArtifact `json:"artifact"`
}

type ArtifactoryPermissionDetailsArtifact struct {
	Actions ArtifactoryPermissionDetailsActions           `json:"actions"`
	Targets map[string]ArtifactoryPermissionDetailsTarget `json:"targets"`
}

type ArtifactoryPermissionDetailsActions struct {
	Users  map[string][]string `json:"users"`
	Groups map[string][]string `json:"groups"`
}

type ArtifactoryPermissionDetailsTarget struct {
	IncludePatterns []string `json:"include_patterns"`
	ExcludePatterns []string `json:"exclude_patterns"`
}

type ArtifactoryUsers struct {
	Users  []ArtifactoryUser `json:"users"`
	Cursor string            `json:"cursor"`
}

type ArtifactoryUser struct {
	Username string `json:"username"`
	Uri      string `json:"uri"`
	Realm    string `json:"realm"`
	Status   string `json:"status"`
}

type ArtifactoryGroups struct {
	Groups []ArtifactoryGroup `json:"groups"`
	Cursor string             `json:"cursor"`
}

type ArtifactoryGroup struct {
	GroupName string `json:"group_name"`
	Uri       string `json:"uri"`
}

type Repo struct {
	Name         string   `json:"name,omitempty"`
	Names        []string `json:"names,omitempty"`
	Description  string   `json:"description,omitempty"`
	Rclass       string   `json:"rclass,omitempty"`
	PackageType  string   `json:"packageType,omitempty"`
	Layout       string   `json:"layout,omitempty"`
	Read         []string `json:"read,omitempty"`
	Annotate     []string `json:"annotate,omitempty"`
	Write        []string `json:"write,omitempty"`
	Delete       []string `json:"delete,omitempty"`
	Manage       []string `json:"manage,omitempty"`
	Scan         []string `json:"scan,omitempty"`
	SourceFile   string   `json:"-"`
	SourceOffset int      `json:"-"`
	SourceLine   int      `json:"-"`
}
