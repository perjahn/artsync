package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
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
		fmt.Println("Usage: artsync [-a] [-d] [-g] [-o] [-y] <baseurl> <tokenfile> <repofile1> [repofile2] ...")
		fmt.Println()
		flag.Usage()
		fmt.Println("baseurl:    Base URL of Artifactory instance, like https://artifactory.example.com")
		fmt.Println("repofile:   Input file with repo definitions (output file when using -g flag).")
		fmt.Println("tokenfile:  File with access token (aka bearer token).")
		os.Exit(1)
	}

	baseurl := args[0]
	tokenfile := args[1]
	repofiles := args[2:]

	if *generate && len(repofiles) > 1 {
		fmt.Println("Only one repo file is allowed when using -g flag.")
		os.Exit(1)
	}

	if !*generate {
		success := true
		for _, repofile := range repofiles {
			if _, err := os.Stat(repofile); os.IsNotExist(err) {
				fmt.Printf("Repo file not found: '%s'\n", repofile)
				success = false
			}
		}
		if !success {
			os.Exit(1)
		}
	}

	if *generate && !*overwrite {
		if _, err := os.Stat(repofiles[0]); err == nil {
			fmt.Printf("File already exists, will not overwrite: '%s'\n", repofiles[0])
			os.Exit(1)
		}
	}

	data, err := os.ReadFile(tokenfile)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}
	token := string(data)

	client := &http.Client{}

	var reposToProvision []Repo

	if !*generate {
		reposToProvision, err = LoadRepoFiles(repofiles)
		if err != nil {
			fmt.Printf("Error validating repo file: %v\n", err)
			os.Exit(1)
		}
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
