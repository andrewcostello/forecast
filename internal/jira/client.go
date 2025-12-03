package jira

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/andrewcostello/forecast/internal/config"
	"github.com/andrewcostello/forecast/pkg/forecast"
)

// Client represents a JIRA API client
type Client struct {
	baseURL  string
	email    string
	apiToken string
	client   *http.Client
}

// NewClient creates a new JIRA client
func NewClient(cfg *config.JIRAConfig) *Client {
	return &Client{
		baseURL:  cfg.URL,
		email:    cfg.Email,
		apiToken: cfg.APIToken,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// Issue represents a JIRA issue
type Issue struct {
	Key    string `json:"key"`
	Fields struct {
		Summary     string    `json:"summary"`
		Status      Status    `json:"status"`
		IssueType   IssueType `json:"issuetype"`
		Labels      []string  `json:"labels"`
		Assignee    *User     `json:"assignee"`
		Created     string    `json:"created"`
		Updated     string    `json:"updated"`
		Resolution  *Resolution `json:"resolution"`
	} `json:"fields"`
	Changelog *Changelog `json:"changelog,omitempty"`
}

type Status struct {
	Name string `json:"name"`
}

type IssueType struct {
	Name string `json:"name"`
}

type User struct {
	DisplayName string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
}

type Resolution struct {
	Name string `json:"name"`
}

type Changelog struct {
	Histories []History `json:"histories"`
}

type History struct {
	Created string `json:"created"`
	Items   []ChangeItem `json:"items"`
}

type ChangeItem struct {
	Field      string `json:"field"`
	FieldType  string `json:"fieldtype"`
	From       string `json:"from"`
	FromString string `json:"fromString"`
	To         string `json:"to"`
	ToString   string `json:"toString"`
}

// SearchResponse represents JIRA search API response
type SearchResponse struct {
	Issues []Issue `json:"issues"`
	Total  int     `json:"total"`
}

// FetchIssues fetches issues from JIRA based on config
func (c *Client) FetchIssues(cfg *config.Config) ([]forecast.Item, error) {
	// Build JQL query
	jql := c.buildJQL(cfg)

	// Make API request
	issues, err := c.search(jql)
	if err != nil {
		return nil, err
	}

	// Convert JIRA issues to forecast items
	items := make([]forecast.Item, 0, len(issues))
	for _, issue := range issues {
		item := c.convertIssue(issue, cfg)
		items = append(items, item)
	}

	return items, nil
}

func (c *Client) buildJQL(cfg *config.Config) string {
	jql := fmt.Sprintf(`project = %s`, cfg.JIRA.ProjectKey)

	if cfg.JIRA.Epic != "" {
		jql += fmt.Sprintf(` AND "Epic Link" = %s`, cfg.JIRA.Epic)
	}

	if len(cfg.JIRA.Labels) > 0 {
		jql += ` AND labels in (`
		for i, label := range cfg.JIRA.Labels {
			if i > 0 {
				jql += `, `
			}
			jql += label
		}
		jql += `)`
	}

	jql += ` ORDER BY created ASC`

	return jql
}

func (c *Client) search(jql string) ([]Issue, error) {
	u := fmt.Sprintf("%s/rest/api/3/search/jql", c.baseURL)

	// Use POST with JSON body
	requestBody := map[string]interface{}{
		"jql":        jql,
		"expand":     "changelog",
		"maxResults": 1000,
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", u, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(c.email, c.apiToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("JIRA API error: %d - %s", resp.StatusCode, string(body))
	}

	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, err
	}

	return searchResp.Issues, nil
}

func (c *Client) convertIssue(issue Issue, cfg *config.Config) forecast.Item {
	item := forecast.Item{
		ID:          issue.Key,
		JiraKey:     issue.Key,
		Description: issue.Fields.Summary,
		Status:      issue.Fields.Status.Name,
	}

	// Parse created time
	if created, err := time.Parse(time.RFC3339, issue.Fields.Created); err == nil {
		item.Created = created
	}

	// Extract item type from labels
	item.Type = c.extractItemType(issue.Fields.Labels, cfg)

	// Extract size from labels
	item.Size = c.extractSize(issue.Fields.Labels, cfg)

	// Extract assignee
	if issue.Fields.Assignee != nil {
		item.Assignee = issue.Fields.Assignee.DisplayName
	}

	// Calculate cycle time from changelog
	inProgressTime, doneTime := c.extractStateTransitions(issue.Changelog)
	if inProgressTime != nil {
		item.InProgress = inProgressTime
	}
	if doneTime != nil {
		item.Done = doneTime
		if inProgressTime != nil {
			item.CycleTime = doneTime.Sub(*inProgressTime).Hours()
		}
	}

	return item
}

func (c *Client) extractItemType(labels []string, cfg *config.Config) string {
	mapping := cfg.JIRAMapping.ItemType

	for _, label := range labels {
		for _, m := range mapping.Mappings {
			if label == m.JIRA {
				return m.Forecast
			}
		}
	}

	return "Unknown"
}

func (c *Client) extractSize(labels []string, cfg *config.Config) string {
	mapping := cfg.JIRAMapping.Size

	for _, label := range labels {
		for _, m := range mapping.Mappings {
			if label == m.JIRA {
				return m.Forecast
			}
		}
	}

	return "M" // Default to Medium
}

func (c *Client) extractStateTransitions(changelog *Changelog) (*time.Time, *time.Time) {
	if changelog == nil {
		return nil, nil
	}

	var inProgressTime, doneTime *time.Time

	for _, history := range changelog.Histories {
		for _, item := range history.Items {
			if item.Field == "status" {
				timestamp, err := time.Parse(time.RFC3339, history.Created)
				if err != nil {
					continue
				}

				// Track "In Progress" transition
				if item.ToString == "In Progress" && inProgressTime == nil {
					inProgressTime = &timestamp
				}

				// Track "Done" transition
				if item.ToString == "Done" && doneTime == nil {
					doneTime = &timestamp
				}
			}
		}
	}

	return inProgressTime, doneTime
}
