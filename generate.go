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
	combineRepos bool,
	split bool,
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
		slices.Sort(repoToSave.Scan)

		if combineRepos {
			found := false
			for i := range reposToSave {
				if reposToSave[i].PackageType == repoToSave.PackageType &&
					reposToSave[i].Description == repoToSave.Description &&
					reposToSave[i].Rclass == repoToSave.Rclass &&
					reposToSave[i].Layout == repoToSave.Layout &&
					equalStringSlices(reposToSave[i].Read, repoToSave.Read) &&
					equalStringSlices(reposToSave[i].Annotate, repoToSave.Annotate) &&
					equalStringSlices(reposToSave[i].Write, repoToSave.Write) &&
					equalStringSlices(reposToSave[i].Delete, repoToSave.Delete) &&
					equalStringSlices(reposToSave[i].Manage, repoToSave.Manage) &&
					equalStringSlices(reposToSave[i].Scan, repoToSave.Scan) {
					found = true
					if reposToSave[i].Name != "" {
						reposToSave[i].Names = append(reposToSave[i].Names, reposToSave[i].Name, repoToSave.Name)
						reposToSave[i].Name = ""
					} else {
						reposToSave[i].Names = append(reposToSave[i].Names, repoToSave.Name)
					}
					fmt.Printf("'%s': Identical repo already generated (%s), compacting duplicate.\n", repo.Key, reposToSave[i].Names[0])
					break
				}
			}
			if found {
				continue
			}
		}

		reposToSave = append(reposToSave, repoToSave)
	}

	sort.Slice(reposToSave, func(i, j int) bool {
		if reposToSave[i].Name == "" && reposToSave[j].Name == "" {
			return reposToSave[i].Names[0] < reposToSave[j].Names[0]
		}
		if reposToSave[i].Name == "" {
			return true
		}
		if reposToSave[j].Name == "" {
			return false
		}
		return reposToSave[i].Name < reposToSave[j].Name
	})

	if !split {
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

	for _, repo := range reposToSave {
		var reponame string
		var filename string
		var data []byte
		if generateyaml {
			filename = repo.Name + ".yaml"
			reponame = repo.Name
			repo.Name = ""

			var err error
			data, err = yaml.Marshal(repo)
			if err != nil {
				return fmt.Errorf("error generating yaml: %w", err)
			}
		} else {
			filename = repo.Name + ".json"
			reponame = repo.Name
			repo.Name = ""

			var err error
			data, err = json.MarshalIndent(repo, "", "  ")
			if err != nil {
				return fmt.Errorf("error generating json: %w", err)
			}
		}

		fmt.Printf("Saving repo '%s' to file '%s'\n", reponame, filename)
		file, err := os.Create(filename)
		if err != nil {
			return fmt.Errorf("error creating file: %w", err)
		}
		defer file.Close()

		_, err = file.Write(data)
		if err != nil {
			return fmt.Errorf("error saving file: %w", err)
		}
	}

	return nil
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aCopy := slices.Clone(a)
	bCopy := slices.Clone(b)
	slices.Sort(aCopy)
	slices.Sort(bCopy)
	for i := range aCopy {
		if aCopy[i] != bCopy[i] {
			return false
		}
	}
	return true
}

func isClean(reponame string, permissiontargetname string, target ArtifactoryPermissionDetailsTarget) bool {
	include := target.IncludePatterns
	exclude := target.ExcludePatterns

	if !slices.Equal(include, []string{"**"}) || (len(exclude) != 0 && !slices.Equal(exclude, []string{""})) {
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
		if slices.Contains(rolePermissions, "SCAN") && !slices.Contains(repo.Scan, name) {
			repo.Scan = append(repo.Scan, name)
		}
	}
}
