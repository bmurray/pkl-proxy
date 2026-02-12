package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"

	"golang.org/x/oauth2"
)

type GithubPrivateReleaseProxy struct {
	client  *http.Client
	handler http.Handler
	log     *slog.Logger
}

func NewGithubPrivateReleaseProxy(tokenSource oauth2.TokenSource) *GithubPrivateReleaseProxy {
	client := &http.Client{
		Transport: &GithubTripper{tokenSource: tokenSource},
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

	files, err := p.files(r.Context(), user, repo, tag)
	if err != nil {
		p.log.Error("Error fetching release files", "error", err)
		http.Error(w, "Error fetching release files: "+err.Error(), http.StatusInternalServerError)
		return
	}
	for _, file := range files {
		if file.Name == tag {
			p.log.Info("Found matching file for tag", "file", file.Name, "url", file.BrowserDownloadURL)
			d, err := p.file(r.Context(), &file)
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

	files, err := p.files(r.Context(), user, repo, tag)
	if err != nil {
		p.log.Error("Error fetching release files", "error", err)
		http.Error(w, "Error fetching release files: "+err.Error(), http.StatusInternalServerError)
		return
	}
	for _, f := range files {
		if f.Name == file {
			p.log.Info("Found matching file for tag", "file", f.Name, "url", f.BrowserDownloadURL)
			d, err := p.file(r.Context(), &f)
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
	tokenSource oauth2.TokenSource
}

func (t *GithubTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := t.tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("error getting token: %w", err)
	}
	req.Header.Set("Authorization", "token "+token.AccessToken)
	return http.DefaultTransport.RoundTrip(req)
}
