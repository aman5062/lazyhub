// Package github is a lean GitHub REST API client. No SDK — just
// net/http + encoding/json, so the binary stays small and dependency-light.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const apiBase = "https://api.github.com"

// Client talks to the GitHub REST API with a bearer token.
type Client struct {
	token string
	http  *http.Client
}

func New(token string) *Client {
	return &Client{
		token: token,
		http:  &http.Client{Timeout: 20 * time.Second},
	}
}

// do performs an authenticated request and decodes JSON into out (if non-nil).
// It also returns the raw response so callers can read headers (scopes, paging).
func (c *Client) do(ctx context.Context, method, path string, out any) (*http.Response, error) {
	u := path
	if strings.HasPrefix(path, "/") {
		u = apiBase + path
	}
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "lazyhub")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return resp, fmt.Errorf("github %s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(body)))
	}
	if out != nil {
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp, fmt.Errorf("decode %s: %w", path, err)
		}
	}
	return resp, nil
}

// User is the authenticated account.
type User struct {
	Login string `json:"login"`
	Name  string `json:"name"`
	ID    int64  `json:"id"`
}

// Me returns the authenticated user. Used to validate a token and to
// grab the OAuth scopes from the response header.
func (c *Client) Me(ctx context.Context) (*User, string, error) {
	var u User
	resp, err := c.do(ctx, http.MethodGet, "/user", &u)
	if err != nil {
		return nil, "", err
	}
	scopes := resp.Header.Get("X-OAuth-Scopes")
	return &u, scopes, nil
}

// Repo is a trimmed repository record — only the fields the TUI shows.
type Repo struct {
	Name            string `json:"name"`
	FullName        string `json:"full_name"`
	NodeID          string `json:"node_id"` // GraphQL id, needed to create issues
	Private         bool   `json:"private"`
	Description     string `json:"description"`
	Language        string `json:"language"`
	StargazersCount int    `json:"stargazers_count"`
	OpenIssues      int    `json:"open_issues_count"`
	Fork            bool   `json:"fork"`
	Archived        bool   `json:"archived"`
	HasIssues       bool   `json:"has_issues"`
	PushedAt        string `json:"pushed_at"`
	HTMLURL         string `json:"html_url"`
	SSHURL          string `json:"ssh_url"`
	DefaultBranch   string `json:"default_branch"`
	Permissions     struct {
		Admin    bool `json:"admin"`
		Maintain bool `json:"maintain"`
		Push     bool `json:"push"`
		Triage   bool `json:"triage"`
	} `json:"permissions"`
}

// CanFileIssues reports whether the viewer may open issues in this repo:
// issues enabled, not archived, and at least triage/write access.
func (r Repo) CanFileIssues() bool {
	if r.Archived || !r.HasIssues {
		return false
	}
	p := r.Permissions
	return p.Push || p.Triage || p.Maintain || p.Admin
}

// ListWritableRepos returns the viewer's repos they can open issues in,
// most-recently-pushed first — so filing a real issue is just picking one.
func (c *Client) ListWritableRepos(ctx context.Context) ([]Repo, error) {
	repos, err := c.ListRepos(ctx, 1, 100)
	if err != nil {
		return nil, err
	}
	out := make([]Repo, 0, len(repos))
	for _, r := range repos {
		if r.CanFileIssues() {
			out = append(out, r)
		}
	}
	return out, nil
}

// doJSON is like do but sends a JSON body.
func (c *Client) doJSON(ctx context.Context, method, path string, in, out any) error {
	u := apiBase + path
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "lazyhub")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("github %s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(b)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// Assignee is a user who can be assigned to an issue/PR.
type Assignee struct {
	Login string `json:"login"`
}

// ListAssignableUsers returns logins that can be assigned in a repo.
func (c *Client) ListAssignableUsers(ctx context.Context, owner, repo string) ([]string, error) {
	var users []Assignee
	err := c.doJSON(ctx, http.MethodGet,
		fmt.Sprintf("/repos/%s/%s/assignees?per_page=100", owner, repo), nil, &users)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(users))
	for _, u := range users {
		out = append(out, u.Login)
	}
	return out, nil
}

// AddAssignees assigns logins to an issue/PR (works for both).
func (c *Client) AddAssignees(ctx context.Context, owner, repo string, number int, logins []string) error {
	return c.doJSON(ctx, http.MethodPost,
		fmt.Sprintf("/repos/%s/%s/issues/%d/assignees", owner, repo, number),
		map[string]any{"assignees": logins}, nil)
}

// RemoveAssignees unassigns logins from an issue/PR.
func (c *Client) RemoveAssignees(ctx context.Context, owner, repo string, number int, logins []string) error {
	return c.doJSON(ctx, http.MethodDelete,
		fmt.Sprintf("/repos/%s/%s/issues/%d/assignees", owner, repo, number),
		map[string]any{"assignees": logins}, nil)
}

// AddComment posts a comment on an issue or PR.
func (c *Client) AddComment(ctx context.Context, owner, repo string, number int, body string) error {
	return c.doJSON(ctx, http.MethodPost,
		fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, number),
		map[string]any{"body": body}, nil)
}

// ListRepos returns repos for the authenticated user, most-recently-pushed
// first, paginated. perPage max is 100.
func (c *Client) ListRepos(ctx context.Context, page, perPage int) ([]Repo, error) {
	q := url.Values{}
	q.Set("sort", "pushed")
	q.Set("affiliation", "owner,collaborator,organization_member")
	q.Set("per_page", strconv.Itoa(perPage))
	q.Set("page", strconv.Itoa(page))
	var repos []Repo
	_, err := c.do(ctx, http.MethodGet, "/user/repos?"+q.Encode(), &repos)
	return repos, err
}
