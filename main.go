package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func main() {
	useAllPermissionTargetsAsSourceFlag := flag.Bool("a", false, "Use all permission targets as source, when generating.")
	combineReposFlag := flag.Bool("c", false, "Combine identical repos, when generating.")
	dryRunFlag := flag.Bool("d", false, "Enable dry run mode (read-only, no changes will be made).")
	provisionEmpty := flag.Bool("e", false, "Provision empty files.")
	showDiffFlag := flag.Bool("f", false, "Show json diff, when applying permission targets.")
	generateFlag := flag.Bool("g", false, "Generate repo file.")
	ignoreCertFlag := flag.Bool("k", false, "Ignore https cert validation errors.")
	importGroupsFlag := flag.Bool("l", false, "Import missing groups from ldap.")
	onlyGenerateMatchingReposFlag := flag.Bool("m", false, "Only generate repos that has a matching named permission target.")
	allowpatternsFlag := flag.Bool("p", false, "Allow permission targets include/exclude patterns, when provisioning. This will delete all custom filters.")
	onlyGenerateCleanReposFlag := flag.Bool("q", false, "Only generate repos whose permission targets are default, i.e. without any include/exclude patterns.")
	allowRenamedPermissionsFlag := flag.Bool("r", false, "Allow non-conventional permission target names, when generating.")
	splitFlag := flag.Bool("s", false, "Split into one file for each repo, when generating. Uses specified repofile as subfolder. Ignores combine flag.")
	createUsersFlag := flag.Bool("u", false, "Create missing users, from ldap.")
	overwriteFlag := flag.Bool("w", false, "Allow overwriting of existing repo file, when generating.")
	generateyamlFlag := flag.Bool("y", false, "Generate output in yaml format.")
	flag.Parse()

	useAllPermissionTargetsAsSource := getFlagEnv(*useAllPermissionTargetsAsSourceFlag, "ARTSYNC_USE_ALL_PERMISSIONS")
	combineRepos := getFlagEnv(*combineReposFlag, "ARTSYNC_COMBINE_REPOS")
	dryRun := getFlagEnv(*dryRunFlag, "ARTSYNC_DRYRUN")
	showDiff := getFlagEnv(*showDiffFlag, "ARTSYNC_SHOW_DIFF")
	generate := getFlagEnv(*generateFlag, "ARTSYNC_GENERATE")
	ignoreCert := getFlagEnv(*ignoreCertFlag, "ARTSYNC_IGNORE_CERT")
	importGroups := getFlagEnv(*importGroupsFlag, "ARTSYNC_IMPORT_LDAP_GROUPS_FILENAME")
	onlyGenerateMatchingRepos := getFlagEnv(*onlyGenerateMatchingReposFlag, "ARTSYNC_ONLY_GENERATE_MATCHING")
	allowpatterns := getFlagEnv(*allowpatternsFlag, "ARTSYNC_ALLOW_PATTERNS")
	onlyGenerateCleanRepos := getFlagEnv(*onlyGenerateCleanReposFlag, "ARTSYNC_ONLY_GENERATE_CLEAN_REPOS")
	allowRenamedPermissions := getFlagEnv(*allowRenamedPermissionsFlag, "ARTSYNC_ALLOW_RENAMED_PERMISSIONS")
	split := getFlagEnv(*splitFlag, "ARTSYNC_SPLIT")
	createUsers := getFlagEnv(*createUsersFlag, "ARTSYNC_CREATE_USERS")
	overwrite := getFlagEnv(*overwriteFlag, "ARTSYNC_OVERWRITE")
	generateyaml := getFlagEnv(*generateyamlFlag, "ARTSYNC_GENERATE_YAML")

	args := flag.Args()
	if len(args) < 3 || args[0] == "" || args[1] == "" || args[2] == "" {
		usage()
		os.Exit(1)
	}

	baseurl := getBaseURL(args[0])
	token := getToken(args[1])
	repofiles := getRepoFiles(args[2:])

	if generate && len(repofiles) > 1 {
		fmt.Println("Error: Only one repo file is allowed when using -g flag.")
		os.Exit(1)
	}

	if !generate && useAllPermissionTargetsAsSource {
		fmt.Println("Error: -a flag can only be used together with -g flag.")
		os.Exit(1)
	}
	if !generate && combineRepos {
		fmt.Println("Error: -c flag can only be used together with -g flag.")
		os.Exit(1)
	}
	if !generate && onlyGenerateMatchingRepos {
		fmt.Println("Error: -m flag can only be used together with -g flag.")
		os.Exit(1)
	}
	if !generate && onlyGenerateCleanRepos {
		fmt.Println("Error: -q flag can only be used together with -g flag.")
		os.Exit(1)
	}
	if !generate && allowRenamedPermissions {
		fmt.Println("Error: -r flag can only be used together with -g flag.")
		os.Exit(1)
	}
	if !generate && overwrite {
		fmt.Println("Error: -w flag can only be used together with -g flag.")
		os.Exit(1)
	}
	if !generate && split {
		fmt.Println("Error: -s flag can only be used together with -g flag.")
		os.Exit(1)
	}
	if !generate && generateyaml {
		fmt.Println("Error: -y flag can only be used together with -g flag.")
		os.Exit(1)
	}

	if !generate {
		success := true
		for _, repofile := range repofiles {
			if _, err := os.Stat(repofile); os.IsNotExist(err) {
				fmt.Printf("Error: Repo file not found: '%s'\n", repofile)
				success = false
			}
		}
		if !success {
			os.Exit(1)
		}
	}

	if generate && !overwrite {
		if _, err := os.Stat(repofiles[0]); err == nil {
			fmt.Printf("Error: File already exists, will not overwrite: '%s'\n", repofiles[0])
			os.Exit(1)
		}
	}

	client := &http.Client{}
	if ignoreCert {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	var reposToProvision []Repo

	if !generate {
		reposToProvision = LoadRepoFiles(repofiles, *provisionEmpty)
		if len(reposToProvision) == 0 {
			fmt.Println("Error: No valid repos to provision found in the provided repo files.")
			os.Exit(1)
		}
	}

	retrieveldapsettings := false
	if createUsers || importGroups {
		retrieveldapsettings = true
	}

	repos, users, groups, permissiondetails, ldapsettings, ldapgroupsettings, err := GetStuff(client, baseurl, token, retrieveldapsettings)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if generate {
		err = Generate(repos, permissiondetails, useAllPermissionTargetsAsSource, onlyGenerateMatchingRepos, onlyGenerateCleanRepos, allowRenamedPermissions, combineRepos, split, repofiles[0], generateyaml)
		if err != nil {
			fmt.Printf("Error generating: %v\n", err)
			os.Exit(1)
		}
	} else {
		var ldapConfig LdapConfig
		if retrieveldapsettings {
			ldapConfig, err = loadLdapConfig(createUsers, importGroups, "ldap.config", ldapsettings, ldapgroupsettings)
			if err != nil {
				fmt.Printf("Error reading ldap config: %v\n", err)
				os.Exit(1)
			}
		}

		reposToProvision, err = Validate(reposToProvision, repos, permissiondetails)
		if err != nil {
			fmt.Printf("Error validating: %v\n", err)
			os.Exit(1)
		}

		err = Provision(client, baseurl, token, reposToProvision, repos, users, groups, permissiondetails, showDiff, allowpatterns, ldapConfig, dryRun)
		if err != nil {
			fmt.Printf("Error provisioning: %v\n", err)
			os.Exit(1)
		}
	}
}

func getFlagEnv(value bool, envname string) bool {
	envValue := strings.TrimSpace(os.Getenv(envname))
	if envValue == "1" || strings.EqualFold(envValue, "true") {
		return true
	}
	if envValue == "0" || strings.EqualFold(envValue, "false") {
		return false
	}
	return value
}

func loadLdapConfig(createUsers bool, importGroups bool, configFile string, ldapsettings []ArtifactoryLDAPSettings, ldapgroupsettings []ArtifactoryLDAPGroupSettings) (LdapConfig, error) {
	empty := LdapConfig{CreateUsers: false, ImportGroups: false}

	if len(ldapsettings) == 0 || len(ldapgroupsettings) == 0 {
		return empty, fmt.Errorf("Error: Couldn't retrieve ldap settings from Artifactory, cannot import ldap groups: ldapsettings count: %d, ldapgroupsettings count: %d",
			len(ldapsettings), len(ldapgroupsettings))
	}

	envLdapUsername := os.Getenv("ARTSYNC_LDAP_USERNAME")
	envLdapPassword := os.Getenv("ARTSYNC_LDAP_PASSWORD")
	envGroupsettingsname := os.Getenv("ARTSYNC_GROUPSETTINGSNAME")
	envArtifactoryUsername := os.Getenv("ARTSYNC_ARTIFACTORY_USERNAME")
	envArtifactoryPassword := os.Getenv("ARTSYNC_ARTIFACTORY_PASSWORD")

	if envLdapUsername != "" && envLdapPassword != "" && envGroupsettingsname != "" && envArtifactoryUsername != "" && envArtifactoryPassword != "" {
		return LdapConfig{
			CreateUsers:         createUsers,
			ImportGroups:        importGroups,
			LdapUsername:        envLdapUsername,
			LdapPassword:        envLdapPassword,
			Groupsettingsname:   envGroupsettingsname,
			ArtifactoryUsername: envArtifactoryUsername,
			ArtifactoryPassword: envArtifactoryPassword,
			Ldapsettings:        ldapsettings,
			Ldapgroupsettings:   ldapgroupsettings,
		}, nil
	}

	var ldapConfig LdapConfig

	fmt.Printf("Using ldap config file: '%s'\n", configFile)

	data, err := os.ReadFile(configFile)
	if err != nil {
		return empty, fmt.Errorf("Error reading ldap config file '%s': %v\n", configFile, err)
	}
	err = json.Unmarshal(data, &ldapConfig)
	if err != nil {
		return empty, fmt.Errorf("Error parsing ldap config file '%s': %v\n", configFile, err)
	}

	if envLdapUsername != "" {
		ldapConfig.LdapUsername = envLdapUsername
	}
	if envLdapPassword != "" {
		ldapConfig.LdapPassword = envLdapPassword
	}
	if envGroupsettingsname != "" {
		ldapConfig.Groupsettingsname = envGroupsettingsname
	}
	if envArtifactoryUsername != "" {
		ldapConfig.ArtifactoryUsername = envArtifactoryUsername
	}
	if envArtifactoryPassword != "" {
		ldapConfig.ArtifactoryPassword = envArtifactoryPassword
	}

	ldapConfig.CreateUsers = createUsers
	ldapConfig.ImportGroups = importGroups
	ldapConfig.Ldapsettings = ldapsettings
	ldapConfig.Ldapgroupsettings = ldapgroupsettings

	return ldapConfig, nil
}

func getBaseURL(arg string) string {
	var baseurl string

	if envBaseURL := os.Getenv("ARTSYNC_BASEURL"); envBaseURL != "" {
		baseurl = envBaseURL
	} else {
		baseurl = arg
	}
	if baseurl == "" {
		fmt.Println("Error: Base URL is empty.")
		os.Exit(1)
	}
	if _, err := url.ParseRequestURI(baseurl); err != nil {
		fmt.Printf("Error: Invalid base URL '%s': %v\n", baseurl, err)
		os.Exit(1)
	}

	return baseurl
}

func getToken(arg string) string {
	var token string

	if envToken := os.Getenv("ARTSYNC_TOKEN"); envToken != "" {
		token = envToken
	} else {
		data, err := os.ReadFile(arg)
		if err != nil {
			fmt.Printf("Error reading token file: %v\n", err)
			os.Exit(1)
		}
		token = string(data)
	}
	if token == "" {
		fmt.Println("Error: Token is empty.")
		os.Exit(1)
	}

	return token
}

func getRepoFiles(args []string) []string {
	var repofiles []string

	if envRepoFiles := os.Getenv("ARTSYNC_REPOFILES"); envRepoFiles != "" {
		repofiles = strings.Split(envRepoFiles, ",")
	} else {
		repofiles = args
	}
	for _, repofile := range repofiles {
		if repofile == "" {
			fmt.Println("Error: Repo file name empty.")
			os.Exit(1)
		}
	}

	return repofiles
}

func usage() {
	fmt.Println("ARTSYNC - Artifactory Repo Provisioning Tool")
	fmt.Println()
	fmt.Println("This tool is used to provision Artifactory repositories and matching permission targets.")
	fmt.Println("It can also generate a declarative file based on existing repos and permission targets.")
	fmt.Println()
	fmt.Println("Usage: artsync [-a] [-c] [-d] [-e] [-f] [-g] [-k] [-l] [-m] [-p] [-q] [-r] [-s] [-u] [-w] [-y] <baseurl> <tokenfile> <repofile1> [repofile2] ...")
	fmt.Println()
	fmt.Println("baseurl:    Base URL of Artifactory instance, like https://artifactory.example.com")
	fmt.Println("tokenfile:  File with access token (aka bearer token).")
	fmt.Println("repofile:   Input file with repo definitions (output file when using -g flag).")
	fmt.Println()
	flag.PrintDefaults()
	fmt.Println()
	fmt.Println("ARTSYNC_BASEURL: Environment variable that overrides the base URL value.")
	fmt.Println("ARTSYNC_TOKEN: Environment variable that overrides the token value.")
	fmt.Println("ARTSYNC_REPOFILES: Environment variable that overrides the repo files value. Comma separated list of repo files.")
	fmt.Println("")
	fmt.Println("Environment variables for overriding values in ldap.config:")
	fmt.Println("ARTSYNC_LDAP_USERNAME: Credentials for connecting to the LDAP server.")
	fmt.Println("ARTSYNC_LDAP_PASSWORD: -")
	fmt.Println("ARTSYNC_GROUPSETTINGSNAME: Selecting between multiple ldap group settings in Artifactory.")
	fmt.Println("ARTSYNC_ARTIFACTORY_USERNAME: Credentials for connecting to the Artifactory server.")
	fmt.Println("ARTSYNC_ARTIFACTORY_PASSWORD: -")
}
