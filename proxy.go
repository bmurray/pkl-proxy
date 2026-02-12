package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
)

type repoContextKey struct{}

type repoInfo struct {
	Owner string
	Repo  string
}

func withRepo(ctx context.Context, owner, repo string) context.Context {
	return context.WithValue(ctx, repoContextKey{}, repoInfo{Owner: owner, Repo: repo})
}

func repoFromContext(ctx context.Context) (owner, repo string, ok bool) {
	ri, ok := ctx.Value(repoContextKey{}).(repoInfo)
	if !ok {
		return "", "", false
	}
	return ri.Owner, ri.Repo, true
}

type GithubPrivateReleaseProxy struct {
	client  *http.Client
	handler http.Handler
	log     *slog.Logger
}

func NewGithubPrivateReleaseProxy(tm *TokenManager) *GithubPrivateReleaseProxy {
	client := &http.Client{
		Transport: &GithubTripper{tm: tm},
	}
	prox := &GithubPrivateReleaseProxy{
		client: client,
		log:    slog.Default().With("component", "GithubPrivateReleaseProxy"),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/{user}/{repo}/{tag}", prox.taggedHandler)
	mux.HandleFunc("/{user}/{repo}/{tag}/{file}", prox.taggedFileHandler)
	mux.HandleFunc("/{user}/{repo}/releases/download/{tag}/{file}", prox.taggedFileHandler)
	prox.handler = mux
	return prox
}

func (p *GithubPrivateReleaseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.log.Info("Received request", "method", r.Method, "url", r.URL.String())
	p.handler.ServeHTTP(w, r)
}

func (p *GithubPrivateReleaseProxy) taggedHandler(w http.ResponseWriter, r *http.Request) {
	user := r.PathValue("user")
	repo := r.PathValue("repo")
	tag := r.PathValue("tag")

	p.log.Info("Handling request for GitHub release", "user", user, "repo", repo, "tag", tag)

	ctx := withRepo(r.Context(), user, repo)
	files, err := p.files(ctx, user, repo, tag)
	if err != nil {
		p.log.Error("Error fetching release files", "error", err)
		http.Error(w, "Error fetching release files: "+err.Error(), http.StatusInternalServerError)
		return
	}
	for _, file := range files {
		if file.Name == tag {
			p.log.Info("Found matching file for tag", "file", file.Name, "url", file.BrowserDownloadURL)
			d, err := p.file(ctx, &file)
			if err != nil {
				p.log.Error("Error fetching file content", "error", err)
				http.Error(w, "Error fetching file content: "+err.Error(), http.StatusInternalServerError)
				return
			}
			defer d.Close()
			io.Copy(w, d)
			return
		}
	}
}

func (p *GithubPrivateReleaseProxy) taggedFileHandler(w http.ResponseWriter, r *http.Request) {
	user := r.PathValue("user")
	repo := r.PathValue("repo")
	tag := r.PathValue("tag")
	file := r.PathValue("file")
	p.log.Info("Handling request for GitHub release asset", "user", user, "repo", repo, "tag", tag, "file", file)

	ctx := withRepo(r.Context(), user, repo)
	files, err := p.files(ctx, user, repo, tag)
	if err != nil {
		p.log.Error("Error fetching release files", "error", err)
		http.Error(w, "Error fetching release files: "+err.Error(), http.StatusInternalServerError)
		return
	}
	for _, f := range files {
		if f.Name == file {
			p.log.Info("Found matching file for tag", "file", f.Name, "url", f.BrowserDownloadURL)
			d, err := p.file(ctx, &f)
			if err != nil {
				p.log.Error("Error fetching file content", "error", err)
				http.Error(w, "Error fetching file content: "+err.Error(), http.StatusInternalServerError)
				return
			}
			defer d.Close()
			io.Copy(w, d)
			return
		}
	}

	http.Error(w, "File not found in release assets", http.StatusNotFound)
}

func (p *GithubPrivateReleaseProxy) files(ctx context.Context, user, repo, tag string) ([]githubFileAsset, error) {
	ux, err := url.Parse("https://api.github.com/repos/")
	if err != nil {
		return nil, fmt.Errorf("error parsing base URL: %w", err)
	}
	ux = ux.JoinPath(user, repo, "releases", "tags", tag)

	p.log.Info("Fetching release info from GitHub API", "url", ux.String())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ux.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request to GitHub API: %w", err)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request to GitHub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned non-200 status: %s", resp.Status)
	}

	files := githubFilesReponse{}
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("error decoding GitHub API response: %w", err)
	}

	return files.Assets, nil
}

func (p *GithubPrivateReleaseProxy) file(ctx context.Context, asset *githubFileAsset) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request for asset: %w", err)
	}
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request for asset: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned non-200 status for asset: %s", resp.Status)
	}
	return resp.Body, nil
}

type githubFilesReponse struct {
	Assets []githubFileAsset `json:"assets"`
}

type githubFileAsset struct {
	Name               string `json:"name"`
	ContentType        string `json:"content_type"`
	BrowserDownloadURL string `json:"browser_download_url"`
	URL                string `json:"url"`
}

type GithubTripper struct {
	tm *TokenManager
}

func (t *GithubTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	owner, repo, ok := repoFromContext(req.Context())
	if !ok {
		return nil, fmt.Errorf("no repo context set on request")
	}
	token, err := t.tm.TokenForRepo(owner, repo)
	if err != nil {
		return nil, fmt.Errorf("error getting token: %w", err)
	}
	req.Header.Set("Authorization", "token "+token.AccessToken)
	return http.DefaultTransport.RoundTrip(req)
}
