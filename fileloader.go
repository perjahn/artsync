package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
)

func LoadRepoFiles(repofile []string) ([]Repo, error) {
	var allrepos []Repo

	for _, repofile := range repofile {
		repos, err := loadRepoFile(repofile)
		if err != nil {
			return nil, err
		}

		allrepos = append(allrepos, repos...)
	}

	allrepos = removeDups(allrepos)

	return allrepos, nil
}

func loadRepoFile(repofile string) ([]Repo, error) {
	data, err := os.ReadFile(repofile)
	if err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	var repos []Repo

	decoder := json.NewDecoder(strings.NewReader(string(data)))
	errjson := decoder.Decode(&repos)
	if errjson != nil {
		erryaml := yaml.Unmarshal(data, &repos)
		if erryaml != nil {
			return nil, fmt.Errorf("error parsing json/yaml file: %w %w", errjson, erryaml)
		} else {
			if len(repos) == 0 {
				fmt.Printf("Warning: Ignoring empty yaml file: '%s'\n", repofile)
				ignoredInvalidRepoFilesCount++
				return repos, nil
			}

			var node ast.Node
			if err := yaml.Unmarshal(data, &node); err != nil {
				return nil, fmt.Errorf("error parsing yaml file: %w", erryaml)
			}

			type position struct {
				offset int
				line   int
			}
			positions := []position{}
			if node != nil {
				t := node.GetToken()
				for {
					if t.Value == "-" && t.Position.IndentLevel == 0 {
						positions = append(positions, position{offset: t.Position.Offset, line: t.Position.Line})
					}
					t = t.Next
					if t == nil {
						break
					}
				}
			}
			if len(positions) != len(repos) {
				fmt.Printf("Warning: Ignoring repo file (%s): Number of repos (%d) does not match number of yaml objects (%d)\n",
					repofile, len(repos), len(positions))
				ignoredInvalidRepoFilesCount++
			}
			for i := range repos {
				repos[i].SourceFile = repofile
				repos[i].SourceOffset = positions[i].offset
				repos[i].SourceLine = positions[i].line
			}
		}
	} else {
		if len(repos) == 0 {
			fmt.Printf("Warning: Ignoring empty json file: '%s'\n", repofile)
			ignoredInvalidRepoFilesCount++
			return repos, nil
		}

		decoder = json.NewDecoder(strings.NewReader(string(data)))
		offsets := []int{}

		for {
			t, err := decoder.Token()
			if err != nil {
				break
			}
			if t == json.Delim('{') {
				offsets = append(offsets, int(decoder.InputOffset()-1))
			}
		}
		if len(offsets) != len(repos) {
			fmt.Printf("Warning: Ignoring repo file (%s): Number of repos (%d) does not match number of json objects (%d)\n",
				repofile, len(repos), len(offsets))
			ignoredInvalidRepoFilesCount++
		}
		for i := range repos {
			repos[i].SourceFile = repofile
			repos[i].SourceOffset = offsets[i]
			line := 1
			for j := range data {
				if data[j] == '\n' {
					line++
				}
				if offsets[i] == j {
					break
				}
			}
			repos[i].SourceLine = line
		}
	}

	repos = expandRepos(repos)

	return repos, nil
}

func expandRepos(repos []Repo) []Repo {
	var expandedRepos []Repo

	for i := range repos {
		if repos[i].Name == "" && len(repos[i].Names) == 0 {
			b := filepath.Base(repos[i].SourceFile)
			repos[i].Name = strings.TrimSuffix(b, filepath.Ext(b))
		}
		if repos[i].Name != "" && len(repos[i].Names) > 0 {
			fmt.Printf("Ignoring repo: Repo must not have both a name (%s) and names (%s)\n", repos[i].Name, strings.Join(repos[i].Names, ", "))
			continue
		}

		if repos[i].Name != "" {
			expandedRepos = append(expandedRepos, repos[i])
		} else {
			names := repos[i].Names
			repos[i].Names = []string{}
			for _, name := range names {
				fmt.Printf("Expanding: '%s' -> '%s'\n", strings.Join(names, "', '"), name)
				repos[i].Name = name
				expandedRepos = append(expandedRepos, repos[i])
			}
		}
	}

	return expandedRepos
}

func removeDups(repos []Repo) []Repo {
	type jsonobject struct {
		Index      int
		SourceFile string
		SourceLine int
	}

	reposToDelete := make(map[string][]jsonobject)

	for i := range repos {
		name := repos[i].Name
		for j := i + 1; j < len(repos); j++ {
			if name == repos[j].Name {
				indices, ok := reposToDelete[name]
				if !ok {
					jo := jsonobject{Index: i, SourceFile: repos[i].SourceFile, SourceLine: repos[i].SourceLine}
					reposToDelete[name] = []jsonobject{jo}
				}
				found := false
				for _, index := range indices {
					if index.Index == j {
						found = true
						break
					}
				}
				if !found {
					jo := jsonobject{Index: j, SourceFile: repos[j].SourceFile, SourceLine: repos[j].SourceLine}
					reposToDelete[name] = append(reposToDelete[name], jo)
				}
			}
		}
	}

	keys := make([]string, 0, len(reposToDelete))
	for key := range reposToDelete {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		positions := make([]string, len(reposToDelete[key]))
		for i, jo := range reposToDelete[key] {
			positions[i] = fmt.Sprintf("%s:%d", jo.SourceFile, jo.SourceLine)
		}
		fmt.Printf("Warning: Ignoring %d repos due to duplicate name. Name: '%s', objects (file:line): %s\n", len(reposToDelete[key]), key, strings.Join(positions, ", "))
	}

	repoIndicesToDelete := []int{}
	sort.Ints(repoIndicesToDelete)
	for _, value := range reposToDelete {
		for _, jo := range value {
			repoIndicesToDelete = append(repoIndicesToDelete, jo.Index)
		}
	}
	sort.Ints(repoIndicesToDelete)

	ignoredDuplicated_RepoCount = len(repoIndicesToDelete)

	for i := len(repoIndicesToDelete) - 1; i >= 0; i-- {
		repos = slices.Delete(repos, repoIndicesToDelete[i], repoIndicesToDelete[i]+1)
	}

	return repos
}
