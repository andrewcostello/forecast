package jira

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"bitbucket.org/supermoneygames/forecast/internal/config"
	"bitbucket.org/supermoneygames/forecast/pkg/forecast"
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
	Issues        []Issue `json:"issues"`
	Total         int     `json:"total"`
	NextPageToken string  `json:"nextPageToken,omitempty"`
	IsLast        bool    `json:"isLast,omitempty"`
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

	var allIssues []Issue
	var nextPageToken string

	for {
		// Use POST with JSON body
		requestBody := map[string]interface{}{
			"jql":        jql,
			"expand":     "changelog",
			"fields":     []string{"summary", "status", "issuetype", "labels", "assignee", "created", "updated", "resolution"},
			"maxResults": 100,
		}

		if nextPageToken != "" {
			requestBody["nextPageToken"] = nextPageToken
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

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("JIRA API error: %d - %s", resp.StatusCode, string(respBody))
		}

		var searchResp SearchResponse
		if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		allIssues = append(allIssues, searchResp.Issues...)

		// Check if there are more pages
		if searchResp.IsLast || searchResp.NextPageToken == "" {
			break
		}
		nextPageToken = searchResp.NextPageToken
	}

	return allIssues, nil
}

func (c *Client) convertIssue(issue Issue, cfg *config.Config) forecast.Item {
	item := forecast.Item{
		ID:          issue.Key,
		JiraKey:     issue.Key,
		Description: issue.Fields.Summary,
		Status:      issue.Fields.Status.Name,
	}

	// Parse created time - JIRA uses format like "2025-12-04T05:24:06.077-0800"
	if issue.Fields.Created != "" {
		// Try multiple formats
		formats := []string{
			"2006-01-02T15:04:05.000-0700",
			"2006-01-02T15:04:05.000Z",
			time.RFC3339,
			"2006-01-02T15:04:05.000-07:00",
		}
		for _, format := range formats {
			if created, err := time.Parse(format, issue.Fields.Created); err == nil {
				item.Created = created
				break
			}
		}
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

// parseJIRATime parses JIRA timestamp format
func parseJIRATime(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05.000Z",
		time.RFC3339,
		"2006-01-02T15:04:05.000-07:00",
	}
	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("failed to parse time: %s", s)
}

func (c *Client) extractStateTransitions(changelog *Changelog) (*time.Time, *time.Time) {
	if changelog == nil {
		return nil, nil
	}

	var inProgressTime, doneTime *time.Time

	for _, history := range changelog.Histories {
		for _, item := range history.Items {
			if item.Field == "status" {
				timestamp, err := parseJIRATime(history.Created)
				if err != nil {
					continue
				}

				// Track "In Progress" or "In Development" transition
				if (item.ToString == "In Progress" || item.ToString == "In Development") && inProgressTime == nil {
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

// CreateIssueRequest represents the request body for creating an issue
type CreateIssueRequest struct {
	Summary     string
	IssueType   string
	Description string
	Priority    string
	Labels      []string
	Assignee    string // email address
	Epic        string // parent epic key
	Project     string
}

// CreateIssueResponse represents the response from creating an issue
type CreateIssueResponse struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Self string `json:"self"`
}

// UpdateIssueRequest represents the request body for updating an issue
type UpdateIssueRequest struct {
	Summary     *string
	Description *string
	Priority    *string
	Labels      []string
	Assignee    *string // email address
}

// Transition represents a JIRA workflow transition
type Transition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	To   Status `json:"to"`
}

// TransitionsResponse represents the response from getting transitions
type TransitionsResponse struct {
	Transitions []Transition `json:"transitions"`
}

// Priority represents a JIRA priority
type Priority struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ProjectMeta represents project metadata including issue types
type ProjectMeta struct {
	Key        string      `json:"key"`
	Name       string      `json:"name"`
	IssueTypes []IssueType `json:"issueTypes"`
}

// IssueTypeWithID extends IssueType with ID
type IssueTypeWithID struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// doRequest performs an HTTP request and returns the response body
func (c *Client) doRequest(method, endpoint string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, c.baseURL+endpoint, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.email, c.apiToken)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("JIRA API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// GetIssueTypes returns available issue types for the project
func (c *Client) GetIssueTypes(projectKey string) (map[string]string, error) {
	endpoint := fmt.Sprintf("/rest/api/3/project/%s", projectKey)
	respBody, err := c.doRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var project struct {
		IssueTypes []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"issueTypes"`
	}

	if err := json.Unmarshal(respBody, &project); err != nil {
		return nil, fmt.Errorf("failed to parse project: %w", err)
	}

	result := make(map[string]string)
	for _, it := range project.IssueTypes {
		result[it.Name] = it.ID
	}

	return result, nil
}

// GetPriorities returns available priorities
func (c *Client) GetPriorities() (map[string]string, error) {
	respBody, err := c.doRequest("GET", "/rest/api/3/priority", nil)
	if err != nil {
		return nil, err
	}

	var priorities []Priority
	if err := json.Unmarshal(respBody, &priorities); err != nil {
		return nil, fmt.Errorf("failed to parse priorities: %w", err)
	}

	result := make(map[string]string)
	for _, p := range priorities {
		result[p.Name] = p.ID
	}

	return result, nil
}

// GetUserAccountID looks up a user's account ID by email
func (c *Client) GetUserAccountID(email string) (string, error) {
	endpoint := fmt.Sprintf("/rest/api/3/user/search?query=%s", email)
	respBody, err := c.doRequest("GET", endpoint, nil)
	if err != nil {
		return "", err
	}

	var users []struct {
		AccountID string `json:"accountId"`
	}

	if err := json.Unmarshal(respBody, &users); err != nil {
		return "", fmt.Errorf("failed to parse users: %w", err)
	}

	if len(users) == 0 {
		return "", fmt.Errorf("user not found: %s", email)
	}

	return users[0].AccountID, nil
}

// CreateIssue creates a new JIRA issue
func (c *Client) CreateIssue(req CreateIssueRequest) (*CreateIssueResponse, error) {
	// Get issue type ID
	issueTypes, err := c.GetIssueTypes(req.Project)
	if err != nil {
		return nil, fmt.Errorf("failed to get issue types: %w", err)
	}

	issueTypeID, ok := issueTypes[req.IssueType]
	if !ok {
		return nil, fmt.Errorf("invalid issue type '%s', available: %v", req.IssueType, mapKeys(issueTypes))
	}

	// Get priority ID
	priorities, err := c.GetPriorities()
	if err != nil {
		return nil, fmt.Errorf("failed to get priorities: %w", err)
	}

	priorityID, ok := priorities[req.Priority]
	if !ok {
		return nil, fmt.Errorf("invalid priority '%s', available: %v", req.Priority, mapKeys(priorities))
	}

	// Build request body
	fields := map[string]interface{}{
		"project":   map[string]string{"key": req.Project},
		"summary":   req.Summary,
		"issuetype": map[string]string{"id": issueTypeID},
		"priority":  map[string]string{"id": priorityID},
	}

	// Add description in Atlassian Document Format
	if req.Description != "" {
		fields["description"] = map[string]interface{}{
			"type":    "doc",
			"version": 1,
			"content": []map[string]interface{}{
				{
					"type": "paragraph",
					"content": []map[string]interface{}{
						{
							"type": "text",
							"text": req.Description,
						},
					},
				},
			},
		}
	}

	// Add labels
	if len(req.Labels) > 0 {
		fields["labels"] = req.Labels
	}

	// Add assignee
	if req.Assignee != "" {
		accountID, err := c.GetUserAccountID(req.Assignee)
		if err != nil {
			return nil, fmt.Errorf("failed to find assignee: %w", err)
		}
		fields["assignee"] = map[string]string{"accountId": accountID}
	}

	// Add parent epic
	if req.Epic != "" {
		fields["parent"] = map[string]string{"key": req.Epic}
	}

	payload := map[string]interface{}{"fields": fields}

	respBody, err := c.doRequest("POST", "/rest/api/3/issue", payload)
	if err != nil {
		return nil, err
	}

	var result CreateIssueResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// UpdateIssue updates an existing JIRA issue
func (c *Client) UpdateIssue(issueKey string, req UpdateIssueRequest) error {
	fields := make(map[string]interface{})

	if req.Summary != nil {
		fields["summary"] = *req.Summary
	}

	if req.Description != nil {
		fields["description"] = map[string]interface{}{
			"type":    "doc",
			"version": 1,
			"content": []map[string]interface{}{
				{
					"type": "paragraph",
					"content": []map[string]interface{}{
						{
							"type": "text",
							"text": *req.Description,
						},
					},
				},
			},
		}
	}

	if req.Priority != nil {
		priorities, err := c.GetPriorities()
		if err != nil {
			return fmt.Errorf("failed to get priorities: %w", err)
		}
		priorityID, ok := priorities[*req.Priority]
		if !ok {
			return fmt.Errorf("invalid priority '%s'", *req.Priority)
		}
		fields["priority"] = map[string]string{"id": priorityID}
	}

	if req.Labels != nil {
		fields["labels"] = req.Labels
	}

	if req.Assignee != nil {
		accountID, err := c.GetUserAccountID(*req.Assignee)
		if err != nil {
			return fmt.Errorf("failed to find assignee: %w", err)
		}
		fields["assignee"] = map[string]string{"accountId": accountID}
	}

	if len(fields) == 0 {
		return fmt.Errorf("no fields to update")
	}

	payload := map[string]interface{}{"fields": fields}

	_, err := c.doRequest("PUT", fmt.Sprintf("/rest/api/3/issue/%s", issueKey), payload)
	return err
}

// GetIssue retrieves a single issue by key
func (c *Client) GetIssue(issueKey string) (*Issue, error) {
	endpoint := fmt.Sprintf("/rest/api/3/issue/%s", issueKey)
	respBody, err := c.doRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var issue Issue
	if err := json.Unmarshal(respBody, &issue); err != nil {
		return nil, fmt.Errorf("failed to parse issue: %w", err)
	}

	return &issue, nil
}

// GetTransitions returns available transitions for an issue
func (c *Client) GetTransitions(issueKey string) ([]Transition, error) {
	endpoint := fmt.Sprintf("/rest/api/3/issue/%s/transitions", issueKey)
	respBody, err := c.doRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var result TransitionsResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse transitions: %w", err)
	}

	return result.Transitions, nil
}

// TransitionIssue moves an issue to a new status
func (c *Client) TransitionIssue(issueKey, transitionName string, comment string) error {
	// Get available transitions
	transitions, err := c.GetTransitions(issueKey)
	if err != nil {
		return fmt.Errorf("failed to get transitions: %w", err)
	}

	// Find matching transition by name or target status
	var transitionID string
	for _, t := range transitions {
		if t.Name == transitionName || t.To.Name == transitionName {
			transitionID = t.ID
			break
		}
	}

	if transitionID == "" {
		available := make([]string, 0, len(transitions))
		for _, t := range transitions {
			available = append(available, fmt.Sprintf("%s -> %s", t.Name, t.To.Name))
		}
		return fmt.Errorf("transition '%s' not available, options: %v", transitionName, available)
	}

	payload := map[string]interface{}{
		"transition": map[string]string{"id": transitionID},
	}

	// Add comment if provided
	if comment != "" {
		payload["update"] = map[string]interface{}{
			"comment": []map[string]interface{}{
				{
					"add": map[string]interface{}{
						"body": map[string]interface{}{
							"type":    "doc",
							"version": 1,
							"content": []map[string]interface{}{
								{
									"type": "paragraph",
									"content": []map[string]interface{}{
										{
											"type": "text",
											"text": comment,
										},
									},
								},
							},
						},
					},
				},
			},
		}
	}

	_, err = c.doRequest("POST", fmt.Sprintf("/rest/api/3/issue/%s/transitions", issueKey), payload)
	return err
}

// AddComment adds a comment to an issue
func (c *Client) AddComment(issueKey, comment string) error {
	payload := map[string]interface{}{
		"body": map[string]interface{}{
			"type":    "doc",
			"version": 1,
			"content": []map[string]interface{}{
				{
					"type": "paragraph",
					"content": []map[string]interface{}{
						{
							"type": "text",
							"text": comment,
						},
					},
				},
			},
		},
	}

	_, err := c.doRequest("POST", fmt.Sprintf("/rest/api/3/issue/%s/comment", issueKey), payload)
	return err
}

// SearchJQL searches for issues using JQL and returns raw issues
func (c *Client) SearchJQL(jql string) ([]Issue, error) {
	return c.search(jql)
}

// Helper function to get map keys
func mapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
