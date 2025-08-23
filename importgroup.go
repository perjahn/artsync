package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type TokenRequest struct {
	Username string `json:"username"`
	Scope    string `json:"scope"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func ImportGroup(artifactoryUrl string, username string, password string) {
	client := &http.Client{}
	reqBody := fmt.Appendf(nil, `{"username=%s&password=%s&grant_type=password"}`, username, password)
	req, err := http.NewRequest("POST", artifactoryUrl+"/access/api/v1/tokens", bytes.NewBuffer(reqBody))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}
	req.Header.Set("Content-Type", "applciation/x-www-form-urlencoded")
	req.SetBasicAuth(username, password)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}
	defer req.Body.Close()

	body, _ := io.ReadAll(req.Body)
	if resp.StatusCode != 200 {
		fmt.Printf("Error: %s\n", string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		fmt.Printf("Error parsing response: %v\n", err)
		return
	}
	fmt.Printf("Access Token: %s\n", tokenResp.AccessToken)
	fmt.Printf("Refresh Token: %s\n", tokenResp.RefreshToken)

	importGroupFromLDAP(client, artifactoryUrl, tokenResp.AccessToken, tokenResp.RefreshToken)
}

func importGroupFromLDAP(client *http.Client, artifactoryUrl, accessToken, refreshToken string) {
	url := artifactoryUrl + "/ui/api/v1/access/api/ui/ldap/groups/import"

	payload := []byte(`{
   		"groupName": "example-group",
   		"strategy": "STATIC"
   	}`)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		fmt.Printf("Error creating group1: %v\n", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.AddCookie(&http.Cookie{Name: "ACCESSTOKEN", Value: accessToken})
	req.AddCookie(&http.Cookie{Name: "REFRESHTOKEN", Value: refreshToken})

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error creating group2: %v\n", err)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	fmt.Println("Status:", resp.Status)
	fmt.Println("Response:", string(respBody))
}
