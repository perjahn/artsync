package main

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"

	"github.com/goccy/go-yaml"
)

func Generate(repos []ArtifactoryRepoResponse, permissiondetails []ArtifactoryPermissionDetails, useAllPermissionTargetsAsSource bool, repofile string, generateyaml bool) error {
	reposToSave := make([]Repo, len(repos))

	for i, repo := range repos {
		reposToSave[i] = Repo{
			Name:        repo.Key,
			Description: repo.Description,
			Rclass:      repo.Type,
			PackageType: repo.PackageType,
		}
		if repo.Type == "LOCAL" {
			reposToSave[i].Rclass = ""
		}
		if repo.PackageType == "Generic" {
			reposToSave[i].PackageType = ""
		}

		if useAllPermissionTargetsAsSource {
			for _, permission := range permissiondetails {
				for reponame := range permission.Resources.Artifact.Targets {
					if reponame == repo.Key {
						addPermissionsToRepo(reposToSave, i, permission.Resources.Artifact.Actions.Users)
						addPermissionsToRepo(reposToSave, i, permission.Resources.Artifact.Actions.Groups)
					}
				}
			}
		} else {
			for _, permission := range permissiondetails {
				if permission.Name == repo.Key {
					addPermissionsToRepo(reposToSave, i, permission.Resources.Artifact.Actions.Users)
					addPermissionsToRepo(reposToSave, i, permission.Resources.Artifact.Actions.Groups)
				}
			}
		}

		slices.Sort(reposToSave[i].Read)
		slices.Sort(reposToSave[i].Write)
		slices.Sort(reposToSave[i].Annotate)
		slices.Sort(reposToSave[i].Delete)
		slices.Sort(reposToSave[i].Manage)
	}

	sort.Slice(reposToSave, func(i, j int) bool {
		return reposToSave[i].Name < reposToSave[j].Name
	})

	if generateyaml {
		data, err := yaml.Marshal(reposToSave)
		if err != nil {
			return fmt.Errorf("error generating yaml: %w", err)
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
	} else {
		json, err := json.MarshalIndent(reposToSave, "", "  ")
		if err != nil {
			return fmt.Errorf("error generating json: %w", err)
		}

		file, err := os.Create(repofile)
		if err != nil {
			return fmt.Errorf("error creating file: %w", err)
		}
		defer file.Close()

		_, err = file.Write(json)
		if err != nil {
			return fmt.Errorf("error saving file: %w", err)
		}
	}

	return nil
}

func addPermissionsToRepo(reposToSave []Repo, i int, permissions map[string][]string) {
	for name, rolePermissions := range permissions {
		if slices.Contains(rolePermissions, "READ") && !slices.Contains(reposToSave[i].Read, name) {
			reposToSave[i].Read = append(reposToSave[i].Read, name)
		}
		if slices.Contains(rolePermissions, "WRITE") && !slices.Contains(reposToSave[i].Write, name) {
			reposToSave[i].Write = append(reposToSave[i].Write, name)
		}
		if slices.Contains(rolePermissions, "ANNOTATE") && !slices.Contains(reposToSave[i].Annotate, name) {
			reposToSave[i].Annotate = append(reposToSave[i].Annotate, name)
		}
		if slices.Contains(rolePermissions, "DELETE") && !slices.Contains(reposToSave[i].Delete, name) {
			reposToSave[i].Delete = append(reposToSave[i].Delete, name)
		}
		if slices.Contains(rolePermissions, "MANAGE") && !slices.Contains(reposToSave[i].Manage, name) {
			reposToSave[i].Manage = append(reposToSave[i].Manage, name)
		}
	}
}
