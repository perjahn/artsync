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
	allowRenamedPermissions bool,
	combineRepos bool,
	split bool,
	repofile string,
	generatejson bool) error {

	var reposToSave []Repo

	for _, repo := range repos {
		if onlyGenerateMatchingRepos && !includeOnlyMatchingRepos(repo.Key, permissiondetails) {
			continue
		}

		var permissionName string
		if allowRenamedPermissions {
			var filter bool
			permissionName, filter = includeOnlyMatchingAndRenamedRepos(repo.Key, permissiondetails)
			if !filter {
				continue
			}
		}

		if onlyGenerateCleanRepos && !includeOnlyCleanRepos(repo.Key, permissiondetails, useAllPermissionTargetsAsSource) {
			continue
		}

		repoToSave := Repo{
			Name:           repo.Key,
			Description:    repo.Description,
			Rclass:         repo.Rclass,
			PackageType:    repo.PackageType,
			Layout:         repo.RepoLayoutRef,
			PermissionName: permissionName,
		}

		if allowRenamedPermissions {
			if permissionName == "" {
				for _, permission := range permissiondetails {
					if permission.Name == repo.Key {
						addPermissionsToRepo(&repoToSave, permission.Resources.Artifact.Actions.Users)
						addPermissionsToRepo(&repoToSave, permission.Resources.Artifact.Actions.Groups)
					}
				}
			} else {
				for _, permission := range permissiondetails {
					if permission.Name == permissionName {
						addPermissionsToRepo(&repoToSave, permission.Resources.Artifact.Actions.Users)
						addPermissionsToRepo(&repoToSave, permission.Resources.Artifact.Actions.Groups)
					}
				}
			}
		} else if useAllPermissionTargetsAsSource {
			for _, permission := range permissiondetails {
				for reponame := range permission.Resources.Artifact.Targets {
					if reponame == repo.Key {
						addPermissionsToRepo(&repoToSave, permission.Resources.Artifact.Actions.Users)
						addPermissionsToRepo(&repoToSave, permission.Resources.Artifact.Actions.Groups)
					}
				}
			}
		} else {
			for _, permission := range permissiondetails {
				if permission.Name == repo.Key {
					addPermissionsToRepo(&repoToSave, permission.Resources.Artifact.Actions.Users)
					addPermissionsToRepo(&repoToSave, permission.Resources.Artifact.Actions.Groups)
				}
			}
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
					reposToSave[i].PermissionName == repoToSave.PermissionName &&
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

	if split {
		return saveSplitRepos(reposToSave, repofile, generatejson)
	}
	return saveCombinedRepos(reposToSave, repofile, generatejson)
}

func includeOnlyMatchingRepos(repokey string, permissiondetails []ArtifactoryPermissionDetails) bool {
	for _, permission := range permissiondetails {
		if permission.Name == repokey {
			if len(permission.Resources.Artifact.Targets) < 1 {
				fmt.Printf("Ignoring repo: '%s'. The permission target named the same as the repo, isn't connected to any repo.\n", repokey)
				return false
			}
			if len(permission.Resources.Artifact.Targets) > 1 {
				fmt.Printf("Ignoring repo: '%s'. The permission target named the same as the repo, is used by multiple repos.\n", repokey)
				return false
			}

			_, exists := permission.Resources.Artifact.Targets[repokey]
			if exists {
				return true
			} else {
				fmt.Printf("Ignoring repo: '%s'. The permission target named the same as the repo, isn't connected to the matching repo.\n", repokey)
				return false
			}
		}
	}
	return false
}

func includeOnlyMatchingAndRenamedRepos(repokey string, permissiondetails []ArtifactoryPermissionDetails) (string, bool) {
	for _, permission := range permissiondetails {
		if permission.Name == repokey {
			if len(permission.Resources.Artifact.Targets) < 1 {
				fmt.Printf("Ignoring repo: '%s'. The permission target named the same as the repo, isn't connected to any repo.\n", repokey)
				return "", false
			}
			if len(permission.Resources.Artifact.Targets) > 1 {
				fmt.Printf("Ignoring repo: '%s'. The permission target named the same as the repo, is used by multiple repos.\n", repokey)
				return "", false
			}

			_, exists := permission.Resources.Artifact.Targets[repokey]
			if exists {
				return "", true
			} else {
				fmt.Printf("Ignoring repo: '%s'. The permission target named the same as the repo, isn't connected to the matching repo.\n", repokey)
				return "", false
			}
		}
	}

	var permissionNames []string
	for _, permission := range permissiondetails {
		_, exists := permission.Resources.Artifact.Targets[repokey]
		if exists && len(permission.Resources.Artifact.Targets) == 1 {
			permissionNames = append(permissionNames, permission.Name)
		}
	}

	if len(permissionNames) < 1 {
		fmt.Printf("Ignoring repo: '%s'. No non-shared connected permission target.\n", repokey)
		return "", false
	}
	if len(permissionNames) > 1 {
		fmt.Printf("Ignoring repo: '%s'. Too many permission targets (%d) connected to the repo: %q.\n", repokey, len(permissionNames), permissionNames)
		return "", false
	}

	return permissionNames[0], true
}

func includeOnlyCleanRepos(repokey string, permissiondetails []ArtifactoryPermissionDetails, useAllPermissionTargetsAsSource bool) bool {
	if useAllPermissionTargetsAsSource {
		for _, permission := range permissiondetails {
			for reponame := range permission.Resources.Artifact.Targets {
				if reponame == repokey {
					if !isClean(repokey, permission.Name, permission.Resources.Artifact.Targets[repokey]) {
						return false
					}
				}
			}
		}
	} else {
		for _, permission := range permissiondetails {
			if permission.Name == repokey {
				if !isClean(repokey, permission.Name, permission.Resources.Artifact.Targets[repokey]) {
					return false
				}
			}
		}
	}

	return true
}

func saveCombinedRepos(reposToSave []Repo, repofile string, generatejson bool) error {
	var data []byte
	var err error
	if generatejson {
		data, err = json.MarshalIndent(reposToSave, "", "  ")
		if err != nil {
			return fmt.Errorf("error generating json: %w", err)
		}
	} else {
		data, err = yaml.Marshal(reposToSave)
		if err != nil {
			return fmt.Errorf("error generating yaml: %w", err)
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

func saveSplitRepos(reposToSave []Repo, folder string, generatejson bool) error {
	if _, err := os.Stat(folder); os.IsNotExist(err) {
		fmt.Printf("Creating folder: '%s'\n", folder)
		err = os.Mkdir(folder, 0755)
		if err != nil {
			return fmt.Errorf("error creating folder: '%s' %w", folder, err)
		}
	}

	for _, repo := range reposToSave {
		var filename, reponame string
		var data []byte
		var err error
		if generatejson {
			filename = fmt.Sprintf("%s/%s.json", folder, repo.Name)
			reponame = repo.Name
			repo.Name = ""
			data, err = json.MarshalIndent(repo, "", "  ")
			if err != nil {
				return fmt.Errorf("error generating json: %w", err)
			}
		} else {
			filename = fmt.Sprintf("%s/%s.yaml", folder, repo.Name)
			reponame = repo.Name
			repo.Name = ""
			data, err = yaml.Marshal(repo)
			if err != nil {
				return fmt.Errorf("error generating yaml: %w", err)
			}
			if len(data) == 3 || string(data) == "{}\n" {
				data = []byte{}
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
