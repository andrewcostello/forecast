# Forecast CLI - Command Reference

## Quick Reference

```bash
# Initialize project
forecast init

# Sync data from JIRA
forecast sync

# Run Monte Carlo forecast
forecast run
forecast run --confidence 50,70,85,95
forecast run --iterations 50000

# Generate reports
forecast report --type eva        # Earned Value Analysis
forecast report --type montecarlo # Monte Carlo results
forecast report --type full       # Complete report

# Multi-project dashboard
forecast dashboard                      # Summary of all projects
forecast dashboard --project monorepo   # Detailed view of specific project
```

## JIRA Commands

`forecast jira --help` lists every subcommand grouped by lifecycle phase.
The summary below mirrors that grouping.

### Read & Search

```bash
# Ticket details: status, type, assignee, labels, dates, description
# (rendered from ADF to Markdown), and issue links with link IDs.
forecast jira get SMG-123
forecast jira get SMG-123 --history     # also dumps changelog (status/assignee/priority)

# Comments and worklog (rendered from ADF; worklogs include a total)
forecast jira comments SMG-123
forecast jira comments SMG-123 --limit 5
forecast jira worklogs SMG-123

# Watchers and attachments
forecast jira watchers SMG-123
forecast jira attachments SMG-123

# JQL search
forecast jira search "project=SMG AND status='To Do'"
forecast jira search "assignee=currentUser() AND status not in (Done, Canceled)"
forecast jira search "project=SMG AND labels in (size:L)" --limit 20
```

### Create & Edit

```bash
# Create — full field surface
forecast jira create --summary "Fix login bug" --type Bug --priority High
forecast jira create --summary "Add feature" --type Story \
  --labels "size:M,type:component" --story-points 5 --due-date 2026-06-30
forecast jira create --summary "Task" --type Task \
  --assignee user@email.com --epic SMG-100

# Sub-task: --parent is distinct from --epic
forecast jira create --summary "Add UI toggle" --type Sub-task --parent SMG-100

# Fix versions / components
forecast jira create --summary "Hotfix" --type Bug \
  --fix-versions "v1.4.2" --components "Backend,API"

# Update — omit fields to leave them unchanged
forecast jira update SMG-123 --priority Highest
forecast jira update SMG-123 --story-points 8 --due-date 2026-07-15
forecast jira update SMG-123 --clear-due-date    # explicit clear
forecast jira update SMG-123 --labels "size:L,type:integration"  # replaces

# Comments and attachments
forecast jira comment SMG-123 --body "## Repro\n- step one\n- step two"
forecast jira attach SMG-123 ./screenshot.png
forecast jira download <attachment-id> --out screenshot.png
forecast jira download <attachment-id> --out -    # write bytes to stdout
```

### Workflow & Collaboration

```bash
# Transitions (optionally with comment and/or resolution)
forecast jira transitions SMG-123              # show valid transitions first
forecast jira transition SMG-123 --to "In Progress"
forecast jira transition SMG-123 --to "Done" --comment "Shipped" --resolution Done
forecast jira transition SMG-123 --to "Done" --resolution "Won't Do"

# Worklog (parses 2h, 30m, 1h30m, 1.5h)
forecast jira log SMG-123 --time 2h --comment "pairing with @alex"

# Issue links — relationship reads: <from> <type.outward> <to>
forecast jira link SMG-100 --to SMG-200 --type Blocks      # SMG-100 blocks SMG-200
forecast jira link SMG-100 --to SMG-200 --type Relates
forecast jira unlink <link-id>                              # IDs shown in `get` Links section
forecast jira link-types

# Watchers (defaults to the authenticated user)
forecast jira watch SMG-123
forecast jira watch SMG-123 --user other@company.com
forecast jira unwatch SMG-123
```

### Sprint & Release

```bash
forecast jira boards                                 # discover board IDs
forecast jira sprints                                # active sprints; auto-discovers single board
forecast jira sprints --board 42 --state active,future
forecast jira sprint-add 42 SMG-100 SMG-101         # add issues to sprint 42
forecast jira sprint-backlog SMG-100                 # remove from any sprint
forecast jira resolutions                            # available resolution names
```

### Bulk Operations

```bash
# Apply a transition over a JQL query (use --dry-run to preview)
forecast jira bulk transition --jql "project=SMG AND status='To Do'" \
  --to "In Progress" --dry-run
forecast jira bulk transition --jql "..." --to Done --resolution Done --comment "shipped"

# Add and/or remove labels in bulk
forecast jira bulk label --jql "project=SMG AND status=Done" --add archived
forecast jira bulk label --jql "..." --add foo --remove bar --dry-run
```

### Discovery

```bash
forecast jira projects                  # accessible projects
forecast jira types                     # issue types for the project
forecast jira priorities                # priority names
forecast jira resolutions               # resolution names (for --resolution)
forecast jira link-types                # issue link type names
forecast jira transitions SMG-123       # valid transitions out of current status
forecast jira fields SMG-123            # custom field IDs (find story_points_field, etc.)
forecast jira missing-times             # audit Done tickets without cycle time
forecast jira missing-times --fix       # backfill from story points
```

> **Rich-text rendering.** Descriptions, comments, and worklog comments are
> stored in JIRA as Atlassian Document Format (ADF). The CLI renders them
> as Markdown for reading, and accepts a small markdown subset
> (`## headings`, `- bullets`, `*bold*`) for writing.

## Typical Workflows

### Starting Work on a Ticket

```bash
# 1. Read the ticket end-to-end (description + links + recent comments)
forecast jira get SMG-123
forecast jira comments SMG-123 --limit 5

# 2. Ensure it has size/type labels
forecast jira update SMG-123 --labels "size:M,type:component"

# 3. Watch it so you see future updates
forecast jira watch SMG-123

# 4. Start work (begins cycle time tracking)
forecast jira transition SMG-123 --to "In Progress"
```

### During Work

```bash
# Leave progress notes (markdown subset supported)
forecast jira comment SMG-123 --body "## Status\n- backend done\n- starting UI"

# Log time as you go
forecast jira log SMG-123 --time 1h30m --comment "pairing with @alex"

# Surface a blocker
forecast jira link SMG-123 --to SMG-456 --type "is blocked by"
```

### Completing Work

```bash
# 1. Mark done with summary and resolution
forecast jira transition SMG-123 --to "Done" \
  --resolution Done \
  --comment "Implemented feature X. Files: src/foo.ts, src/bar.ts"

# 2. Sync forecast data
forecast sync

# 3. Check updated forecast
forecast report --type eva
```

### Creating a New Feature

```bash
# 1. Create sized ticket with full context
forecast jira create --summary "Add user notifications" \
  --type Story \
  --labels "size:L,type:component" \
  --story-points 8 \
  --due-date 2026-06-30 \
  --description "Implement email and push notifications"

# 2. Add a sub-task with --parent
forecast jira create --summary "Email service integration" \
  --type Sub-task --parent SMG-456 \
  --labels "size:M,type:integration"

# 3. Start work
forecast jira transition SMG-456 --to "In Progress"

# 4. Complete and sync
forecast jira transition SMG-456 --to "Done" --resolution Done --comment "Complete"
forecast sync
```

### Sprint Operations

```bash
# Pull this sprint into focus
forecast jira sprints                            # find active sprint ID
forecast jira search "sprint = <id> AND assignee = currentUser()"

# Move tickets in/out of the sprint
forecast jira sprint-add 42 SMG-100 SMG-101
forecast jira sprint-backlog SMG-099             # de-prioritize
```

### Bulk Triage

```bash
# Preview, then apply, a bulk transition over a query
forecast jira bulk transition --jql "project=SMG AND status='To Do' AND created < -30d" \
  --to "Won't Do" --resolution "Won't Do" --dry-run

# Bulk-add a label to an entire epic
forecast jira bulk label --jql "'Epic Link' = SMG-100" --add phase1-archive
```

### Getting a Project Forecast

```bash
# 1. Sync latest data
forecast sync

# 2. Run Monte Carlo simulation
forecast run --confidence 50,70,85,95

# 3. Generate detailed report
forecast report --type full
```

## Size Labels Reference

| Size | Hours | Use For |
|------|-------|---------|
| `size:S` | 2-4h | Bug fixes, config changes, simple features |
| `size:M` | 4-12h | Standard features, new components |
| `size:L` | 12-24h | Complex features, multi-module changes |
| `size:XL` | 24h+ | Major changes (break down if possible) |

## Type Labels Reference

| Label | Use For |
|-------|---------|
| `type:component` | New UI component or module |
| `type:fix` | Bug fix |
| `type:migration` | Data or code migration |
| `type:integration` | External system integration |
| `type:test` | Test infrastructure |
| `type:extraction` | Extract to shared library |
| `type:documentation` | Documentation work |
| `type:refactor` | Code refactoring |

## Report Metrics Explained

### EVA Report

| Metric | Meaning |
|--------|---------|
| **PV** (Planned Value) | Items planned to be complete by now |
| **EV** (Earned Value) | Items actually completed |
| **SV** (Schedule Variance) | EV - PV (positive = ahead) |
| **SPI** (Schedule Performance Index) | EV / PV (>1 = ahead, <1 = behind) |

### Monte Carlo Forecast

| Confidence | Meaning |
|------------|---------|
| 50% | Median estimate - as likely to be early as late |
| 70% | Reasonable planning date |
| 85% | Conservative estimate |
| 95% | High-confidence buffer for commitments |

## Configuration

Config file location: `.forecast/config.yaml`

```yaml
project_name: "My Project"
team_size: 3
team_capacity: 8  # hours per day

jira:
  url: https://company.atlassian.net
  email: you@company.com
  api_token: YOUR_TOKEN  # or use ${JIRA_API_TOKEN}
  project_key: PROJ
  epic: PROJ-100        # optional: filter by epic
  labels:               # optional: filter by labels
    - phase1

# Track multiple projects/initiatives by Epic
projects:
  - name: "Monorepo Migration"
    key: "monorepo"           # Short key for --project flag
    epic: "PROJ-100"          # Epic key in JIRA
    capacity: 8               # Optional: team hours/day for this project
  - name: "Feature X"
    key: "feature-x"
    epic: "PROJ-200"
  - name: "Infrastructure"
    key: "infra"
    epic: "PROJ-300"
    capacity: 4               # Smaller team allocation

jira_mapping:
  item_type:
    field: labels
    mappings:
      - jira: "type:component"
        forecast: "Component"
  size:
    field: labels
    mappings:
      - jira: "size:S"
        forecast: "S"
```

## Multi-Project Dashboard

Track 2-3 initiatives simultaneously:

```bash
# View all projects at a glance
forecast dashboard

# Output:
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#   PROJECT DASHBOARD
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Project              Epic         Total     Done Progress   70% Conf   95% Conf
# ──────────────────────────────────────────────────────────────────────────
# Monorepo Migration   PROJ-100        45       12    26.7%     Mar 15     Apr 1
# Feature X            PROJ-200        23        8    34.8%     Feb 28    Mar 10
# Infrastructure       PROJ-300        12        3    25.0%     Mar 20     Apr 5

# Detailed view for specific project
forecast dashboard --project monorepo

# Filter reports/forecasts to specific project
forecast run --project monorepo
forecast report --type eva --project monorepo
```

## Troubleshooting

### Authentication errors
```bash
# Verify token works
forecast jira priorities

# Check config has correct email and token
cat .forecast/config.yaml
```

### Project not found errors
```bash
# List all accessible projects
forecast jira projects

# Verify project_key in config matches an available project
# Common issue: running from wrong directory (no .forecast/config.yaml)
```

### No forecast data
```bash
# Need completed items with cycle time
forecast jira search "project=PROJ AND status=Done"

# Ensure items were moved through workflow properly
# (To Do → In Progress → Done)
```

### Forecast seems wrong
- Need 15+ completed items for accuracy
- Check that status transitions are being tracked
- Run `forecast sync` to refresh data
- Check average cycle time in report
