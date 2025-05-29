package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func main() {
	useAllPermissionTargetsAsSource := flag.Bool("a", false, "Use all permission targets as source, when generating.")
	dryRun := flag.Bool("d", false, "Enable dry run mode (read-only, no changes will be made).")
	generate := flag.Bool("g", false, "Generate repo file.")
	overwrite := flag.Bool("o", false, "Allow overwriting of existing repo file.")
	generateyaml := flag.Bool("y", false, "Generate output in yaml format.")

	flag.Parse()
	args := flag.Args()
	if len(args) < 3 || args[0] == "" || args[1] == "" || args[2] == "" {
		usage()
		os.Exit(1)
	}

	baseurl := getBaseURL(args[0])
	token := getToken(args[1])
	repofiles := getRepoFiles(args[2:])

	if *generate && len(repofiles) > 1 {
		fmt.Println("Error: Only one repo file is allowed when using -g flag.")
		os.Exit(1)
	}

	if !*generate {
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

	if *generate && !*overwrite {
		if _, err := os.Stat(repofiles[0]); err == nil {
			fmt.Printf("Error: File already exists, will not overwrite: '%s'\n", repofiles[0])
			os.Exit(1)
		}
	}

	client := &http.Client{}

	var reposToProvision []Repo

	if !*generate {
		var err error
		reposToProvision, err = LoadRepoFiles(repofiles)
		if err != nil {
			fmt.Printf("Error validating repo file: %v\n", err)
			os.Exit(1)
		}
	}

	if len(reposToProvision) == 0 && !*generate {
		fmt.Println("Error: No repos to provision found in the provided repo files.")
		os.Exit(1)
	}

	repos, users, groups, permissiondetails, err := GetStuff(client, baseurl, token)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if *generate {
		Generate(repos, permissiondetails, *useAllPermissionTargetsAsSource, repofiles[0], *generateyaml)
		if err != nil {
			fmt.Printf("Error generating: %v\n", err)
			os.Exit(1)
		}
	} else {
		err = Provision(reposToProvision, repos, users, groups, permissiondetails, client, baseurl, token, *dryRun)
		if err != nil {
			fmt.Printf("Error provisioning: %v\n", err)
			os.Exit(1)
		}
	}
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
			fmt.Println("Error: Repo file empty.")
			os.Exit(1)
		}
	}

	return repofiles
}

func usage() {
	fmt.Println("Usage: artsync [-a] [-d] [-g] [-o] [-y] <baseurl> <tokenfile> <repofile1> [repofile2] ...")
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
}
