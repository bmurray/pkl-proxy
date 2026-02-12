package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/bmurray/pkl-proxy/gen/appconfig"
	"github.com/jferrl/go-githubauth"
	"golang.org/x/oauth2"
)

// TokenManager lazily discovers and caches installation token sources per owner.
type TokenManager struct {
	appTokenSource oauth2.TokenSource
	installationId *int // optional fixed installation ID from config

	mu    sync.RWMutex
	cache map[string]oauth2.TokenSource // owner -> token source
}

// NewTokenManager creates a TokenManager from config. If installationId is set,
// all repos use that installation (no per-repo lookup). Otherwise, installations
// are auto-discovered per owner on first request.
func NewTokenManager(config *appconfig.AppConfig, privateKey []byte) (*TokenManager, error) {
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

	tm := &TokenManager{
		appTokenSource: appTokenSource,
		installationId: config.InstallationId,
		cache:          make(map[string]oauth2.TokenSource),
	}

	// Print available installations at startup for diagnostics
	installations, err := discoverInstallations(appTokenSource)
	if err != nil {
		fmt.Printf("Warning: could not list installations: %v\n", err)
	} else if len(installations) == 0 {
		fmt.Println("Warning: no installations found; install the GitHub App on an account first")
	} else {
		fmt.Println("Available installations:")
		for _, inst := range installations {
			fmt.Printf("  - %s (installation ID: %d)\n", inst.Account.Login, inst.ID)
		}
	}

	return tm, nil
}

// TokenForRepo returns a token valid for the given owner/repo. Results are cached
// per owner since installations are typically per-account.
func (tm *TokenManager) TokenForRepo(owner, repo string) (*oauth2.Token, error) {
	// If a fixed installation ID is configured, use it for everything
	if tm.installationId != nil {
		ts := tm.getOrSetSource(owner, func() oauth2.TokenSource {
			return githubauth.NewInstallationTokenSource(int64(*tm.installationId), tm.appTokenSource)
		})
		return ts.Token()
	}

	// Check cache (read lock)
	tm.mu.RLock()
	ts, ok := tm.cache[owner]
	tm.mu.RUnlock()
	if ok {
		return ts.Token()
	}

	// Cache miss — look up the installation for this repo
	installationID, err := tm.lookupRepoInstallation(owner, repo)
	if err != nil {
		return nil, fmt.Errorf("looking up installation for %s/%s: %w", owner, repo, err)
	}

	ts = tm.getOrSetSource(owner, func() oauth2.TokenSource {
		return githubauth.NewInstallationTokenSource(int64(installationID), tm.appTokenSource)
	})
	return ts.Token()
}

// getOrSetSource returns the cached token source for owner, or creates one using
// the provided factory function. Handles the race where two goroutines both miss
// the read cache concurrently.
func (tm *TokenManager) getOrSetSource(owner string, factory func() oauth2.TokenSource) oauth2.TokenSource {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if ts, ok := tm.cache[owner]; ok {
		return ts
	}
	ts := factory()
	tm.cache[owner] = ts
	return ts
}

// lookupRepoInstallation calls GET /repos/{owner}/{repo}/installation to find
// the installation ID covering a specific repo.
func (tm *TokenManager) lookupRepoInstallation(owner, repo string) (int, error) {
	token, err := tm.appTokenSource.Token()
	if err != nil {
		return 0, fmt.Errorf("getting app token: %w", err)
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/installation", owner, repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("GitHub API returned %s for %s/%s installation lookup", resp.Status, owner, repo)
	}

	var inst ghInstallation
	if err := json.NewDecoder(resp.Body).Decode(&inst); err != nil {
		return 0, fmt.Errorf("decoding installation response: %w", err)
	}

	fmt.Printf("Discovered installation %d (%s) for %s/%s\n", inst.ID, inst.Account.Login, owner, repo)
	return inst.ID, nil
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
