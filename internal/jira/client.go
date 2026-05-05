package jira

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
		Summary     string                 `json:"summary"`
		Status      Status                 `json:"status"`
		IssueType   IssueType              `json:"issuetype"`
		Labels      []string               `json:"labels"`
		Assignee    *User                  `json:"assignee"`
		Created     string                 `json:"created"`
		Updated     string                 `json:"updated"`
		Resolution  *Resolution            `json:"resolution"`
		TimeSpent   int                    `json:"timespent"`   // Built-in time tracking (seconds)
		Description interface{}            `json:"description"` // Raw ADF — render with RenderADF
		IssueLinks  []IssueLink            `json:"issuelinks"`
		Attachment  []Attachment           `json:"attachment"`
		Custom      map[string]interface{} `json:"-"`           // For custom fields
	} `json:"fields"`
	RawFields map[string]interface{} `json:"-"` // Raw fields for custom field access
	Changelog *Changelog             `json:"changelog,omitempty"`
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

// IssueLink represents a link between two JIRA issues.
type IssueLink struct {
	ID           string         `json:"id"`
	Type         IssueLinkType  `json:"type"`
	InwardIssue  *LinkedIssue   `json:"inwardIssue,omitempty"`
	OutwardIssue *LinkedIssue   `json:"outwardIssue,omitempty"`
}

type IssueLinkType struct {
	Name    string `json:"name"`
	Inward  string `json:"inward"`
	Outward string `json:"outward"`
}

type LinkedIssue struct {
	Key    string `json:"key"`
	Fields struct {
		Summary string `json:"summary"`
		Status  Status `json:"status"`
	} `json:"fields"`
}

// Attachment represents a file attached to an issue.
type Attachment struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	MimeType string `json:"mimeType"`
	Size     int64  `json:"size"`
	Created  string `json:"created"`
	Author   *User  `json:"author"`
	Content  string `json:"content"` // Direct download URL
}

type Changelog struct {
	Histories []History `json:"histories"`
}

type History struct {
	Created string       `json:"created"`
	Author  *User        `json:"author"`
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

	// Include custom fields if configured
	var extraFields []string
	if cfg.JIRA.CycleTimeField != "" {
		extraFields = append(extraFields, cfg.JIRA.CycleTimeField)
	}
	if cfg.JIRA.StoryPointsField != "" {
		extraFields = append(extraFields, cfg.JIRA.StoryPointsField)
	}

	// Make API request
	issues, err := c.searchWithFields(jql, extraFields)
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
	return c.searchWithFields(jql, nil)
}

func (c *Client) searchWithFields(jql string, extraFields []string) ([]Issue, error) {
	return c.searchWithFieldsInternal(jql, extraFields, false)
}

// SearchWithAllFields searches and returns all fields (for field discovery)
func (c *Client) SearchWithAllFields(jql string) ([]Issue, error) {
	return c.searchWithFieldsInternal(jql, nil, true)
}

func (c *Client) searchWithFieldsInternal(jql string, extraFields []string, allFields bool) ([]Issue, error) {
	u := fmt.Sprintf("%s/rest/api/3/search/jql", c.baseURL)

	var allIssues []Issue
	var nextPageToken string

	// Base fields to fetch
	var fields interface{}
	if allFields {
		fields = []string{"*all"}
	} else {
		f := []string{"summary", "status", "issuetype", "labels", "assignee", "created", "updated", "resolution", "timespent"}
		f = append(f, extraFields...)
		fields = f
	}

	for {
		// Use POST with JSON body
		requestBody := map[string]interface{}{
			"jql":        jql,
			"expand":     "changelog",
			"fields":     fields,
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

		// Decode into raw structure first to capture custom fields
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		var searchResp SearchResponse
		if err := json.Unmarshal(respBody, &searchResp); err != nil {
			return nil, err
		}

		// Also decode raw to get custom fields
		var rawResp struct {
			Issues []struct {
				Key    string                 `json:"key"`
				Fields map[string]interface{} `json:"fields"`
			} `json:"issues"`
		}
		json.Unmarshal(respBody, &rawResp)

		// Merge raw fields into issues
		for i := range searchResp.Issues {
			if i < len(rawResp.Issues) {
				searchResp.Issues[i].RawFields = rawResp.Issues[i].Fields
			}
		}

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
	doneStatuses := cfg.JIRA.EffectiveDoneStatuses()
	inProgressStatuses := cfg.JIRA.EffectiveInProgressStatuses()
	rawStatus := issue.Fields.Status.Name
	normalizedStatus := rawStatus
	switch {
	case isStatusIn(rawStatus, doneStatuses):
		normalizedStatus = "Done"
	case isStatusIn(rawStatus, inProgressStatuses):
		normalizedStatus = "In Progress"
	}
	item := forecast.Item{
		ID:          issue.Key,
		JiraKey:     issue.Key,
		Description: issue.Fields.Summary,
		Status:      normalizedStatus,
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
	inProgressTime, doneTime := c.extractStateTransitions(issue.Changelog, inProgressStatuses, doneStatuses)
	if inProgressTime != nil {
		item.InProgress = inProgressTime
	}
	if doneTime != nil {
		item.Done = doneTime
		if inProgressTime != nil {
			item.CycleTime = doneTime.Sub(*inProgressTime).Hours()
		}
	}

	// Check for cycle time overrides (custom field takes priority, then TimeSpent)
	// 1. Custom field override (if configured)
	if cfg.JIRA.CycleTimeField != "" && issue.RawFields != nil {
		if val, ok := issue.RawFields[cfg.JIRA.CycleTimeField]; ok && val != nil {
			// Custom field value could be a number (hours)
			switch v := val.(type) {
			case float64:
				if v > 0 {
					item.CycleTime = v
				}
			case int:
				if v > 0 {
					item.CycleTime = float64(v)
				}
			}
		}
	}

	// 2. JIRA TimeSpent field override (if no cycle time yet and TimeSpent is set)
	if item.CycleTime == 0 && issue.Fields.TimeSpent > 0 {
		// TimeSpent is in seconds, convert to hours
		item.CycleTime = float64(issue.Fields.TimeSpent) / 3600.0
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

// isStatusIn reports whether status is in the supplied set.
// Comparison is exact (case-sensitive) to match raw Jira status names.
func isStatusIn(status string, statuses []string) bool {
	for _, s := range statuses {
		if s == status {
			return true
		}
	}
	return false
}

func (c *Client) extractStateTransitions(changelog *Changelog, inProgressStatuses, doneStatuses []string) (*time.Time, *time.Time) {
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

				// Track first transition into any in-progress-equivalent status
				// (e.g. "In Progress" or "In Development").
				if isStatusIn(item.ToString, inProgressStatuses) && inProgressTime == nil {
					inProgressTime = &timestamp
				}

				// Track first transition into any done-equivalent status.
				// "Awaiting Dev Deployment" can be treated as done via config so
				// dev-complete tickets contribute cycle-time samples to Monte Carlo
				// before the deploy gate clears.
				if isStatusIn(item.ToString, doneStatuses) && doneTime == nil {
					doneTime = &timestamp
				}
			}
		}
	}

	return inProgressTime, doneTime
}

// ExtractClosedBy returns the name of the person who transitioned the issue to Done
func (c *Client) ExtractClosedBy(changelog *Changelog) string {
	if changelog == nil {
		return ""
	}

	for _, history := range changelog.Histories {
		for _, item := range history.Items {
			if item.Field == "status" && item.ToString == "Done" {
				if history.Author != nil {
					return history.Author.DisplayName
				}
				return ""
			}
		}
	}

	return ""
}

// CreateIssueRequest represents the request body for creating an issue
type CreateIssueRequest struct {
	Summary          string
	IssueType        string
	Description      string
	Priority         string
	Labels           []string
	Assignee         string  // email address
	Epic             string  // parent epic key (alias for Parent — kept for backward compat)
	Parent           string  // parent issue key (epic or sub-task parent)
	Project          string
	StoryPoints      float64  // 0 = omit
	DueDate          string   // YYYY-MM-DD; empty = omit
	StoryPointsField string   // customfield_XXXXX for story points; if empty StoryPoints is ignored
	FixVersions      []string // version names; empty = omit
	Components       []string // component names; empty = omit
}

// CreateIssueResponse represents the response from creating an issue
type CreateIssueResponse struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Self string `json:"self"`
}

// UpdateIssueRequest represents the request body for updating an issue
type UpdateIssueRequest struct {
	Summary          *string
	Description      *string
	Priority         *string
	Labels           []string
	Assignee         *string  // email address
	Epic             *string  // parent epic key (alias for Parent)
	Parent           *string  // parent issue key (epic or sub-task parent)
	StoryPoints      *float64 // nil = no change
	DueDate          *string  // YYYY-MM-DD; nil = no change, "" = clear
	StoryPointsField string   // customfield_XXXXX for story points; required if StoryPoints set
	FixVersions      []string // version names; nil/empty = no change
	Components       []string // component names; nil/empty = no change
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

// Project represents a JIRA project
type Project struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

// GetProjects returns all accessible JIRA projects
func (c *Client) GetProjects() ([]Project, error) {
	respBody, err := c.doRequest("GET", "/rest/api/3/project", nil)
	if err != nil {
		return nil, err
	}

	var projects []Project
	if err := json.Unmarshal(respBody, &projects); err != nil {
		return nil, fmt.Errorf("failed to parse projects: %w", err)
	}

	return projects, nil
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

// parseDescriptionToADF converts a plain text description with simple markup to ADF format.
// Supported markup:
//   - Lines starting with "## " become h2 headings
//   - Lines starting with "### " become h3 headings
//   - Lines starting with "- " become bullet list items
//   - Lines starting with "* " become bullet list items
//   - Text wrapped in *asterisks* becomes bold
//   - Empty lines separate paragraphs
func parseDescriptionToADF(desc string) map[string]interface{} {
	lines := strings.Split(desc, "\n")
	content := []map[string]interface{}{}

	var currentList []map[string]interface{}
	flushList := func() {
		if len(currentList) > 0 {
			content = append(content, map[string]interface{}{
				"type":    "bulletList",
				"content": currentList,
			})
			currentList = nil
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Empty line - flush any pending list
		if trimmed == "" {
			flushList()
			continue
		}

		// H2 heading
		if strings.HasPrefix(trimmed, "## ") {
			flushList()
			headingText := strings.TrimPrefix(trimmed, "## ")
			content = append(content, map[string]interface{}{
				"type":  "heading",
				"attrs": map[string]interface{}{"level": 2},
				"content": []map[string]interface{}{
					{"type": "text", "text": headingText},
				},
			})
			continue
		}

		// H3 heading
		if strings.HasPrefix(trimmed, "### ") {
			flushList()
			headingText := strings.TrimPrefix(trimmed, "### ")
			content = append(content, map[string]interface{}{
				"type":  "heading",
				"attrs": map[string]interface{}{"level": 3},
				"content": []map[string]interface{}{
					{"type": "text", "text": headingText},
				},
			})
			continue
		}

		// Bullet list item (- or *)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			itemText := strings.TrimPrefix(strings.TrimPrefix(trimmed, "- "), "* ")
			listItem := map[string]interface{}{
				"type": "listItem",
				"content": []map[string]interface{}{
					{
						"type":    "paragraph",
						"content": parseInlineText(itemText),
					},
				},
			}
			currentList = append(currentList, listItem)
			continue
		}

		// Regular paragraph
		flushList()
		content = append(content, map[string]interface{}{
			"type":    "paragraph",
			"content": parseInlineText(trimmed),
		})
	}

	// Flush any remaining list
	flushList()

	return map[string]interface{}{
		"type":    "doc",
		"version": 1,
		"content": content,
	}
}

// parseInlineText converts inline markup like *bold* to ADF text nodes
func parseInlineText(text string) []map[string]interface{} {
	var result []map[string]interface{}

	// Simple regex-free parsing for *bold* text
	i := 0
	for i < len(text) {
		// Look for opening *
		start := strings.Index(text[i:], "*")
		if start == -1 {
			// No more asterisks, add remaining text
			if i < len(text) {
				result = append(result, map[string]interface{}{
					"type": "text",
					"text": text[i:],
				})
			}
			break
		}

		// Add text before the asterisk
		if start > 0 {
			result = append(result, map[string]interface{}{
				"type": "text",
				"text": text[i : i+start],
			})
		}

		// Look for closing *
		end := strings.Index(text[i+start+1:], "*")
		if end == -1 {
			// No closing asterisk, treat as literal
			result = append(result, map[string]interface{}{
				"type": "text",
				"text": text[i+start:],
			})
			break
		}

		// Add bold text
		boldText := text[i+start+1 : i+start+1+end]
		result = append(result, map[string]interface{}{
			"type": "text",
			"text": boldText,
			"marks": []map[string]interface{}{
				{"type": "strong"},
			},
		})

		i = i + start + 1 + end + 1
	}

	// If no content was added, ensure we have at least empty text
	if len(result) == 0 {
		result = append(result, map[string]interface{}{
			"type": "text",
			"text": text,
		})
	}

	return result
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

	// Add description in Atlassian Document Format (with markdown parsing)
	if req.Description != "" {
		fields["description"] = parseDescriptionToADF(req.Description)
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

	// Add parent (epic or sub-task parent). Parent takes precedence over Epic.
	parentKey := req.Parent
	if parentKey == "" {
		parentKey = req.Epic
	}
	if parentKey != "" {
		fields["parent"] = map[string]string{"key": parentKey}
	}

	// Story points (custom field)
	if req.StoryPoints > 0 && req.StoryPointsField != "" {
		fields[req.StoryPointsField] = req.StoryPoints
	}

	// Due date (standard field)
	if req.DueDate != "" {
		fields["duedate"] = req.DueDate
	}

	// Fix versions
	if len(req.FixVersions) > 0 {
		fields["fixVersions"] = namesArray(req.FixVersions)
	}

	// Components
	if len(req.Components) > 0 {
		fields["components"] = namesArray(req.Components)
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
		fields["description"] = parseDescriptionToADF(*req.Description)
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

	if req.Parent != nil {
		fields["parent"] = map[string]string{"key": *req.Parent}
	} else if req.Epic != nil {
		fields["parent"] = map[string]string{"key": *req.Epic}
	}

	if req.StoryPoints != nil && req.StoryPointsField != "" {
		fields[req.StoryPointsField] = *req.StoryPoints
	}

	if req.DueDate != nil {
		// Empty string clears the field; non-empty sets it.
		if *req.DueDate == "" {
			fields["duedate"] = nil
		} else {
			fields["duedate"] = *req.DueDate
		}
	}

	if len(req.FixVersions) > 0 {
		fields["fixVersions"] = namesArray(req.FixVersions)
	}

	if len(req.Components) > 0 {
		fields["components"] = namesArray(req.Components)
	}

	if len(fields) == 0 {
		return fmt.Errorf("no fields to update")
	}

	payload := map[string]interface{}{"fields": fields}

	_, err := c.doRequest("PUT", fmt.Sprintf("/rest/api/3/issue/%s", issueKey), payload)
	return err
}

// DownloadAttachment fetches an attachment's bytes by ID and writes them to dst.
// dst may be a file path, or "-" for stdout.
func (c *Client) DownloadAttachment(attachmentID, dst string) (int64, error) {
	endpoint := fmt.Sprintf("%s/rest/api/3/attachment/content/%s", c.baseURL, attachmentID)
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return 0, err
	}
	req.SetBasicAuth(c.email, c.apiToken)
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("download failed %d: %s", resp.StatusCode, string(body))
	}
	var w io.Writer
	if dst == "-" {
		w = os.Stdout
	} else {
		f, err := os.Create(dst)
		if err != nil {
			return 0, fmt.Errorf("create file: %w", err)
		}
		defer f.Close()
		w = f
	}
	return io.Copy(w, resp.Body)
}

// UploadAttachment uploads a local file as an attachment on the given issue.
func (c *Client) UploadAttachment(issueKey, filePath string) (*Attachment, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("create multipart: %w", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, fmt.Errorf("copy file: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart: %w", err)
	}

	endpoint := fmt.Sprintf("%s/rest/api/3/issue/%s/attachments", c.baseURL, issueKey)
	req, err := http.NewRequest("POST", endpoint, &body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.email, c.apiToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Atlassian-Token", "no-check")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upload failed %d: %s", resp.StatusCode, string(respBody))
	}
	// JIRA returns an array with the uploaded attachment(s)
	var atts []Attachment
	if err := json.Unmarshal(respBody, &atts); err != nil {
		return nil, fmt.Errorf("parse upload response: %w", err)
	}
	if len(atts) == 0 {
		return nil, fmt.Errorf("upload succeeded but no attachment returned")
	}
	return &atts[0], nil
}

// UpdateLabels adds and/or removes labels on an issue using JIRA's update
// operations (atomic, no read-modify-write).
func (c *Client) UpdateLabels(issueKey string, add, remove []string) error {
	if len(add) == 0 && len(remove) == 0 {
		return fmt.Errorf("no labels to add or remove")
	}
	ops := make([]map[string]string, 0, len(add)+len(remove))
	for _, l := range add {
		ops = append(ops, map[string]string{"add": l})
	}
	for _, l := range remove {
		ops = append(ops, map[string]string{"remove": l})
	}
	payload := map[string]interface{}{
		"update": map[string]interface{}{"labels": ops},
	}
	_, err := c.doRequest("PUT", fmt.Sprintf("/rest/api/3/issue/%s", issueKey), payload)
	return err
}

// GetIssue retrieves a single issue by key, including changelog and issue links.
func (c *Client) GetIssue(issueKey string) (*Issue, error) {
	endpoint := fmt.Sprintf("/rest/api/3/issue/%s?expand=changelog", issueKey)
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

// TransitionOptions controls a transition. Comment and Resolution are optional.
type TransitionOptions struct {
	Name       string // transition name or target status
	Comment    string
	Resolution string // sets fields.resolution.name (e.g. "Done", "Won't Do")
}

// TransitionIssue moves an issue to a new status, optionally adding a comment
// and/or setting a resolution.
func (c *Client) TransitionIssue(issueKey string, opts TransitionOptions) error {
	transitions, err := c.GetTransitions(issueKey)
	if err != nil {
		return fmt.Errorf("failed to get transitions: %w", err)
	}

	var transitionID string
	for _, t := range transitions {
		if t.Name == opts.Name || t.To.Name == opts.Name {
			transitionID = t.ID
			break
		}
	}
	if transitionID == "" {
		available := make([]string, 0, len(transitions))
		for _, t := range transitions {
			available = append(available, fmt.Sprintf("%s -> %s", t.Name, t.To.Name))
		}
		return fmt.Errorf("transition '%s' not available, options: %v", opts.Name, available)
	}

	payload := map[string]interface{}{
		"transition": map[string]string{"id": transitionID},
	}

	if opts.Resolution != "" {
		payload["fields"] = map[string]interface{}{
			"resolution": map[string]string{"name": opts.Resolution},
		}
	}

	if opts.Comment != "" {
		payload["update"] = map[string]interface{}{
			"comment": []map[string]interface{}{
				{"add": map[string]interface{}{"body": parseDescriptionToADF(opts.Comment)}},
			},
		}
	}

	_, err = c.doRequest("POST", fmt.Sprintf("/rest/api/3/issue/%s/transitions", issueKey), payload)
	return err
}

// GetResolutions returns all configured resolutions.
func (c *Client) GetResolutions() ([]struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}, error) {
	respBody, err := c.doRequest("GET", "/rest/api/3/resolution", nil)
	if err != nil {
		return nil, err
	}
	var resolutions []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(respBody, &resolutions); err != nil {
		return nil, fmt.Errorf("failed to parse resolutions: %w", err)
	}
	return resolutions, nil
}

// Comment represents a JIRA comment with author and timestamps.
type Comment struct {
	ID      string      `json:"id"`
	Author  *User       `json:"author"`
	Created string      `json:"created"`
	Updated string      `json:"updated"`
	Body    interface{} `json:"body"` // Raw ADF — render with RenderADF
}

// CommentsResponse is the paginated comments envelope.
type CommentsResponse struct {
	Comments   []Comment `json:"comments"`
	StartAt    int       `json:"startAt"`
	MaxResults int       `json:"maxResults"`
	Total      int       `json:"total"`
}

// GetComments returns all comments on an issue, oldest first.
func (c *Client) GetComments(issueKey string) ([]Comment, error) {
	var all []Comment
	startAt := 0
	for {
		endpoint := fmt.Sprintf("/rest/api/3/issue/%s/comment?startAt=%d&maxResults=100&orderBy=created", issueKey, startAt)
		respBody, err := c.doRequest("GET", endpoint, nil)
		if err != nil {
			return nil, err
		}
		var page CommentsResponse
		if err := json.Unmarshal(respBody, &page); err != nil {
			return nil, fmt.Errorf("failed to parse comments: %w", err)
		}
		all = append(all, page.Comments...)
		if startAt+len(page.Comments) >= page.Total || len(page.Comments) == 0 {
			break
		}
		startAt += len(page.Comments)
	}
	return all, nil
}

// WatchersResponse is the response from GET /watchers.
type WatchersResponse struct {
	WatchCount int    `json:"watchCount"`
	IsWatching bool   `json:"isWatching"`
	Watchers   []User `json:"watchers"`
}

// GetWatchers returns the watchers on an issue.
func (c *Client) GetWatchers(issueKey string) (*WatchersResponse, error) {
	respBody, err := c.doRequest("GET", fmt.Sprintf("/rest/api/3/issue/%s/watchers", issueKey), nil)
	if err != nil {
		return nil, err
	}
	var resp WatchersResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse watchers: %w", err)
	}
	return &resp, nil
}

// AddWatcher adds a watcher to an issue. If accountID is empty, the
// authenticated user is added.
func (c *Client) AddWatcher(issueKey, accountID string) error {
	endpoint := fmt.Sprintf("/rest/api/3/issue/%s/watchers", issueKey)
	// JIRA expects the body to be the accountId as a raw JSON string. Empty body
	// adds the calling user.
	var body interface{}
	if accountID != "" {
		body = accountID
	}
	_, err := c.doRequest("POST", endpoint, body)
	return err
}

// RemoveWatcher removes a watcher. accountID is required by the JIRA API.
func (c *Client) RemoveWatcher(issueKey, accountID string) error {
	endpoint := fmt.Sprintf("/rest/api/3/issue/%s/watchers?accountId=%s", issueKey, url.QueryEscape(accountID))
	_, err := c.doRequest("DELETE", endpoint, nil)
	return err
}

// LinkIssues creates a link between two issues.
// The relationship reads: outwardKey [type.outward] inwardKey
// e.g. for type "Blocks": outwardKey blocks inwardKey.
func (c *Client) LinkIssues(outwardKey, inwardKey, typeName string) error {
	payload := map[string]interface{}{
		"type":         map[string]string{"name": typeName},
		"outwardIssue": map[string]string{"key": outwardKey},
		"inwardIssue":  map[string]string{"key": inwardKey},
	}
	_, err := c.doRequest("POST", "/rest/api/3/issueLink", payload)
	return err
}

// DeleteIssueLink removes an issue link by its ID.
func (c *Client) DeleteIssueLink(linkID string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/rest/api/3/issueLink/%s", linkID), nil)
	return err
}

// GetIssueLinkTypes returns all configured issue link types.
func (c *Client) GetIssueLinkTypes() ([]IssueLinkType, error) {
	respBody, err := c.doRequest("GET", "/rest/api/3/issueLinkType", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		IssueLinkTypes []IssueLinkType `json:"issueLinkTypes"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse link types: %w", err)
	}
	return resp.IssueLinkTypes, nil
}

// AddComment adds a comment to an issue. The body supports the same simple
// markdown subset as descriptions (## headings, - bullets, *bold*).
func (c *Client) AddComment(issueKey, comment string) error {
	payload := map[string]interface{}{
		"body": parseDescriptionToADF(comment),
	}
	_, err := c.doRequest("POST", fmt.Sprintf("/rest/api/3/issue/%s/comment", issueKey), payload)
	return err
}

// SearchJQL searches for issues using JQL and returns raw issues
func (c *Client) SearchJQL(jql string) ([]Issue, error) {
	return c.search(jql)
}

// ConvertIssuesToItems converts JIRA issues to forecast items
func (c *Client) ConvertIssuesToItems(issues []Issue, cfg *config.Config) []forecast.Item {
	items := make([]forecast.Item, 0, len(issues))
	for _, issue := range issues {
		item := c.convertIssue(issue, cfg)
		items = append(items, item)
	}
	return items
}

// LogWork adds a worklog entry to a JIRA issue
func (c *Client) LogWork(issueKey string, timeSpentSeconds int, comment string) error {
	payload := map[string]interface{}{
		"timeSpentSeconds": timeSpentSeconds,
	}

	if comment != "" {
		payload["comment"] = map[string]interface{}{
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
		}
	}

	_, err := c.doRequest("POST", fmt.Sprintf("/rest/api/3/issue/%s/worklog", issueKey), payload)
	return err
}

// Board represents an Agile board.
type Board struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// Sprint represents an Agile sprint.
type Sprint struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	State         string `json:"state"` // active, closed, future
	StartDate     string `json:"startDate"`
	EndDate       string `json:"endDate"`
	CompleteDate  string `json:"completeDate"`
	OriginBoardID int    `json:"originBoardId"`
	Goal          string `json:"goal"`
}

// GetBoards returns boards visible to the user, optionally filtered by project key.
func (c *Client) GetBoards(projectKey string) ([]Board, error) {
	endpoint := "/rest/agile/1.0/board"
	if projectKey != "" {
		endpoint += "?projectKeyOrId=" + url.QueryEscape(projectKey)
	}
	respBody, err := c.doRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Values []Board `json:"values"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse boards: %w", err)
	}
	return resp.Values, nil
}

// GetSprints returns sprints on a board. State filters: "active", "closed",
// "future" (comma-separated). Empty returns all states.
func (c *Client) GetSprints(boardID int, state string) ([]Sprint, error) {
	endpoint := fmt.Sprintf("/rest/agile/1.0/board/%d/sprint", boardID)
	if state != "" {
		endpoint += "?state=" + url.QueryEscape(state)
	}
	respBody, err := c.doRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Values []Sprint `json:"values"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse sprints: %w", err)
	}
	return resp.Values, nil
}

// MoveIssuesToSprint adds the given issue keys to a sprint.
func (c *Client) MoveIssuesToSprint(sprintID int, keys []string) error {
	endpoint := fmt.Sprintf("/rest/agile/1.0/sprint/%d/issue", sprintID)
	_, err := c.doRequest("POST", endpoint, map[string]interface{}{"issues": keys})
	return err
}

// MoveIssuesToBacklog removes the given issues from any sprint.
func (c *Client) MoveIssuesToBacklog(keys []string) error {
	_, err := c.doRequest("POST", "/rest/agile/1.0/backlog/issue", map[string]interface{}{"issues": keys})
	return err
}

// Worklog represents a worklog entry on an issue.
type Worklog struct {
	ID               string      `json:"id"`
	Author           *User       `json:"author"`
	Created          string      `json:"created"`
	Started          string      `json:"started"`
	TimeSpent        string      `json:"timeSpent"`
	TimeSpentSeconds int         `json:"timeSpentSeconds"`
	Comment          interface{} `json:"comment"` // Raw ADF
}

// WorklogResponse is the paginated worklogs envelope.
type WorklogResponse struct {
	Worklogs   []Worklog `json:"worklogs"`
	StartAt    int       `json:"startAt"`
	MaxResults int       `json:"maxResults"`
	Total      int       `json:"total"`
}

// GetWorklogs returns worklog entries for an issue, oldest first.
func (c *Client) GetWorklogs(issueKey string) ([]Worklog, error) {
	var all []Worklog
	startAt := 0
	for {
		endpoint := fmt.Sprintf("/rest/api/3/issue/%s/worklog?startAt=%d&maxResults=100", issueKey, startAt)
		respBody, err := c.doRequest("GET", endpoint, nil)
		if err != nil {
			return nil, err
		}
		var page WorklogResponse
		if err := json.Unmarshal(respBody, &page); err != nil {
			return nil, fmt.Errorf("failed to parse worklogs: %w", err)
		}
		all = append(all, page.Worklogs...)
		if startAt+len(page.Worklogs) >= page.Total || len(page.Worklogs) == 0 {
			break
		}
		startAt += len(page.Worklogs)
	}
	return all, nil
}

// GetStoryPoints extracts story points from an issue's raw fields
func GetStoryPoints(rawFields map[string]interface{}, storyPointsField string) float64 {
	if rawFields == nil || storyPointsField == "" {
		return 0
	}

	if val, ok := rawFields[storyPointsField]; ok && val != nil {
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		}
	}

	return 0
}

// FieldDefinition represents a JIRA field definition
type FieldDefinition struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Custom bool   `json:"custom"`
}

// GetFields returns all field definitions from JIRA
func (c *Client) GetFields() ([]FieldDefinition, error) {
	respBody, err := c.doRequest("GET", "/rest/api/3/field", nil)
	if err != nil {
		return nil, err
	}

	var fields []FieldDefinition
	if err := json.Unmarshal(respBody, &fields); err != nil {
		return nil, fmt.Errorf("failed to parse fields: %w", err)
	}

	return fields, nil
}

// namesArray converts a list of names into the [{name: "..."}] form JIRA expects
// for fixVersions, components, etc.
func namesArray(names []string) []map[string]string {
	out := make([]map[string]string, 0, len(names))
	for _, n := range names {
		out = append(out, map[string]string{"name": n})
	}
	return out
}

// Helper function to get map keys
func mapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
