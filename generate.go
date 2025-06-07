package main

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

func Generate(
	repos []ArtifactoryRepoDetailsResponse,
	permissiondetails []ArtifactoryPermissionDetails,
	useAllPermissionTargetsAsSource bool,
	onlyGenerateMatchingRepos bool,
	onlyGenerateCleanRepos bool,
	repofile string,
	generateyaml bool) error {

	var reposToSave []Repo

	for _, repo := range repos {
		if onlyGenerateMatchingRepos {
			found := false
			for _, permission := range permissiondetails {
				if permission.Name == repo.Key {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		repoToSave := Repo{
			Name:        repo.Key,
			Description: repo.Description,
			Rclass:      repo.Rclass,
			PackageType: repo.PackageType,
			Layout:      repo.RepoLayoutRef,
		}

		if strings.EqualFold(repo.Rclass, "local") {
			repoToSave.Rclass = ""
		}
		if strings.EqualFold(repo.PackageType, "generic") {
			repoToSave.PackageType = ""
		}
		if strings.EqualFold(repo.RepoLayoutRef, "simple-default") {
			repoToSave.Layout = ""
		}

		clean := true
		if useAllPermissionTargetsAsSource {
			for _, permission := range permissiondetails {
				for reponame := range permission.Resources.Artifact.Targets {
					if reponame == repo.Key {
						if onlyGenerateCleanRepos && !isClean(repo.Key, permission.Name, permission.Resources.Artifact.Targets[repo.Key]) {
							clean = false
						}

						addPermissionsToRepo(&repoToSave, permission.Resources.Artifact.Actions.Users)
						addPermissionsToRepo(&repoToSave, permission.Resources.Artifact.Actions.Groups)
					}
				}
			}
		} else {
			for _, permission := range permissiondetails {
				if permission.Name == repo.Key {
					if onlyGenerateCleanRepos && !isClean(repo.Key, permission.Name, permission.Resources.Artifact.Targets[repo.Key]) {
						clean = false
					}

					addPermissionsToRepo(&repoToSave, permission.Resources.Artifact.Actions.Users)
					addPermissionsToRepo(&repoToSave, permission.Resources.Artifact.Actions.Groups)
				}
			}
		}
		if !clean {
			continue
		}

		slices.Sort(repoToSave.Read)
		slices.Sort(repoToSave.Annotate)
		slices.Sort(repoToSave.Write)
		slices.Sort(repoToSave.Delete)
		slices.Sort(repoToSave.Manage)

		reposToSave = append(reposToSave, repoToSave)
	}

	sort.Slice(reposToSave, func(i, j int) bool {
		return reposToSave[i].Name < reposToSave[j].Name
	})

	var data []byte
	if generateyaml {
		var err error
		data, err = yaml.Marshal(reposToSave)
		if err != nil {
			return fmt.Errorf("error generating yaml: %w", err)
		}
	} else {
		var err error
		data, err = json.MarshalIndent(reposToSave, "", "  ")
		if err != nil {
			return fmt.Errorf("error generating json: %w", err)
		}
	}

	file, err := os.Create(repofile)
	if err != nil {
		return fmt.Errorf("error creating file: %w", err)
	}
	defer file.Close()

	_, err = file.Write(data)
	if err != nil {
		return fmt.Errorf("error saving file: %w", err)
	}

	return nil
}

func isClean(reponame string, permissiontargetname string, target ArtifactoryPermissionDetailsTarget) bool {
	include := target.IncludePatterns
	exclude := target.ExcludePatterns

	if !slices.Equal(include, []string{"**"}) || (len(exclude) != 0 && !slices.Equal(exclude, []string{})) {
		fmt.Printf("'%s': Ignoring repo due to its permission target having non-default include/exclude patterns: permission target: '%s', include: '%s', exclude: '%s' %d\n",
			reponame, permissiontargetname, include, exclude, len(exclude))
		return false
	}

	return true
}

func addPermissionsToRepo(repo *Repo, permissions map[string][]string) {
	for name, rolePermissions := range permissions {
		if slices.Contains(rolePermissions, "READ") && !slices.Contains(repo.Read, name) {
			repo.Read = append(repo.Read, name)
		}
		if slices.Contains(rolePermissions, "ANNOTATE") && !slices.Contains(repo.Annotate, name) {
			repo.Annotate = append(repo.Annotate, name)
		}
		if slices.Contains(rolePermissions, "WRITE") && !slices.Contains(repo.Write, name) {
			repo.Write = append(repo.Write, name)
		}
		if slices.Contains(rolePermissions, "DELETE") && !slices.Contains(repo.Delete, name) {
			repo.Delete = append(repo.Delete, name)
		}
		if slices.Contains(rolePermissions, "MANAGE") && !slices.Contains(repo.Manage, name) {
			repo.Manage = append(repo.Manage, name)
		}
	}
}
