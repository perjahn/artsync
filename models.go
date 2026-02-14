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
}

type ArtifactoryPermissionDetails struct {
	Name       string                                `json:"name"`
	Resources  ArtifactoryPermissionDetailsResources `json:"resources"`
	JsonSource string                                `json:"-"`
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
}

type ArtifactoryGroups struct {
	Groups []ArtifactoryGroup `json:"groups"`
	Cursor string             `json:"cursor"`
}

type ArtifactoryGroup struct {
	GroupName string `json:"group_name"`
}

type ArtifactoryUserRequest struct {
	Username                 string `json:"username"`
	Email                    string `json:"email"`
	InternalPasswordDisabled bool   `json:"internal_password_disabled"`
}

type Repo struct {
	Name           string   `json:"name,omitempty"`
	Names          []string `json:"names,omitempty"`
	Description    string   `json:"description,omitempty"`
	Rclass         string   `json:"rclass,omitempty"`
	PackageType    string   `json:"packageType,omitempty"`
	Layout         string   `json:"layout,omitempty"`
	PermissionName string   `json:"permissionName,omitempty"`
	Read           []string `json:"read,omitempty"`
	Annotate       []string `json:"annotate,omitempty"`
	Write          []string `json:"write,omitempty"`
	Delete         []string `json:"delete,omitempty"`
	Manage         []string `json:"manage,omitempty"`
	Scan           []string `json:"scan,omitempty"`
	SourceFile     string   `json:"-"`
	SourceOffset   int      `json:"-"`
	SourceLine     int      `json:"-"`
}

type ArtifactoryLDAPGroupSettings struct {
	Name                 string `json:"name"`
	EnabledLdap          string `json:"enabled_ldap"`
	GroupBaseDn          string `json:"group_base_dn"`
	GroupNameAttribute   string `json:"group_name_attribute"`
	GroupMemberAttribute string `json:"group_member_attribute"`
	SubTree              bool   `json:"sub_tree"`
	ForceAttributeSearch bool   `json:"force_attribute_search"`
	Filter               string `json:"filter"`
	DescriptionAttribute string `json:"description_attribute"`
	Strategy             string `json:"strategy"`
}

type ArtifactoryGroupImport struct {
	ImportGroups      []ArtifactoryImportGroups    `json:"importGroups"`
	LdapGroupSettings ArtifactoryLDAPGroupSettings `json:"ldapGroupSettings"`
}

type ArtifactoryImportGroups struct {
	GroupName      string `json:"groupName"`
	Description    string `json:"description"`
	GroupDn        string `json:"groupDn"`
	RequiredUpdate string `json:"requiredUpdate"`
}

type ArtifactoryLDAPSettings struct {
	Key                      string                        `json:"key"`
	Enabled                  bool                          `json:"enabled"`
	LdapUrl                  string                        `json:"ldap_url"`
	Search                   ArtifactoryLDAPSettingsSearch `json:"search"`
	AutoCreateUser           bool                          `json:"auto_create_user"`
	EmailAttribute           string                        `json:"email_attribute"`
	LdapPositionProtection   bool                          `json:"ldap_position_protection"`
	AllowUserToAccessProfile bool                          `json:"allow_user_to_access_profile"`
	PagingSupportEnabled     bool                          `json:"paging_support_enabled"`
}

type ArtifactoryLDAPSettingsSearch struct {
	SearchFilter    string `json:"search_filter"`
	SearchBase      string `json:"search_base"`
	SearchSubtree   bool   `json:"search_subtree"`
	ManagerDn       string `json:"manager_dn"`
	ManagerPassword string `json:"manager_password"`
}

type ArtifactoryLogin struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LdapConfig struct {
	CreateUsers         bool                           `json:"-"`
	ImportGroups        bool                           `json:"-"`
	LdapUsername        string                         `json:"ldapusername"`
	LdapPassword        string                         `json:"ldappassword"`
	ArtifactoryUsername string                         `json:"artifactoryusername"`
	ArtifactoryPassword string                         `json:"artifactorypassword"`
	Ldapsettings        []ArtifactoryLDAPSettings      `json:"-"`
	Ldapgroupsettings   []ArtifactoryLDAPGroupSettings `json:"-"`
}
