package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
)

func LoadRepoFiles(repofiles []string, provisionEmpty bool) []Repo {
	var allrepos []Repo

	for _, repofile := range repofiles {
		repos, err := loadRepoFile(repofile, provisionEmpty)
		if err != nil {
			fmt.Printf("'%s': Warning: Ignoring invalid repo file: %v\n", repofile, err)
			ignoredInvalidRepoFilesCount++
			continue
		}

		allrepos = append(allrepos, repos...)
	}

	allrepos = removeDups(allrepos)

	return allrepos
}

func loadRepoFile(repofile string, provisionEmpty bool) ([]Repo, error) {
	data, err := os.ReadFile(repofile)
	if err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	var repos []Repo
	var errjson, erryaml error

	repos, errjson = tryParseJsonRepos(data, repofile, provisionEmpty)
	if len(repos) == 0 || errjson != nil {
		repos, erryaml = tryParseYamlRepos(data, repofile, provisionEmpty)
		if erryaml != nil {
			return nil, fmt.Errorf("unparsable json/yaml file")
		}
	}

	repos = expandRepos(repos)

	if len(repos) == 0 {
		return nil, fmt.Errorf("empty json/yaml file")
	}

	return repos, nil
}

func tryParseJsonRepos(data []byte, repofile string, provisionEmpty bool) ([]Repo, error) {
	var repos []Repo

	decoder := json.NewDecoder(strings.NewReader(string(data)))
	errjson := decoder.Decode(&repos)
	if errjson != nil {
		var onerepo Repo
		decoder1 := json.NewDecoder(strings.NewReader(string(data)))
		errjson1 := decoder1.Decode(&onerepo)
		if errjson1 != nil {
			return nil, fmt.Errorf("error parsing file as json: %w", errors.Join(errjson, errjson1))
		}

		onerepo.SourceFile = repofile
		onerepo.SourceOffset = 1
		onerepo.SourceLine = 1

		return []Repo{onerepo}, nil
	}

	if len(repos) == 0 {
		if provisionEmpty {
			var onerepo Repo
			b := filepath.Base(repofile)
			onerepo.Name = strings.TrimSuffix(b, filepath.Ext(b))

			onerepo.SourceFile = repofile
			onerepo.SourceOffset = 1
			onerepo.SourceLine = 1

			return []Repo{onerepo}, nil
		} else {
			return repos, nil
		}
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
		return nil, fmt.Errorf("error number of repos (%d) does not match number of json objects (%d)",
			len(repos), len(offsets))
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

	return repos, nil
}

func tryParseYamlRepos(data []byte, repofile string, provisionEmpty bool) ([]Repo, error) {
	var repos []Repo

	erryaml := yaml.Unmarshal(data, &repos)
	if erryaml != nil {
		var onerepo Repo
		erryaml1 := yaml.Unmarshal(data, &onerepo)
		if erryaml1 != nil {
			return nil, fmt.Errorf("error parsing file as yaml1: %w", errors.Join(erryaml, erryaml1))
		}

		onerepo.SourceFile = repofile
		onerepo.SourceOffset = 1
		onerepo.SourceLine = 1

		return []Repo{onerepo}, nil
	}

	if len(repos) == 0 {
		if provisionEmpty {
			var onerepo Repo
			b := filepath.Base(repofile)
			onerepo.Name = strings.TrimSuffix(b, filepath.Ext(b))

			onerepo.SourceFile = repofile
			onerepo.SourceOffset = 1
			onerepo.SourceLine = 1

			return []Repo{onerepo}, nil
		} else {
			return repos, nil
		}
	}

	var node ast.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, fmt.Errorf("error parsing file as yaml2: %w", erryaml)
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
		return nil, fmt.Errorf("error number of repos (%d) does not match number of yaml objects (%d)",
			len(repos), len(positions))
	}
	for i := range repos {
		repos[i].SourceFile = repofile
		repos[i].SourceOffset = positions[i].offset
		repos[i].SourceLine = positions[i].line
	}

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
			fmt.Printf("Warning: Ignoring repo: Repo must not have both a name (%s) and names (%s)\n",
				repos[i].Name, strings.Join(repos[i].Names, ", "))
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
