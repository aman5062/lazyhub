package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const graphqlURL = "https://api.github.com/graphql"

// graphql runs a GraphQL query/mutation and decodes `data` into out.
func (c *Client) graphql(ctx context.Context, query string, vars map[string]any, out any) error {
	body, _ := json.Marshal(map[string]any{"query": query, "variables": vars})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, graphqlURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "lazyhub")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("graphql http %s: %s", resp.Status, string(raw))
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return err
	}
	if len(envelope.Errors) > 0 {
		return fmt.Errorf("graphql: %s", envelope.Errors[0].Message)
	}
	if out != nil {
		return json.Unmarshal(envelope.Data, out)
	}
	return nil
}

// Project is a Projects V2 board.
type Project struct {
	ID          string `json:"id"`
	Number      int    `json:"number"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"shortDescription"`
	Owner       string // login of the user/org that owns it (filled in by us)
	Closed      bool   `json:"closed"`
}

// ListProjects returns the viewer's own projects plus projects from every
// org the viewer belongs to.
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	const q = `
query {
  viewer {
    login
    projectsV2(first: 50, query: "is:open") {
      nodes { id number title url shortDescription closed }
    }
    organizations(first: 25) {
      nodes {
        login
        projectsV2(first: 50, query: "is:open") {
          nodes { id number title url shortDescription closed }
        }
      }
    }
  }
}`
	var data struct {
		Viewer struct {
			Login       string `json:"login"`
			ProjectsV2  struct {
				Nodes []Project `json:"nodes"`
			} `json:"projectsV2"`
			Organizations struct {
				Nodes []struct {
					Login      string `json:"login"`
					ProjectsV2 struct {
						Nodes []Project `json:"nodes"`
					} `json:"projectsV2"`
				} `json:"nodes"`
			} `json:"organizations"`
		} `json:"viewer"`
	}
	if err := c.graphql(ctx, q, nil, &data); err != nil {
		return nil, err
	}
	var out []Project
	for _, p := range data.Viewer.ProjectsV2.Nodes {
		p.Owner = data.Viewer.Login
		out = append(out, p)
	}
	for _, org := range data.Viewer.Organizations.Nodes {
		for _, p := range org.ProjectsV2.Nodes {
			p.Owner = org.Login
			out = append(out, p)
		}
	}
	return out, nil
}

// ProjectItem is one card/ticket on a board.
type ProjectItem struct {
	ItemID    string   // ProjectV2Item node id (used to move it between columns)
	Type      string   // ISSUE | PULL_REQUEST | DRAFT_ISSUE
	Number    int      // issue/PR number (0 for draft)
	Title     string   //
	URL       string   //
	State     string   // OPEN | CLOSED | MERGED
	RepoOwner string   //
	RepoName  string   //
	Assignees []string // logins
	Status    string   // the "Status" single-select field value = the column
}

// ListProjectItems returns the items on a board with their Status column
// and assignees.
func (c *Client) ListProjectItems(ctx context.Context, projectID string) ([]ProjectItem, error) {
	const q = `
query($id: ID!) {
  node(id: $id) {
    ... on ProjectV2 {
      items(first: 100) {
        nodes {
          id
          fieldValues(first: 20) {
            nodes {
              ... on ProjectV2ItemFieldSingleSelectValue {
                name
                field { ... on ProjectV2FieldCommon { name } }
              }
            }
          }
          content {
            __typename
            ... on DraftIssue { title }
            ... on Issue {
              number title url state
              repository { name owner { login } }
              assignees(first: 10) { nodes { login } }
            }
            ... on PullRequest {
              number title url state
              repository { name owner { login } }
              assignees(first: 10) { nodes { login } }
            }
          }
        }
      }
    }
  }
}`
	var data struct {
		Node struct {
			Items struct {
				Nodes []struct {
					ID          string `json:"id"`
					FieldValues struct {
						Nodes []struct {
							Name  string `json:"name"`
							Field struct {
								Name string `json:"name"`
							} `json:"field"`
						} `json:"nodes"`
					} `json:"fieldValues"`
					Content struct {
						Typename   string `json:"__typename"`
						Number     int    `json:"number"`
						Title      string `json:"title"`
						URL        string `json:"url"`
						State      string `json:"state"`
						Repository struct {
							Name  string `json:"name"`
							Owner struct {
								Login string `json:"login"`
							} `json:"owner"`
						} `json:"repository"`
						Assignees struct {
							Nodes []struct {
								Login string `json:"login"`
							} `json:"nodes"`
						} `json:"assignees"`
					} `json:"content"`
				} `json:"nodes"`
			} `json:"items"`
		} `json:"node"`
	}
	if err := c.graphql(ctx, q, map[string]any{"id": projectID}, &data); err != nil {
		return nil, err
	}
	var out []ProjectItem
	for _, n := range data.Node.Items.Nodes {
		it := ProjectItem{
			ItemID:    n.ID,
			Type:      n.Content.Typename,
			Number:    n.Content.Number,
			Title:     n.Content.Title,
			URL:       n.Content.URL,
			State:     n.Content.State,
			RepoOwner: n.Content.Repository.Owner.Login,
			RepoName:  n.Content.Repository.Name,
		}
		for _, a := range n.Content.Assignees.Nodes {
			it.Assignees = append(it.Assignees, a.Login)
		}
		for _, fv := range n.FieldValues.Nodes {
			if fv.Field.Name == "Status" {
				it.Status = fv.Name
			}
		}
		if it.Status == "" {
			it.Status = "No Status"
		}
		out = append(out, it)
	}
	return out, nil
}

// Label is an issue/PR label with its GitHub color (hex, no leading #).
type Label struct {
	Name  string
	Color string
}

// Comment is a single issue/PR comment.
type Comment struct {
	Author    string
	Body      string
	CreatedAt string
}

// ItemDetail is the full read-only detail of a card's underlying issue/PR/draft
// — everything the compact board card can't show: the body, who opened it and
// when, its labels and milestone, and the most recent comments.
type ItemDetail struct {
	Body         string
	Author       string
	CreatedAt    string
	Labels       []Label
	Milestone    string
	Comments     []Comment
	CommentTotal int
}

// GetItemDetail fetches the full detail of a board item by its ProjectV2Item
// node id. Works for issues, pull requests, and draft issues (drafts only have
// a title, body, and creator — no labels/comments).
func (c *Client) GetItemDetail(ctx context.Context, itemID string) (*ItemDetail, error) {
	const q = `
query($id: ID!) {
  node(id: $id) {
    ... on ProjectV2Item {
      content {
        __typename
        ... on DraftIssue {
          body
          creator { login }
          createdAt
        }
        ... on Issue {
          body createdAt
          author { login }
          milestone { title }
          labels(first: 20) { nodes { name color } }
          comments(last: 20) {
            totalCount
            nodes { author { login } body createdAt }
          }
        }
        ... on PullRequest {
          body createdAt
          author { login }
          milestone { title }
          labels(first: 20) { nodes { name color } }
          comments(last: 20) {
            totalCount
            nodes { author { login } body createdAt }
          }
        }
      }
    }
  }
}`
	var data struct {
		Node struct {
			Content struct {
				Typename  string `json:"__typename"`
				Body      string `json:"body"`
				CreatedAt string `json:"createdAt"`
				Author    struct {
					Login string `json:"login"`
				} `json:"author"`
				Creator struct {
					Login string `json:"login"`
				} `json:"creator"`
				Milestone struct {
					Title string `json:"title"`
				} `json:"milestone"`
				Labels struct {
					Nodes []struct {
						Name  string `json:"name"`
						Color string `json:"color"`
					} `json:"nodes"`
				} `json:"labels"`
				Comments struct {
					TotalCount int `json:"totalCount"`
					Nodes      []struct {
						Author struct {
							Login string `json:"login"`
						} `json:"author"`
						Body      string `json:"body"`
						CreatedAt string `json:"createdAt"`
					} `json:"nodes"`
				} `json:"comments"`
			} `json:"content"`
		} `json:"node"`
	}
	if err := c.graphql(ctx, q, map[string]any{"id": itemID}, &data); err != nil {
		return nil, err
	}
	cn := data.Node.Content
	d := &ItemDetail{
		Body:         cn.Body,
		Author:       cn.Author.Login,
		CreatedAt:    cn.CreatedAt,
		Milestone:    cn.Milestone.Title,
		CommentTotal: cn.Comments.TotalCount,
	}
	if d.Author == "" { // drafts expose the opener as `creator`
		d.Author = cn.Creator.Login
	}
	for _, l := range cn.Labels.Nodes {
		d.Labels = append(d.Labels, Label{Name: l.Name, Color: l.Color})
	}
	for _, cm := range cn.Comments.Nodes {
		d.Comments = append(d.Comments, Comment{
			Author:    cm.Author.Login,
			Body:      cm.Body,
			CreatedAt: cm.CreatedAt,
		})
	}
	return d, nil
}

// StatusOption is one column of the board (a Status single-select option).
type StatusOption struct {
	ID   string
	Name string
}

// StatusField is the board's Status field plus its columns, needed to move
// tickets between columns.
type StatusField struct {
	FieldID string
	Options []StatusOption
}

// GetStatusField fetches the "Status" single-select field for a project.
func (c *Client) GetStatusField(ctx context.Context, projectID string) (*StatusField, error) {
	const q = `
query($id: ID!) {
  node(id: $id) {
    ... on ProjectV2 {
      field(name: "Status") {
        ... on ProjectV2SingleSelectField {
          id
          options { id name }
        }
      }
    }
  }
}`
	var data struct {
		Node struct {
			Field struct {
				ID      string `json:"id"`
				Options []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"options"`
			} `json:"field"`
		} `json:"node"`
	}
	if err := c.graphql(ctx, q, map[string]any{"id": projectID}, &data); err != nil {
		return nil, err
	}
	if data.Node.Field.ID == "" {
		return nil, fmt.Errorf("this project has no Status field")
	}
	sf := &StatusField{FieldID: data.Node.Field.ID}
	for _, o := range data.Node.Field.Options {
		sf.Options = append(sf.Options, StatusOption{ID: o.ID, Name: o.Name})
	}
	return sf, nil
}

// SingleSelectField is any single-select custom field on a board (Status,
// Priority, Size, …) with its selectable options.
type SingleSelectField struct {
	ID      string
	Name    string
	Options []StatusOption
}

// ListSingleSelectFields returns every single-select field on the project, so
// the user can set Priority/Size/etc — not just Status. Options are synced
// live from GitHub, so custom values always match the board.
func (c *Client) ListSingleSelectFields(ctx context.Context, projectID string) ([]SingleSelectField, error) {
	const q = `
query($id: ID!) {
  node(id: $id) {
    ... on ProjectV2 {
      fields(first: 50) {
        nodes {
          ... on ProjectV2SingleSelectField {
            id
            name
            options { id name }
          }
        }
      }
    }
  }
}`
	var data struct {
		Node struct {
			Fields struct {
				Nodes []struct {
					ID      string `json:"id"`
					Name    string `json:"name"`
					Options []struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"options"`
				} `json:"nodes"`
			} `json:"fields"`
		} `json:"node"`
	}
	if err := c.graphql(ctx, q, map[string]any{"id": projectID}, &data); err != nil {
		return nil, err
	}
	var out []SingleSelectField
	for _, f := range data.Node.Fields.Nodes {
		if f.ID == "" {
			continue // non-single-select fields decode as empty
		}
		sf := SingleSelectField{ID: f.ID, Name: f.Name}
		for _, o := range f.Options {
			sf.Options = append(sf.Options, StatusOption{ID: o.ID, Name: o.Name})
		}
		out = append(out, sf)
	}
	return out, nil
}

// AddDraftIssue creates a draft ticket directly on the board.
func (c *Client) AddDraftIssue(ctx context.Context, projectID, title, body string) error {
	const m = `
mutation($project: ID!, $title: String!, $body: String) {
  addProjectV2DraftIssue(input: { projectId: $project, title: $title, body: $body }) {
    projectItem { id }
  }
}`
	return c.graphql(ctx, m, map[string]any{"project": projectID, "title": title, "body": body}, nil)
}

// SetItemStatus moves a ticket to a different column.
func (c *Client) SetItemStatus(ctx context.Context, projectID, itemID, fieldID, optionID string) error {
	const m = `
mutation($project: ID!, $item: ID!, $field: ID!, $option: String!) {
  updateProjectV2ItemFieldValue(input: {
    projectId: $project, itemId: $item, fieldId: $field,
    value: { singleSelectOptionId: $option }
  }) { projectV2Item { id } }
}`
	return c.graphql(ctx, m, map[string]any{
		"project": projectID, "item": itemID, "field": fieldID, "option": optionID,
	}, nil)
}
