package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/bmurray/pkl-proxy/gen/appconfig"
	"github.com/jferrl/go-githubauth"
	"golang.org/x/oauth2"
)

// buildTokenSource creates the appropriate oauth2.TokenSource based on config.
// If installationId is set, it uses it directly. Otherwise, it auto-discovers installations.
// appId and clientId are both supported for creating the application token.
func buildTokenSource(config *appconfig.AppConfig, privateKey []byte) (oauth2.TokenSource, error) {
	var appTokenSource oauth2.TokenSource
	var err error

	switch {
	case config.AppId != nil:
		appTokenSource, err = githubauth.NewApplicationTokenSource(int64(*config.AppId), privateKey)
	case config.ClientId != nil:
		appTokenSource, err = githubauth.NewApplicationTokenSource(*config.ClientId, privateKey)
	default:
		return nil, fmt.Errorf("config must set either appId or clientId")
	}
	if err != nil {
		return nil, fmt.Errorf("creating application token source: %w", err)
	}

	// If installationId is explicit, use it directly
	if config.InstallationId != nil {
		return githubauth.NewInstallationTokenSource(int64(*config.InstallationId), appTokenSource), nil
	}

	// Auto-discover installations
	installations, err := discoverInstallations(appTokenSource)
	if err != nil {
		return nil, fmt.Errorf("discovering installations: %w", err)
	}

	if len(installations) == 0 {
		return nil, fmt.Errorf("no installations found; install the GitHub App on an account first")
	}

	if len(installations) == 1 {
		inst := installations[0]
		fmt.Printf("Using installation %d (%s)\n", inst.ID, inst.Account.Login)
		return githubauth.NewInstallationTokenSource(int64(inst.ID), appTokenSource), nil
	}

	// Multiple installations — list them and ask the user to specify
	fmt.Println("Multiple installations found:")
	for _, inst := range installations {
		fmt.Printf("  - %s (installation ID: %d)\n", inst.Account.Login, inst.ID)
	}
	return nil, fmt.Errorf("multiple installations found; set installationId in config to select one")
}

type ghInstallation struct {
	ID      int `json:"id"`
	Account struct {
		Login string `json:"login"`
	} `json:"account"`
	RepositorySelection string `json:"repository_selection"`
}

// discoverInstallations calls GET /app/installations to find all installations for the app.
func discoverInstallations(appTokenSource oauth2.TokenSource) ([]ghInstallation, error) {
	token, err := appTokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("getting app token: %w", err)
	}

	req, err := http.NewRequest("GET", "https://api.github.com/app/installations", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %s", resp.Status)
	}

	var installations []ghInstallation
	if err := json.NewDecoder(resp.Body).Decode(&installations); err != nil {
		return nil, fmt.Errorf("decoding installations response: %w", err)
	}

	return installations, nil
}
