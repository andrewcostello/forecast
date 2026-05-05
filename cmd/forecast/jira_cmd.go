package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/andrewcostello/forecast/internal/config"
	"github.com/andrewcostello/forecast/internal/jira"
	"github.com/spf13/cobra"
)

// JIRA management commands
var jiraCmd = &cobra.Command{
	Use:   "jira",
	Short: "JIRA ticket management",
	Long: `Manage JIRA tickets across the full lifecycle, grouped by phase.

Read & search:
  get [--history]      Ticket details (summary, description, links, optional changelog)
  search <jql>         JQL search
  comments <key>       Read comments (rendered from ADF to Markdown)
  worklogs <key>       Read worklog history with totals
  watchers <key>       List watchers
  attachments <key>    List attachments

Create & edit:
  create               Create ticket (--story-points, --due-date, --parent for sub-tasks,
                       --fix-versions, --components)
  update <key>         Update fields on an existing ticket
  comment <key>        Add a comment (markdown subset: ## headings, - bullets, *bold*)
  attach <key> <file>  Upload an attachment
  download <id>        Download an attachment by ID

Workflow & collaboration:
  transition <key>     Move to a new status (--comment, --resolution)
  log <key>            Log work (--time 2h / 1h30m / 1.5h)
  link <from>          Create issue link (--to <key> --type Blocks|Relates|...)
  unlink <link-id>     Delete an issue link
  watch / unwatch      Add or remove a watcher

Sprint & release:
  boards               List Agile boards for the configured project
  sprints              List sprints (auto-discovers single board, or --board <id>)
  sprint-add <id> ...  Add issues to a sprint
  sprint-backlog ...   Move issues out of any sprint

Bulk:
  bulk transition --jql ... --to ...   Apply transition over a JQL query (--dry-run)
  bulk label --jql ... --add ... --remove ...

Discovery:
  types / priorities / resolutions / link-types / projects
  fields <key>          Show custom field IDs (find story_points_field, etc.)
  transitions <key>     Show valid transitions out of a ticket's current status
  missing-times         Audit Done tickets without cycle-time data; --fix to backfill

Run "forecast jira <cmd> --help" for the full flag set on each subcommand.`,
}

var jiraCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new JIRA ticket",
	Long: `Create a new JIRA ticket with the specified details.

Examples:
  forecast jira create --summary "Fix login bug" --type Bug --priority High
  forecast jira create --summary "Add dark mode" --type Story --labels ui,feature
  forecast jira create --summary "Subtask" --type Sub-task --parent SMG-1234`,
	RunE: func(cmd *cobra.Command, args []string) error {
		labelsStr, _ := cmd.Flags().GetString("labels")
		var labels []string
		if labelsStr != "" {
			labels = splitAndTrim(labelsStr, ",")
		}
		opts := jiraCreateOpts{
			Summary:     mustGetString(cmd, "summary"),
			IssueType:   mustGetString(cmd, "type"),
			Description: mustGetString(cmd, "description"),
			Priority:    mustGetString(cmd, "priority"),
			Labels:      labels,
			Assignee:    mustGetString(cmd, "assignee"),
			Epic:        mustGetString(cmd, "epic"),
			Parent:      mustGetString(cmd, "parent"),
			StoryPoints: mustGetFloat(cmd, "story-points"),
			DueDate:     mustGetString(cmd, "due-date"),
			FixVersions: splitAndTrim(mustGetString(cmd, "fix-versions"), ","),
			Components:  splitAndTrim(mustGetString(cmd, "components"), ","),
		}
		return runJiraCreate(opts)
	},
}

var jiraUpdateCmd = &cobra.Command{
	Use:   "update [issue-key]",
	Short: "Update an existing JIRA ticket",
	Long: `Update an existing JIRA ticket with new values.

Examples:
  forecast jira update SMG-1234 --priority Highest
  forecast jira update SMG-1234 --labels security,urgent
  forecast jira update SMG-1234 --story-points 5 --due-date 2026-06-30`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		labelsStr, _ := cmd.Flags().GetString("labels")
		var labels []string
		if labelsStr != "" {
			labels = splitAndTrim(labelsStr, ",")
		}
		opts := jiraUpdateOpts{
			Key:          args[0],
			Summary:      mustGetString(cmd, "summary"),
			Description:  mustGetString(cmd, "description"),
			Priority:     mustGetString(cmd, "priority"),
			Labels:       labels,
			Assignee:     mustGetString(cmd, "assignee"),
			Epic:         mustGetString(cmd, "epic"),
			Parent:       mustGetString(cmd, "parent"),
			StoryPoints:  mustGetFloat(cmd, "story-points"),
			DueDate:      mustGetString(cmd, "due-date"),
			ClearDueDate: mustGetBool(cmd, "clear-due-date"),
			FixVersions:  splitAndTrim(mustGetString(cmd, "fix-versions"), ","),
			Components:   splitAndTrim(mustGetString(cmd, "components"), ","),
		}
		return runJiraUpdate(opts)
	},
}

var jiraGetCmd = &cobra.Command{
	Use:   "get [issue-key]",
	Short: "Get JIRA ticket details",
	Long: `Show summary, status, type, assignee, labels, dates, description (rendered
from ADF to Markdown) and issue links. Pass --history to also dump the
changelog (status/assignee/priority transitions with author + timestamp).

Examples:
  forecast jira get SMG-1234
  forecast jira get SMG-1234 --history`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		history, _ := cmd.Flags().GetBool("history")
		return runJiraGet(args[0], history)
	},
}

var jiraCommentCmd = &cobra.Command{
	Use:   "comment [issue-key]",
	Short: "Add a comment to a ticket",
	Long: `Add a comment to a JIRA ticket. Supports a small markdown subset
(## headings, - bullets, *bold*).

Examples:
  forecast jira comment SMG-1234 --body "Verified in staging."
  forecast jira comment SMG-1234 -b "## Repro\n- step one\n- step two"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		body, _ := cmd.Flags().GetString("body")
		return runJiraComment(args[0], body)
	},
}

var jiraCommentsCmd = &cobra.Command{
	Use:   "comments [issue-key]",
	Short: "List comments on a ticket",
	Long: `List all comments on a JIRA ticket, oldest first, with author and timestamp.

Examples:
  forecast jira comments SMG-1234
  forecast jira comments SMG-1234 --limit 5`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		limit, _ := cmd.Flags().GetInt("limit")
		return runJiraComments(args[0], limit)
	},
}

var jiraLinkCmd = &cobra.Command{
	Use:   "link [from-key]",
	Short: "Create an issue link",
	Long: `Create a link between two issues. The relationship reads:
  <from-key> <type-outward-verb> <to-key>

Examples:
  forecast jira link SMG-100 --to SMG-200 --type Blocks      # SMG-100 blocks SMG-200
  forecast jira link SMG-100 --to SMG-200 --type Relates
  forecast jira link SMG-100 --to SMG-200 --type Duplicate

Run 'forecast jira link-types' to see available types.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		to, _ := cmd.Flags().GetString("to")
		typ, _ := cmd.Flags().GetString("type")
		return runJiraLink(args[0], to, typ)
	},
}

var jiraUnlinkCmd = &cobra.Command{
	Use:   "unlink [link-id]",
	Short: "Delete an issue link by ID",
	Long: `Delete an issue link by its ID. Get link IDs from 'forecast jira get <key>'
(shown in brackets in the Links section).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runJiraUnlink(args[0])
	},
}

var jiraLinkTypesCmd = &cobra.Command{
	Use:   "link-types",
	Short: "List available issue link types",
	Long:  `List the link type names you can pass to 'forecast jira link --type', with the verbs they read in each direction.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runJiraLinkTypes()
	},
}

var jiraWatchCmd = &cobra.Command{
	Use:   "watch [issue-key]",
	Short: "Add yourself (or another user) as a watcher",
	Long: `Add a watcher to an issue. By default adds the authenticated user.
Use --user to add someone else by email.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		userEmail, _ := cmd.Flags().GetString("user")
		return runJiraWatch(args[0], userEmail)
	},
}

var jiraUnwatchCmd = &cobra.Command{
	Use:   "unwatch [issue-key]",
	Short: "Remove a watcher from an issue",
	Long:  `Remove a watcher. Use --user <email> to remove someone other than yourself (your own email is taken from config).`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		userEmail, _ := cmd.Flags().GetString("user")
		return runJiraUnwatch(args[0], userEmail)
	},
}

var jiraWatchersCmd = &cobra.Command{
	Use:   "watchers [issue-key]",
	Short: "List watchers on an issue",
	Long:  `List everyone watching an issue, with display name and email when available.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runJiraWatchers(args[0])
	},
}

var jiraLogCmd = &cobra.Command{
	Use:   "log [issue-key]",
	Short: "Log work against a ticket",
	Long: `Log time spent working on a ticket.

Examples:
  forecast jira log SMG-1234 --time 2h
  forecast jira log SMG-1234 --time 1h30m --comment "pairing with @alex"
  forecast jira log SMG-1234 --time 90m`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		timeStr, _ := cmd.Flags().GetString("time")
		comment, _ := cmd.Flags().GetString("comment")
		return runJiraLog(args[0], timeStr, comment)
	},
}

var jiraWorklogsCmd = &cobra.Command{
	Use:   "worklogs [issue-key]",
	Short: "List worklog entries on a ticket",
	Long: `List every worklog entry on a ticket with author, time spent, and any
comment (rendered from ADF to Markdown). Prints a total at the end.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runJiraWorklogs(args[0])
	},
}

var jiraTransitionCmd = &cobra.Command{
	Use:   "transition [issue-key]",
	Short: "Transition a ticket to a new status",
	Long: `Move a JIRA ticket to a new status, optionally with a comment and/or resolution.

Examples:
  forecast jira transition SMG-1234 --to "In Development"
  forecast jira transition SMG-1234 --to Done --comment "Completed the work"
  forecast jira transition SMG-1234 --to Done --resolution "Won't Do"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		issueKey := args[0]
		to, _ := cmd.Flags().GetString("to")
		comment, _ := cmd.Flags().GetString("comment")
		resolution, _ := cmd.Flags().GetString("resolution")
		return runJiraTransition(issueKey, to, comment, resolution)
	},
}

var jiraAttachmentsCmd = &cobra.Command{
	Use:   "attachments [issue-key]",
	Short: "List attachments on a ticket",
	Long: `List every attachment on a ticket with its ID, filename, size, upload date,
and author. Use the ID with 'forecast jira download'.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runJiraAttachments(args[0])
	},
}

var jiraAttachCmd = &cobra.Command{
	Use:   "attach [issue-key] [file-path]",
	Short: "Upload a file as an attachment",
	Long: `Upload a local file as an attachment on the ticket.

Example:
  forecast jira attach SMG-1234 ./screenshot.png`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runJiraAttach(args[0], args[1])
	},
}

var jiraDownloadCmd = &cobra.Command{
	Use:   "download [attachment-id]",
	Short: "Download an attachment by ID",
	Long: `Download an attachment by its ID. Get IDs from 'forecast jira attachments <key>'.
By default writes to <filename> in the current directory; use --out to override
or "-" to write to stdout.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		out := mustGetString(cmd, "out")
		return runJiraDownload(args[0], out)
	},
}

var jiraBulkCmd = &cobra.Command{
	Use:   "bulk",
	Short: "Bulk operations over a JQL query",
	Long: `Apply an action to every issue matching a JQL query.
Subcommands:
  transition - move all matching issues to a new status
  label      - add and/or remove labels on all matching issues`,
}

var jiraBulkTransitionCmd = &cobra.Command{
	Use:   "transition",
	Short: "Transition every issue matching --jql to a new status",
	Long: `Transition every JQL-matching issue.

Examples:
  forecast jira bulk transition --jql "project=SMG AND status='To Do'" --to "In Progress"
  forecast jira bulk transition --jql "..." --to Done --comment "shipped" --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		jql := mustGetString(cmd, "jql")
		to := mustGetString(cmd, "to")
		comment := mustGetString(cmd, "comment")
		resolution := mustGetString(cmd, "resolution")
		dry := mustGetBool(cmd, "dry-run")
		return runJiraBulkTransition(jql, to, comment, resolution, dry)
	},
}

var jiraBulkLabelCmd = &cobra.Command{
	Use:   "label",
	Short: "Add and/or remove labels on every issue matching --jql",
	Long: `Mutate labels on every JQL-matching issue.

Examples:
  forecast jira bulk label --jql "project=SMG AND status=Done" --add archived
  forecast jira bulk label --jql "..." --add foo --remove bar --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		jql := mustGetString(cmd, "jql")
		add := splitAndTrim(mustGetString(cmd, "add"), ",")
		remove := splitAndTrim(mustGetString(cmd, "remove"), ",")
		dry := mustGetBool(cmd, "dry-run")
		return runJiraBulkLabel(jql, add, remove, dry)
	},
}

var jiraBoardsCmd = &cobra.Command{
	Use:   "boards",
	Short: "List Agile boards (filtered to configured project by default)",
	Long: `List Agile boards visible to your account. Defaults to filtering by the
configured project_key; pass --project to override.

Use the board ID with 'forecast jira sprints --board <id>' if your project
has more than one board.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		return runJiraBoards(project)
	},
}

var jiraSprintsCmd = &cobra.Command{
	Use:   "sprints",
	Short: "List sprints on a board",
	Long: `List sprints on an Agile board. By default uses the only board for the
configured project; pass --board <id> if there are multiple.

Examples:
  forecast jira sprints
  forecast jira sprints --state active
  forecast jira sprints --board 42 --state active,future`,
	RunE: func(cmd *cobra.Command, args []string) error {
		boardID, _ := cmd.Flags().GetInt("board")
		state, _ := cmd.Flags().GetString("state")
		return runJiraSprints(boardID, state)
	},
}

var jiraSprintAddCmd = &cobra.Command{
	Use:   "sprint-add [sprint-id] [issue-key...]",
	Short: "Add issues to a sprint",
	Long: `Add one or more issues to a sprint by sprint ID. Get sprint IDs from
'forecast jira sprints'.

Example:
  forecast jira sprint-add 42 SMG-100 SMG-101 SMG-102`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runJiraSprintAdd(args[0], args[1:])
	},
}

var jiraSprintBacklogCmd = &cobra.Command{
	Use:   "sprint-backlog [issue-key...]",
	Short: "Move issues out of any sprint and into the backlog",
	Long: `Move one or more issues out of their current sprint and back to the backlog.

Example:
  forecast jira sprint-backlog SMG-100 SMG-101`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runJiraSprintBacklog(args)
	},
}

var jiraResolutionsCmd = &cobra.Command{
	Use:   "resolutions",
	Short: "List available resolutions",
	Long:  `List available resolution names for use with 'forecast jira transition --resolution'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runJiraResolutions()
	},
}

var jiraSearchCmd = &cobra.Command{
	Use:   "search [jql]",
	Short: "Search for tickets using JQL",
	Long: `Search for JIRA tickets using JQL query.

Examples:
  forecast jira search "project=SMG AND status='To Do'"
  forecast jira search "project=SMG AND priority=Highest" --limit 10`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		limit, _ := cmd.Flags().GetInt("limit")
		return runJiraSearch(args[0], limit)
	},
}

var jiraTransitionsCmd = &cobra.Command{
	Use:   "transitions [issue-key]",
	Short: "List available transitions for a ticket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runJiraTransitions(args[0])
	},
}

var jiraTypesCmd = &cobra.Command{
	Use:   "types",
	Short: "List available issue types",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runJiraTypes()
	},
}

var jiraPrioritiesCmd = &cobra.Command{
	Use:   "priorities",
	Short: "List available priorities",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runJiraPriorities()
	},
}

var jiraProjectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "List available JIRA projects",
	Long: `List all JIRA projects accessible with the current credentials.
Useful for debugging configuration issues or finding project keys.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runJiraProjects()
	},
}

var jiraFieldsCmd = &cobra.Command{
	Use:   "fields [issue-key]",
	Short: "List custom fields and their values for an issue",
	Long: `Shows all custom fields for a JIRA issue to help identify field IDs.
Useful for finding the story_points_field or cycle_time_field IDs.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runJiraFields(args[0])
	},
}

var jiraMissingTimesCmd = &cobra.Command{
	Use:   "missing-times",
	Short: "List done tickets missing cycle time data",
	Long: `List all tickets that are marked as Done but don't have cycle time data.
These tickets need manual time entry via JIRA's Time Spent field or a custom field.

Use --project to filter to a specific project/epic.
Use --fix to automatically log time based on story points (requires story_points_field in config).
Use --default-hours to specify hours for tickets without story points.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		fix, _ := cmd.Flags().GetBool("fix")
		hoursPerPoint, _ := cmd.Flags().GetFloat64("hours-per-point")
		defaultHours, _ := cmd.Flags().GetFloat64("default-hours")
		return runJiraMissingTimes(project, fix, hoursPerPoint, defaultHours)
	},
}

func init() {
	// Create command flags
	jiraCreateCmd.Flags().StringP("summary", "s", "", "Issue summary (required)")
	jiraCreateCmd.Flags().StringP("type", "t", "Task", "Issue type (Bug, Story, Task, Epic, Sub-task)")
	jiraCreateCmd.Flags().StringP("description", "d", "", "Issue description")
	jiraCreateCmd.Flags().StringP("priority", "p", "Medium", "Priority (Highest, High, Medium, Low, Lowest)")
	jiraCreateCmd.Flags().StringP("labels", "l", "", "Comma-separated labels")
	jiraCreateCmd.Flags().StringP("assignee", "a", "", "Assignee email")
	jiraCreateCmd.Flags().StringP("epic", "e", "", "Parent epic key (alias for --parent)")
	jiraCreateCmd.Flags().String("parent", "", "Parent key (epic for stories, story for sub-tasks)")
	jiraCreateCmd.Flags().Float64("story-points", 0, "Story points (requires story_points_field in config)")
	jiraCreateCmd.Flags().String("due-date", "", "Due date (YYYY-MM-DD)")
	jiraCreateCmd.Flags().String("fix-versions", "", "Comma-separated fix version names")
	jiraCreateCmd.Flags().String("components", "", "Comma-separated component names")
	jiraCreateCmd.MarkFlagRequired("summary")

	// Update command flags
	jiraUpdateCmd.Flags().StringP("summary", "s", "", "New summary")
	jiraUpdateCmd.Flags().StringP("description", "d", "", "New description")
	jiraUpdateCmd.Flags().StringP("priority", "p", "", "New priority")
	jiraUpdateCmd.Flags().StringP("labels", "l", "", "New labels (comma-separated)")
	jiraUpdateCmd.Flags().StringP("assignee", "a", "", "New assignee email")
	jiraUpdateCmd.Flags().StringP("epic", "e", "", "Parent epic key (alias for --parent)")
	jiraUpdateCmd.Flags().String("parent", "", "New parent key (epic or sub-task parent)")
	jiraUpdateCmd.Flags().Float64("story-points", -1, "New story points (-1 = unchanged)")
	jiraUpdateCmd.Flags().String("due-date", "", "New due date (YYYY-MM-DD); empty unchanged")
	jiraUpdateCmd.Flags().Bool("clear-due-date", false, "Clear the due date")
	jiraUpdateCmd.Flags().String("fix-versions", "", "Comma-separated fix version names (replaces existing)")
	jiraUpdateCmd.Flags().String("components", "", "Comma-separated component names (replaces existing)")

	// Get command flags
	jiraGetCmd.Flags().Bool("history", false, "Include changelog (status/assignee/priority changes)")

	// Comment command flags
	jiraCommentCmd.Flags().StringP("body", "b", "", "Comment body (required)")
	jiraCommentCmd.MarkFlagRequired("body")

	// Comments command flags
	jiraCommentsCmd.Flags().Int("limit", 0, "Limit number of comments (0 = all)")

	// Link command flags
	jiraLinkCmd.Flags().String("to", "", "Target issue key (required)")
	jiraLinkCmd.Flags().String("type", "Relates", "Link type name (e.g. Blocks, Relates, Duplicate)")
	jiraLinkCmd.MarkFlagRequired("to")

	// Watch / unwatch flags
	jiraWatchCmd.Flags().String("user", "", "Email of user to add (default: authenticated user)")
	jiraUnwatchCmd.Flags().String("user", "", "Email of user to remove (default: authenticated user)")

	// Worklog flags
	jiraLogCmd.Flags().String("time", "", "Time spent (e.g. 2h, 30m, 1h30m, 1.5h) — required")
	jiraLogCmd.Flags().StringP("comment", "c", "", "Optional comment for the worklog")
	jiraLogCmd.MarkFlagRequired("time")

	// Board / sprint flags
	jiraBoardsCmd.Flags().StringP("project", "p", "", "Project key (defaults to configured project_key)")
	jiraSprintsCmd.Flags().Int("board", 0, "Board ID (default: auto-discover from project)")
	jiraSprintsCmd.Flags().String("state", "active", "Sprint state filter: active, closed, future (comma-separated, blank for all)")

	// Bulk flags
	jiraBulkTransitionCmd.Flags().String("jql", "", "JQL query selecting issues (required)")
	jiraBulkTransitionCmd.Flags().String("to", "", "Target status (required)")
	jiraBulkTransitionCmd.Flags().StringP("comment", "c", "", "Comment to add with each transition")
	jiraBulkTransitionCmd.Flags().String("resolution", "", "Resolution name (e.g. Done, Won't Do)")
	jiraBulkTransitionCmd.Flags().Bool("dry-run", false, "Show what would happen without applying")
	jiraBulkTransitionCmd.MarkFlagRequired("jql")
	jiraBulkTransitionCmd.MarkFlagRequired("to")

	jiraBulkLabelCmd.Flags().String("jql", "", "JQL query selecting issues (required)")
	jiraBulkLabelCmd.Flags().String("add", "", "Comma-separated labels to add")
	jiraBulkLabelCmd.Flags().String("remove", "", "Comma-separated labels to remove")
	jiraBulkLabelCmd.Flags().Bool("dry-run", false, "Show what would happen without applying")
	jiraBulkLabelCmd.MarkFlagRequired("jql")

	// Download flag
	jiraDownloadCmd.Flags().StringP("out", "o", "", "Output path (default: original filename; '-' for stdout)")

	// Transition command flags
	jiraTransitionCmd.Flags().String("to", "", "Target status (required)")
	jiraTransitionCmd.Flags().StringP("comment", "c", "", "Comment to add with transition")
	jiraTransitionCmd.Flags().String("resolution", "", "Resolution name (e.g. Done, Won't Do)")
	jiraTransitionCmd.MarkFlagRequired("to")

	// Search command flags
	jiraSearchCmd.Flags().Int("limit", 20, "Maximum results to show")

	// Missing times command flags
	jiraMissingTimesCmd.Flags().StringP("project", "p", "", "Filter to specific project (by key or epic)")
	jiraMissingTimesCmd.Flags().Bool("fix", false, "Automatically log time based on story points")
	jiraMissingTimesCmd.Flags().Float64("hours-per-point", 4.0, "Hours per story point for --fix")
	jiraMissingTimesCmd.Flags().Float64("default-hours", 8.0, "Default hours for tickets without story points")

	// Add subcommands to jira command
	jiraCmd.AddCommand(jiraCreateCmd)
	jiraCmd.AddCommand(jiraUpdateCmd)
	jiraCmd.AddCommand(jiraGetCmd)
	jiraCmd.AddCommand(jiraCommentCmd)
	jiraCmd.AddCommand(jiraCommentsCmd)
	jiraCmd.AddCommand(jiraLinkCmd)
	jiraCmd.AddCommand(jiraUnlinkCmd)
	jiraCmd.AddCommand(jiraLinkTypesCmd)
	jiraCmd.AddCommand(jiraWatchCmd)
	jiraCmd.AddCommand(jiraUnwatchCmd)
	jiraCmd.AddCommand(jiraWatchersCmd)
	jiraCmd.AddCommand(jiraLogCmd)
	jiraCmd.AddCommand(jiraWorklogsCmd)
	jiraCmd.AddCommand(jiraTransitionCmd)
	jiraCmd.AddCommand(jiraResolutionsCmd)
	jiraCmd.AddCommand(jiraBoardsCmd)
	jiraCmd.AddCommand(jiraSprintsCmd)
	jiraCmd.AddCommand(jiraSprintAddCmd)
	jiraCmd.AddCommand(jiraSprintBacklogCmd)

	jiraBulkCmd.AddCommand(jiraBulkTransitionCmd)
	jiraBulkCmd.AddCommand(jiraBulkLabelCmd)
	jiraCmd.AddCommand(jiraBulkCmd)

	jiraCmd.AddCommand(jiraAttachmentsCmd)
	jiraCmd.AddCommand(jiraAttachCmd)
	jiraCmd.AddCommand(jiraDownloadCmd)
	jiraCmd.AddCommand(jiraSearchCmd)
	jiraCmd.AddCommand(jiraTransitionsCmd)
	jiraCmd.AddCommand(jiraTypesCmd)
	jiraCmd.AddCommand(jiraPrioritiesCmd)
	jiraCmd.AddCommand(jiraProjectsCmd)
	jiraCmd.AddCommand(jiraFieldsCmd)
	jiraCmd.AddCommand(jiraMissingTimesCmd)
}

type jiraCreateOpts struct {
	Summary     string
	IssueType   string
	Description string
	Priority    string
	Labels      []string
	Assignee    string
	Epic        string
	Parent      string
	StoryPoints float64
	DueDate     string
	FixVersions []string
	Components  []string
}

type jiraUpdateOpts struct {
	Key          string
	Summary      string
	Description  string
	Priority     string
	Labels       []string
	Assignee     string
	Epic         string
	Parent       string
	StoryPoints  float64 // -1 means unchanged
	DueDate      string
	ClearDueDate bool
	FixVersions  []string
	Components   []string
}

func runJiraCreate(opts jiraCreateOpts) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}

	cfg := config.Get()
	req := jira.CreateIssueRequest{
		Summary:          opts.Summary,
		IssueType:        opts.IssueType,
		Description:      opts.Description,
		Priority:         opts.Priority,
		Labels:           opts.Labels,
		Assignee:         opts.Assignee,
		Epic:             opts.Epic,
		Parent:           opts.Parent,
		StoryPoints:      opts.StoryPoints,
		DueDate:          opts.DueDate,
		FixVersions:      opts.FixVersions,
		Components:       opts.Components,
		StoryPointsField: cfg.JIRA.StoryPointsField,
		Project:          cfg.JIRA.ProjectKey,
	}

	if opts.StoryPoints > 0 && cfg.JIRA.StoryPointsField == "" {
		fmt.Println("Warning: --story-points ignored because story_points_field is not set in config")
	}

	result, err := client.CreateIssue(req)
	if err != nil {
		return fmt.Errorf("failed to create issue: %w", err)
	}

	fmt.Printf("Created: %s\n", result.Key)
	fmt.Printf("URL: %s/browse/%s\n", cfg.JIRA.URL, result.Key)
	return nil
}

func runJiraUpdate(opts jiraUpdateOpts) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}
	cfg := config.Get()

	req := jira.UpdateIssueRequest{StoryPointsField: cfg.JIRA.StoryPointsField}

	if opts.Summary != "" {
		req.Summary = &opts.Summary
	}
	if opts.Description != "" {
		req.Description = &opts.Description
	}
	if opts.Priority != "" {
		req.Priority = &opts.Priority
	}
	if len(opts.Labels) > 0 {
		req.Labels = opts.Labels
	}
	if opts.Assignee != "" {
		req.Assignee = &opts.Assignee
	}
	if opts.Parent != "" {
		req.Parent = &opts.Parent
	} else if opts.Epic != "" {
		req.Epic = &opts.Epic
	}
	if opts.StoryPoints >= 0 {
		if cfg.JIRA.StoryPointsField == "" {
			fmt.Println("Warning: --story-points ignored because story_points_field is not set in config")
		} else {
			pts := opts.StoryPoints
			req.StoryPoints = &pts
		}
	}
	if opts.ClearDueDate {
		empty := ""
		req.DueDate = &empty
	} else if opts.DueDate != "" {
		dd := opts.DueDate
		req.DueDate = &dd
	}
	if len(opts.FixVersions) > 0 {
		req.FixVersions = opts.FixVersions
	}
	if len(opts.Components) > 0 {
		req.Components = opts.Components
	}

	if err := client.UpdateIssue(opts.Key, req); err != nil {
		return fmt.Errorf("failed to update issue: %w", err)
	}

	fmt.Printf("Updated: %s\n", opts.Key)
	return nil
}

func runJiraGet(issueKey string, showHistory bool) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}

	issue, err := client.GetIssue(issueKey)
	if err != nil {
		return fmt.Errorf("failed to get issue: %w", err)
	}

	fmt.Printf("\n%s: %s\n", issue.Key, issue.Fields.Summary)
	fmt.Printf("  Status:   %s\n", issue.Fields.Status.Name)
	fmt.Printf("  Type:     %s\n", issue.Fields.IssueType.Name)
	if issue.Fields.Assignee != nil {
		fmt.Printf("  Assignee: %s\n", issue.Fields.Assignee.DisplayName)
	} else {
		fmt.Printf("  Assignee: Unassigned\n")
	}
	if len(issue.Fields.Labels) > 0 {
		fmt.Printf("  Labels:   %v\n", issue.Fields.Labels)
	}
	if len(issue.Fields.Created) >= 10 {
		fmt.Printf("  Created:  %s\n", issue.Fields.Created[:10])
	}
	if len(issue.Fields.Updated) >= 10 {
		fmt.Printf("  Updated:  %s\n", issue.Fields.Updated[:10])
	}

	if desc := jira.RenderADF(issue.Fields.Description); desc != "" {
		fmt.Printf("\nDescription:\n%s\n", desc)
	}

	printIssueLinks(issue.Fields.IssueLinks)

	if showHistory {
		printChangelog(issue.Changelog)
	}

	return nil
}

func printIssueLinks(links []jira.IssueLink) {
	if len(links) == 0 {
		return
	}
	fmt.Println("\nLinks:")
	for _, l := range links {
		var verb string
		var target *jira.LinkedIssue
		if l.OutwardIssue != nil {
			verb = l.Type.Outward
			target = l.OutwardIssue
		} else if l.InwardIssue != nil {
			verb = l.Type.Inward
			target = l.InwardIssue
		} else {
			continue
		}
		summary := target.Fields.Summary
		if len(summary) > 50 {
			summary = summary[:47] + "..."
		}
		fmt.Printf("  [%-8s] %-15s %-12s [%-12s] %s\n", l.ID, verb, target.Key, target.Fields.Status.Name, summary)
	}
}

func printChangelog(cl *jira.Changelog) {
	if cl == nil || len(cl.Histories) == 0 {
		fmt.Println("\nHistory: (none)")
		return
	}
	fmt.Println("\nHistory:")
	for _, h := range cl.Histories {
		when := h.Created
		if len(when) >= 19 {
			when = strings.Replace(when[:19], "T", " ", 1)
		}
		who := "Unknown"
		if h.Author != nil && h.Author.DisplayName != "" {
			who = h.Author.DisplayName
		}
		for _, item := range h.Items {
			from := item.FromString
			to := item.ToString
			if from == "" {
				from = "(none)"
			}
			if to == "" {
				to = "(none)"
			}
			fmt.Printf("  %s · %-20s · %-12s %s → %s\n", when, who, item.Field, from, to)
		}
	}
}

func runJiraComment(issueKey, body string) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}
	if err := client.AddComment(issueKey, body); err != nil {
		return fmt.Errorf("failed to add comment: %w", err)
	}
	fmt.Printf("Comment added to %s\n", issueKey)
	return nil
}

func runJiraComments(issueKey string, limit int) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}
	comments, err := client.GetComments(issueKey)
	if err != nil {
		return fmt.Errorf("failed to fetch comments: %w", err)
	}

	if len(comments) == 0 {
		fmt.Printf("No comments on %s\n", issueKey)
		return nil
	}

	// If limited, take the most recent N (comments are oldest-first).
	start := 0
	if limit > 0 && len(comments) > limit {
		start = len(comments) - limit
		fmt.Printf("Showing %d of %d comments on %s\n", limit, len(comments), issueKey)
	} else {
		fmt.Printf("%d comment(s) on %s\n", len(comments), issueKey)
	}

	for i := start; i < len(comments); i++ {
		c := comments[i]
		author := "Unknown"
		if c.Author != nil && c.Author.DisplayName != "" {
			author = c.Author.DisplayName
		}
		when := c.Created
		if len(when) >= 19 {
			when = strings.Replace(when[:19], "T", " ", 1)
		}
		fmt.Printf("\n— %s · %s\n", author, when)
		body := jira.RenderADF(c.Body)
		if body == "" {
			body = "(empty)"
		}
		fmt.Println(body)
	}
	return nil
}

func runJiraLink(fromKey, toKey, linkType string) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}
	if err := client.LinkIssues(fromKey, toKey, linkType); err != nil {
		return fmt.Errorf("failed to create link: %w", err)
	}
	fmt.Printf("Linked: %s [%s] %s\n", fromKey, linkType, toKey)
	return nil
}

func runJiraUnlink(linkID string) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}
	if err := client.DeleteIssueLink(linkID); err != nil {
		return fmt.Errorf("failed to delete link: %w", err)
	}
	fmt.Printf("Deleted link %s\n", linkID)
	return nil
}

func runJiraLinkTypes() error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}
	types, err := client.GetIssueLinkTypes()
	if err != nil {
		return fmt.Errorf("failed to fetch link types: %w", err)
	}
	fmt.Println("Available issue link types:")
	fmt.Printf("  %-15s %-25s %-25s\n", "Name", "Outward (from→to)", "Inward (to→from)")
	fmt.Println("  " + strings.Repeat("-", 65))
	for _, t := range types {
		fmt.Printf("  %-15s %-25s %-25s\n", t.Name, t.Outward, t.Inward)
	}
	return nil
}

func runJiraWatch(issueKey, userEmail string) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}
	accountID := ""
	who := "you"
	if userEmail != "" {
		accountID, err = client.GetUserAccountID(userEmail)
		if err != nil {
			return fmt.Errorf("failed to find user: %w", err)
		}
		who = userEmail
	}
	if err := client.AddWatcher(issueKey, accountID); err != nil {
		return fmt.Errorf("failed to add watcher: %w", err)
	}
	fmt.Printf("Added %s as watcher on %s\n", who, issueKey)
	return nil
}

func runJiraUnwatch(issueKey, userEmail string) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}
	cfg := config.Get()
	target := userEmail
	if target == "" {
		target = cfg.JIRA.Email
	}
	if target == "" {
		return fmt.Errorf("no user specified and no email in config")
	}
	accountID, err := client.GetUserAccountID(target)
	if err != nil {
		return fmt.Errorf("failed to find user: %w", err)
	}
	if err := client.RemoveWatcher(issueKey, accountID); err != nil {
		return fmt.Errorf("failed to remove watcher: %w", err)
	}
	fmt.Printf("Removed %s as watcher on %s\n", target, issueKey)
	return nil
}

func runJiraWatchers(issueKey string) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}
	resp, err := client.GetWatchers(issueKey)
	if err != nil {
		return fmt.Errorf("failed to fetch watchers: %w", err)
	}
	fmt.Printf("%d watcher(s) on %s\n", resp.WatchCount, issueKey)
	for _, w := range resp.Watchers {
		email := w.EmailAddress
		if email == "" {
			email = "(no email)"
		}
		fmt.Printf("  %s · %s\n", w.DisplayName, email)
	}
	return nil
}

func runJiraLog(issueKey, timeStr, comment string) error {
	dur, err := time.ParseDuration(timeStr)
	if err != nil {
		return fmt.Errorf("invalid time %q: use 2h, 30m, 1h30m, 1.5h, etc.", timeStr)
	}
	if dur <= 0 {
		return fmt.Errorf("time must be positive")
	}
	client, err := getJiraClient()
	if err != nil {
		return err
	}
	seconds := int(dur.Seconds())
	if err := client.LogWork(issueKey, seconds, comment); err != nil {
		return fmt.Errorf("failed to log work: %w", err)
	}
	fmt.Printf("Logged %s on %s\n", dur, issueKey)
	return nil
}

func runJiraWorklogs(issueKey string) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}
	logs, err := client.GetWorklogs(issueKey)
	if err != nil {
		return fmt.Errorf("failed to fetch worklogs: %w", err)
	}
	if len(logs) == 0 {
		fmt.Printf("No worklog entries on %s\n", issueKey)
		return nil
	}
	totalSeconds := 0
	fmt.Printf("%d worklog entry(ies) on %s\n", len(logs), issueKey)
	for _, w := range logs {
		totalSeconds += w.TimeSpentSeconds
		when := w.Started
		if when == "" {
			when = w.Created
		}
		if len(when) >= 19 {
			when = strings.Replace(when[:19], "T", " ", 1)
		}
		who := "Unknown"
		if w.Author != nil && w.Author.DisplayName != "" {
			who = w.Author.DisplayName
		}
		fmt.Printf("\n— %s · %s · %s\n", who, when, w.TimeSpent)
		body := jira.RenderADF(w.Comment)
		if body != "" {
			fmt.Println(body)
		}
	}
	fmt.Printf("\nTotal: %s\n", time.Duration(totalSeconds)*time.Second)
	return nil
}

func runJiraTransition(issueKey, to, comment, resolution string) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}

	if err := client.TransitionIssue(issueKey, jira.TransitionOptions{
		Name:       to,
		Comment:    comment,
		Resolution: resolution,
	}); err != nil {
		return fmt.Errorf("failed to transition issue: %w", err)
	}

	fmt.Printf("Transitioned %s to '%s'\n", issueKey, to)
	if resolution != "" {
		fmt.Printf("Resolution: %s\n", resolution)
	}
	return nil
}

func runJiraAttachments(issueKey string) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}
	issue, err := client.GetIssue(issueKey)
	if err != nil {
		return fmt.Errorf("failed to fetch issue: %w", err)
	}
	if len(issue.Fields.Attachment) == 0 {
		fmt.Printf("No attachments on %s\n", issueKey)
		return nil
	}
	fmt.Printf("%d attachment(s) on %s:\n", len(issue.Fields.Attachment), issueKey)
	fmt.Printf("  %-12s %-30s %-10s %-12s %s\n", "ID", "Filename", "Size", "Created", "Author")
	fmt.Println("  " + strings.Repeat("-", 75))
	for _, a := range issue.Fields.Attachment {
		author := ""
		if a.Author != nil {
			author = a.Author.DisplayName
		}
		filename := a.Filename
		if len(filename) > 30 {
			filename = filename[:27] + "..."
		}
		fmt.Printf("  %-12s %-30s %-10s %-12s %s\n", a.ID, filename, humanSize(a.Size), datePart(a.Created), author)
	}
	return nil
}

func runJiraAttach(issueKey, filePath string) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}
	att, err := client.UploadAttachment(issueKey, filePath)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	fmt.Printf("Uploaded %s (id=%s, %s) to %s\n", att.Filename, att.ID, humanSize(att.Size), issueKey)
	return nil
}

func runJiraDownload(attachmentID, out string) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}
	if out == "" {
		// Default: name file by attachment ID; user can re-set with --out.
		// We can't know the original filename without an extra fetch, so use the
		// ID as a safe default.
		out = "attachment-" + attachmentID
	}
	n, err := client.DownloadAttachment(attachmentID, out)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	if out == "-" {
		// Don't print to stdout — content already written there.
		return nil
	}
	fmt.Printf("Wrote %s (%s)\n", out, humanSize(n))
	return nil
}

// humanSize formats a byte count as a short human-readable string.
func humanSize(n int64) string {
	const k = 1024
	if n < k {
		return fmt.Sprintf("%dB", n)
	}
	if n < k*k {
		return fmt.Sprintf("%.1fKB", float64(n)/k)
	}
	if n < k*k*k {
		return fmt.Sprintf("%.1fMB", float64(n)/(k*k))
	}
	return fmt.Sprintf("%.1fGB", float64(n)/(k*k*k))
}

func runJiraBulkTransition(jql, to, comment, resolution string, dry bool) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}
	issues, err := client.SearchJQL(jql)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}
	if len(issues) == 0 {
		fmt.Println("No issues match.")
		return nil
	}
	fmt.Printf("Matched %d issue(s); transitioning to %q\n", len(issues), to)
	if dry {
		for _, i := range issues {
			fmt.Printf("  [dry-run] %-12s %s → %s\n", i.Key, i.Fields.Status.Name, to)
		}
		return nil
	}
	failed := 0
	for _, i := range issues {
		err := client.TransitionIssue(i.Key, jira.TransitionOptions{
			Name:       to,
			Comment:    comment,
			Resolution: resolution,
		})
		if err != nil {
			fmt.Printf("  ✗ %-12s %v\n", i.Key, err)
			failed++
		} else {
			fmt.Printf("  ✓ %-12s → %s\n", i.Key, to)
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d transition(s) failed", failed)
	}
	return nil
}

func runJiraBulkLabel(jql string, add, remove []string, dry bool) error {
	if len(add) == 0 && len(remove) == 0 {
		return fmt.Errorf("specify --add and/or --remove")
	}
	client, err := getJiraClient()
	if err != nil {
		return err
	}
	issues, err := client.SearchJQL(jql)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}
	if len(issues) == 0 {
		fmt.Println("No issues match.")
		return nil
	}
	fmt.Printf("Matched %d issue(s); add=%v remove=%v\n", len(issues), add, remove)
	if dry {
		for _, i := range issues {
			fmt.Printf("  [dry-run] %-12s\n", i.Key)
		}
		return nil
	}
	failed := 0
	for _, i := range issues {
		if err := client.UpdateLabels(i.Key, add, remove); err != nil {
			fmt.Printf("  ✗ %-12s %v\n", i.Key, err)
			failed++
		} else {
			fmt.Printf("  ✓ %-12s\n", i.Key)
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d label update(s) failed", failed)
	}
	return nil
}

func runJiraBoards(projectKey string) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}
	if projectKey == "" {
		projectKey = config.Get().JIRA.ProjectKey
	}
	boards, err := client.GetBoards(projectKey)
	if err != nil {
		return fmt.Errorf("failed to fetch boards: %w", err)
	}
	if len(boards) == 0 {
		fmt.Println("No boards found.")
		return nil
	}
	fmt.Printf("Boards (project=%s):\n", projectKey)
	fmt.Printf("  %-6s %-12s %s\n", "ID", "Type", "Name")
	fmt.Println("  " + strings.Repeat("-", 50))
	for _, b := range boards {
		fmt.Printf("  %-6d %-12s %s\n", b.ID, b.Type, b.Name)
	}
	return nil
}

// resolveBoardID returns the board ID, auto-discovering from the configured
// project when boardID is 0. Errors if zero or multiple boards exist.
func resolveBoardID(client *jira.Client, boardID int) (int, error) {
	if boardID > 0 {
		return boardID, nil
	}
	cfg := config.Get()
	boards, err := client.GetBoards(cfg.JIRA.ProjectKey)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch boards for auto-discovery: %w", err)
	}
	switch len(boards) {
	case 0:
		return 0, fmt.Errorf("no boards found for project %s — pass --board <id>", cfg.JIRA.ProjectKey)
	case 1:
		return boards[0].ID, nil
	default:
		ids := make([]string, 0, len(boards))
		for _, b := range boards {
			ids = append(ids, fmt.Sprintf("%d (%s)", b.ID, b.Name))
		}
		return 0, fmt.Errorf("multiple boards for project %s — pass --board <id>: %s", cfg.JIRA.ProjectKey, strings.Join(ids, ", "))
	}
}

func runJiraSprints(boardID int, state string) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}
	boardID, err = resolveBoardID(client, boardID)
	if err != nil {
		return err
	}
	sprints, err := client.GetSprints(boardID, state)
	if err != nil {
		return fmt.Errorf("failed to fetch sprints: %w", err)
	}
	if len(sprints) == 0 {
		fmt.Printf("No sprints found on board %d (state=%q)\n", boardID, state)
		return nil
	}
	fmt.Printf("Sprints on board %d:\n", boardID)
	fmt.Printf("  %-6s %-8s %-30s %-12s %s\n", "ID", "State", "Name", "Start", "End")
	fmt.Println("  " + strings.Repeat("-", 75))
	for _, s := range sprints {
		start := datePart(s.StartDate)
		end := datePart(s.EndDate)
		name := s.Name
		if len(name) > 30 {
			name = name[:27] + "..."
		}
		fmt.Printf("  %-6d %-8s %-30s %-12s %s\n", s.ID, s.State, name, start, end)
		if s.Goal != "" {
			fmt.Printf("         Goal: %s\n", s.Goal)
		}
	}
	return nil
}

func runJiraSprintAdd(sprintIDStr string, keys []string) error {
	sprintID, err := strconv.Atoi(sprintIDStr)
	if err != nil {
		return fmt.Errorf("invalid sprint ID %q (must be a number)", sprintIDStr)
	}
	client, err := getJiraClient()
	if err != nil {
		return err
	}
	if err := client.MoveIssuesToSprint(sprintID, keys); err != nil {
		return fmt.Errorf("failed to move issues to sprint: %w", err)
	}
	fmt.Printf("Added %d issue(s) to sprint %d\n", len(keys), sprintID)
	return nil
}

func runJiraSprintBacklog(keys []string) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}
	if err := client.MoveIssuesToBacklog(keys); err != nil {
		return fmt.Errorf("failed to move issues to backlog: %w", err)
	}
	fmt.Printf("Moved %d issue(s) to backlog\n", len(keys))
	return nil
}

// datePart returns the YYYY-MM-DD prefix of a JIRA timestamp, or "" if absent.
func datePart(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return ""
}

func runJiraResolutions() error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}
	resolutions, err := client.GetResolutions()
	if err != nil {
		return fmt.Errorf("failed to fetch resolutions: %w", err)
	}
	fmt.Println("Available resolutions:")
	for _, r := range resolutions {
		fmt.Printf("  %-20s (ID: %s)\n", r.Name, r.ID)
	}
	return nil
}

func runJiraSearch(jql string, limit int) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}

	issues, err := client.SearchJQL(jql)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	fmt.Printf("Found %d issues\n\n", len(issues))

	displayed := 0
	for _, issue := range issues {
		if displayed >= limit {
			break
		}

		status := issue.Fields.Status.Name
		summary := issue.Fields.Summary
		if len(summary) > 50 {
			summary = summary[:50] + "..."
		}

		fmt.Printf("%-12s | %-20s | %s\n", issue.Key, status, summary)
		displayed++
	}

	if len(issues) > limit {
		fmt.Printf("\n... and %d more (use --limit to see more)\n", len(issues)-limit)
	}

	return nil
}

func runJiraTransitions(issueKey string) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}

	transitions, err := client.GetTransitions(issueKey)
	if err != nil {
		return fmt.Errorf("failed to get transitions: %w", err)
	}

	fmt.Printf("Available transitions for %s:\n\n", issueKey)
	for _, t := range transitions {
		fmt.Printf("  %-25s -> %s\n", t.Name, t.To.Name)
	}

	return nil
}

func runJiraTypes() error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}

	cfg := config.Get()
	types, err := client.GetIssueTypes(cfg.JIRA.ProjectKey)
	if err != nil {
		return fmt.Errorf("failed to get issue types: %w", err)
	}

	fmt.Println("Available issue types:")
	for name, id := range types {
		fmt.Printf("  %-20s (ID: %s)\n", name, id)
	}

	return nil
}

func runJiraPriorities() error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}

	priorities, err := client.GetPriorities()
	if err != nil {
		return fmt.Errorf("failed to get priorities: %w", err)
	}

	fmt.Println("Available priorities:")
	for name, id := range priorities {
		fmt.Printf("  %-15s (ID: %s)\n", name, id)
	}

	return nil
}

func runJiraProjects() error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}

	projects, err := client.GetProjects()
	if err != nil {
		return fmt.Errorf("failed to get projects: %w", err)
	}

	fmt.Println("Available JIRA projects:")
	fmt.Printf("  %-12s %-40s %s\n", "Key", "Name", "ID")
	fmt.Println("  " + strings.Repeat("-", 60))
	for _, p := range projects {
		name := p.Name
		if len(name) > 40 {
			name = name[:37] + "..."
		}
		fmt.Printf("  %-12s %-40s %s\n", p.Key, name, p.ID)
	}

	cfg := config.Get()
	fmt.Printf("\nConfigured project_key: %s\n", cfg.JIRA.ProjectKey)

	return nil
}

func runJiraFields(issueKey string) error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("failed to load config - run 'forecast init' first")
	}

	client := jira.NewClient(&cfg.JIRA)

	// Get field definitions to show names
	fieldDefs, err := client.GetFields()
	if err != nil {
		fmt.Printf("Warning: couldn't get field definitions: %v\n", err)
	}
	fieldNames := make(map[string]string)
	for _, f := range fieldDefs {
		fieldNames[f.ID] = f.Name
	}

	// Fetch the issue with all fields
	jql := fmt.Sprintf(`key = %s`, issueKey)
	issues, err := client.SearchWithAllFields(jql)
	if err != nil {
		return fmt.Errorf("failed to fetch issue: %w", err)
	}

	if len(issues) == 0 {
		return fmt.Errorf("issue %s not found", issueKey)
	}

	issue := issues[0]

	fmt.Printf("\nCustom fields for %s:\n", issueKey)
	fmt.Println("────────────────────────────────────────────────────────────────────────────────")
	fmt.Printf("%-25s %-25s %-10s %s\n", "Field ID", "Name", "Type", "Value")
	fmt.Println("────────────────────────────────────────────────────────────────────────────────")

	// Look for story points and other interesting fields
	for fieldID, value := range issue.RawFields {
		if strings.HasPrefix(fieldID, "customfield_") && value != nil {
			valueStr := fmt.Sprintf("%v", value)
			if len(valueStr) > 25 {
				valueStr = valueStr[:22] + "..."
			}

			name := fieldNames[fieldID]
			if len(name) > 25 {
				name = name[:22] + "..."
			}

			// Detect type
			typeStr := "unknown"
			switch value.(type) {
			case float64:
				typeStr = "number"
			case string:
				typeStr = "string"
			case map[string]interface{}:
				typeStr = "object"
			case []interface{}:
				typeStr = "array"
			}

			fmt.Printf("%-25s %-25s %-10s %s\n", fieldID, name, typeStr, valueStr)
		}
	}

	// Show potential story points fields
	fmt.Println("\nPotential story points fields:")
	for _, f := range fieldDefs {
		nameLower := strings.ToLower(f.Name)
		if strings.Contains(nameLower, "story") || strings.Contains(nameLower, "point") || strings.Contains(nameLower, "estimate") {
			fmt.Printf("  %-25s %s\n", f.ID, f.Name)
		}
	}

	fmt.Println("\nTo use a field for story points, add to .forecast/config.yaml:")
	fmt.Println("  jira:")
	fmt.Println("    story_points_field: customfield_XXXXX")

	return nil
}

func runJiraMissingTimes(projectFilter string, fix bool, hoursPerPoint float64, defaultHours float64) error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("failed to load config - run 'forecast init' first")
	}

	// Warn if story points field not configured but fix is requested
	if fix && cfg.JIRA.StoryPointsField == "" {
		fmt.Printf("Note: story_points_field not configured, using default of %.0f hours per ticket\n", defaultHours)
	}

	client := jira.NewClient(&cfg.JIRA)

	// Determine which projects to check
	var projects []config.ProjectConfig
	if projectFilter != "" {
		proj := cfg.GetProject(projectFilter)
		if proj == nil {
			return fmt.Errorf("project '%s' not found in config", projectFilter)
		}
		projects = []config.ProjectConfig{*proj}
	} else {
		projects = cfg.GetAllProjects()
	}

	if len(projects) == 0 {
		fmt.Println("No projects configured.")
		return nil
	}

	if fix {
		fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("  LOGGING TIME FOR MISSING TICKETS")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("\nUsing %v hours per story point\n", hoursPerPoint)
	} else {
		fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("  TICKETS MISSING CYCLE TIME DATA")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("\nThese tickets are Done but have no cycle time from workflow transitions or time tracking.")
		fmt.Println("Please log time in JIRA or update the custom cycle time field.")
	}

	totalMissing := 0
	totalFixed := 0

	for _, proj := range projects {
		// Build JQL for done tickets in this epic
		jql := fmt.Sprintf(`project = %s AND "Epic Link" = %s AND status = Done`, cfg.JIRA.ProjectKey, proj.Epic)

		issues, err := client.SearchJQL(jql)
		if err != nil {
			// Try parent field for next-gen projects
			jql = fmt.Sprintf(`project = %s AND parent = %s AND status = Done`, cfg.JIRA.ProjectKey, proj.Epic)
			issues, err = client.SearchJQL(jql)
			if err != nil {
				fmt.Printf("Error fetching %s: %v\n", proj.Name, err)
				continue
			}
		}

		// Build maps for lookup
		issueMap := make(map[string]*jira.Changelog)
		rawFieldsMap := make(map[string]map[string]interface{})
		for i := range issues {
			issueMap[issues[i].Key] = issues[i].Changelog
			rawFieldsMap[issues[i].Key] = issues[i].RawFields
		}

		// Convert to items to calculate cycle times
		items := client.ConvertIssuesToItems(issues, cfg)

		// Filter for items missing cycle time
		type missingItem struct {
			key       string
			summary   string
			closedBy  string
			rawFields map[string]interface{}
		}
		var missing []missingItem
		for _, item := range items {
			if item.CycleTime == 0 {
				closedBy := ""
				if changelog, ok := issueMap[item.JiraKey]; ok {
					closedBy = client.ExtractClosedBy(changelog)
				}
				missing = append(missing, missingItem{
					key:       item.JiraKey,
					summary:   item.Description,
					closedBy:  closedBy,
					rawFields: rawFieldsMap[item.JiraKey],
				})
			}
		}

		if len(missing) > 0 {
			fmt.Printf("\n## %s (%d missing)\n", proj.Name, len(missing))
			fmt.Println("────────────────────────────────────────────────────────────────────────────────")
			for _, m := range missing {
				summary := m.summary
				if len(summary) > 40 {
					summary = summary[:37] + "..."
				}
				closedByStr := m.closedBy
				if closedByStr == "" {
					closedByStr = "(unknown)"
				}

				if fix {
					// Get story points and log time
					storyPoints := jira.GetStoryPoints(m.rawFields, cfg.JIRA.StoryPointsField)
					var hours float64
					var comment string
					if storyPoints > 0 {
						hours = storyPoints * hoursPerPoint
						comment = fmt.Sprintf("Retroactive time log: %.0f story points × %.0f hours = %.0f hours", storyPoints, hoursPerPoint, hours)
					} else {
						// Use default hours for tickets without story points
						hours = defaultHours
						comment = fmt.Sprintf("Retroactive time log: default %.0f hours (no story points)", hours)
					}

					seconds := int(hours * 3600)
					err := client.LogWork(m.key, seconds, comment)
					if err != nil {
						fmt.Printf("  %-12s %-42s [%s] ✗ Error: %v\n", m.key, summary, closedByStr, err)
					} else {
						if storyPoints > 0 {
							fmt.Printf("  %-12s %-42s [%s] ✓ Logged %.0fh (%.0f pts)\n", m.key, summary, closedByStr, hours, storyPoints)
						} else {
							fmt.Printf("  %-12s %-42s [%s] ✓ Logged %.0fh (default)\n", m.key, summary, closedByStr, hours)
						}
						totalFixed++
					}
				} else {
					fmt.Printf("  %-12s %-52s [%s]\n", m.key, summary, closedByStr)
					fmt.Printf("               %s/browse/%s\n", cfg.JIRA.URL, m.key)
				}
			}
			totalMissing += len(missing)
		}
	}

	fmt.Println()
	if totalMissing == 0 {
		fmt.Println("✓ All done tickets have cycle time data!")
	} else if fix {
		fmt.Println("────────────────────────────────────────────────────────────────────────────────")
		fmt.Printf("Summary: %d tickets processed\n", totalMissing)
		fmt.Printf("  ✓ Fixed:   %d\n", totalFixed)
		fmt.Printf("  ✗ Errors:  %d\n", totalMissing-totalFixed)
		if totalFixed > 0 {
			fmt.Println("\nTime has been logged. Run 'forecast dashboard' to see updated forecasts.")
		}
	} else {
		fmt.Println("────────────────────────────────────────────────────────────────────────────────")
		fmt.Printf("Total: %d tickets need time data\n\n", totalMissing)
		fmt.Println("To add time in JIRA:")
		fmt.Println("  1. Open the ticket")
		fmt.Println("  2. Click 'Log work' or update the Time Spent field")
		fmt.Println("  3. Or set a custom cycle time field if configured")
		fmt.Println("\nOr use --fix to automatically log time based on story points:")
		fmt.Printf("  forecast jira missing-times --fix --default-hours %.0f\n", defaultHours)
	}

	return nil
}
