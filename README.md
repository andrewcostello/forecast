### Forecast - Probabilistic Software Project Forecasting

**Forecast** uses Reference Class Forecasting, Earned Value Analysis, and Monte Carlo simulation to provide probabilistic completion forecasts for software projects.

## Features

- **JIRA Integration** - Direct REST API integration for managing tickets and pulling cycle time data
- **Monte Carlo Simulation** - Probabilistic forecasts with confidence levels
- **Earned Value Analysis** - Track SPI, CPI, and earned schedule
- **Reference Class Database** - Build historical database for better predictions
- **Flow Metrics** - Throughput and cycle time tracking

## Installation

```bash
# Clone the repository
git clone https://bitbucket.org/supermoneygames/forecast.git
cd forecast

# Build
go build -o forecast cmd/forecast/main.go

# Install (optional)
go install cmd/forecast/main.go
```

## Quick Start

### 1. Initialize Project

```bash
cd /path/to/your/project
forecast init
```

This creates `.forecast/config.yaml` with default settings.

### 2. Configure JIRA

Edit `.forecast/config.yaml`:

```yaml
jira:
  url: https://yourcompany.atlassian.net
  email: your.email@company.com
  api_token_file: ~/.config/jira/credentials  # or use api_token: ${JIRA_API_TOKEN}
  project_key: PROJ
  epic: PROJ-123
  labels:
    - phase1-refactor
```

Set your JIRA API token (choose one method):

```bash
# Option 1: Environment variable
export JIRA_API_TOKEN="your_token_here"

# Option 2: Credentials file (recommended)
echo "your_token_here" > ~/.config/jira/credentials
chmod 600 ~/.config/jira/credentials
```

### 3. Sync Data

```bash
forecast sync
```

This pulls issue data from JIRA including cycle times.

### 4. Run Forecast

```bash
forecast run --confidence 50,70,85,95
```

Output:
```
Monte Carlo Simulation (10,000 runs)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Remaining: 41 items (77.4%)
Throughput: 2.1 items/day (last 7 days)
Avg Cycle Time: 2.8 hours

Forecast:
  50% confidence: 15 days  (Dec 6)
  70% confidence: 18 days  (Dec 9)
  85% confidence: 22 days  (Dec 13)
  95% confidence: 28 days  (Dec 19)
```

### 5. Generate Report

```bash
forecast report --type eva

# Or full report with all metrics
forecast report --type full
```

## Commands

### `forecast init`

Initialize forecasting for a project. Creates `.forecast/config.yaml`.

### `forecast sync`

Sync data from JIRA:
- Pull latest issues
- Calculate cycle times
- Update local database

### `forecast run`

Run Monte Carlo simulation:

```bash
# Default (50, 70, 85, 95 percentiles)
forecast run

# Custom confidence levels
forecast run --confidence 50,90,99

# More iterations for precision
forecast run --iterations 50000
```

### `forecast report`

Generate reports:

```bash
# EVA report only
forecast report --type eva

# Monte Carlo only
forecast report --type montecarlo

# Full report
forecast report --type full
```

### `forecast reference-class`

Manage reference class database:

```bash
# List available reference classes
forecast reference-class list

# Add current project to reference database
forecast reference-class add
```

### `forecast jira`

Full ticket lifecycle CLI on top of the JIRA REST + Agile APIs. Run
`forecast jira --help` for the grouped subcommand list, or
`forecast jira <cmd> --help` for flags. Highlights:

**Read & search**

```bash
# Ticket details: summary, description (rendered from ADF to Markdown),
# issue links with link IDs, and optional changelog
forecast jira get PROJ-123
forecast jira get PROJ-123 --history

# Comments and worklog (rendered from ADF; worklogs include a total)
forecast jira comments PROJ-123
forecast jira worklogs PROJ-123

# Watchers and attachments
forecast jira watchers PROJ-123
forecast jira attachments PROJ-123

# JQL search
forecast jira search "project=PROJ AND status='To Do'"
```

**Create & edit**

```bash
# Create with the full field surface (story points / due date / parent /
# fix versions / components are all optional)
forecast jira create --summary "Add dark mode" --type Story \
  --labels ui,feature --story-points 5 --due-date 2026-06-30

# Sub-task: --parent is distinct from --epic
forecast jira create --summary "Add UI toggle" --type Sub-task --parent PROJ-100

# Update fields (omit to leave unchanged; --clear-due-date to wipe)
forecast jira update PROJ-123 --priority Highest --story-points 8

# Comments: write supports markdown subset (## headings, - bullets, *bold*)
forecast jira comment PROJ-123 --body "Verified in staging."

# Attachments
forecast jira attach PROJ-123 ./screenshot.png
forecast jira download <attachment-id> --out screenshot.png
```

**Workflow & collaboration**

```bash
# Transitions can carry a comment and/or set the resolution
forecast jira transition PROJ-123 --to "In Development"
forecast jira transition PROJ-123 --to Done --comment "Shipped" --resolution Done

# Worklog (parses 2h, 30m, 1h30m, 1.5h)
forecast jira log PROJ-123 --time 2h --comment "pairing with @alex"

# Issue links
forecast jira link PROJ-100 --to PROJ-200 --type Blocks       # PROJ-100 blocks PROJ-200
forecast jira unlink <link-id>                                # IDs shown in `get`
forecast jira link-types

# Watchers (defaults to the authenticated user)
forecast jira watch PROJ-123
forecast jira unwatch PROJ-123 --user other@company.com
```

**Sprint & release**

```bash
forecast jira boards                              # discover board IDs
forecast jira sprints                             # active sprints; auto-discovers single board
forecast jira sprints --board 42 --state active,future
forecast jira sprint-add 42 PROJ-100 PROJ-101     # add issues to sprint 42
forecast jira sprint-backlog PROJ-100             # remove from any sprint
```

**Bulk over JQL**

```bash
forecast jira bulk transition --jql "project=PROJ AND status='To Do'" \
  --to "In Progress" --dry-run
forecast jira bulk label --jql "project=PROJ AND status=Done" --add archived
```

**Discovery**

```bash
forecast jira projects        # accessible projects
forecast jira types           # issue types
forecast jira priorities      # priorities
forecast jira resolutions     # resolutions
forecast jira link-types      # issue link types
forecast jira transitions PROJ-123   # valid transitions out of current status
forecast jira fields PROJ-123        # custom field IDs (find story_points_field, etc.)
forecast jira missing-times          # audit Done tickets without cycle time
```

> **Rich-text rendering.** Descriptions, comments, and worklog comments
> are stored in JIRA as Atlassian Document Format (ADF). The CLI renders
> them as Markdown for reading, and accepts a small markdown subset
> (`## headings`, `- bullets`, `*bold*`) for writing.

## Configuration

### JIRA Mapping

Map JIRA fields to forecast item types:

```yaml
jira_mapping:
  item_type:
    field: labels
    mappings:
      - jira: "type:component"
        forecast: "Component"
      - jira: "type:fix"
        forecast: "Fix"

  size:
    field: labels
    mappings:
      - jira: "size:S"
        forecast: "S"
      - jira: "size:M"
        forecast: "M"
```

### Item Types

Define work item types:

```yaml
item_types:
  - name: Component
    jira_label: type:component
  - name: Fix
    jira_label: type:fix
  - name: Migration
    jira_label: type:migration
```

### Reference Class

Categorize your project for reference database:

```yaml
reference_class:
  name: "My Project Phase 1"
  type: "React Refactoring"
```

## How It Works

### 1. Reference Class Forecasting

Uses historical data from similar projects to establish baseline estimates. As you complete more projects, predictions improve.

### 2. Monte Carlo Simulation

Samples from cycle time distributions 10,000+ times to generate probabilistic forecasts. Accounts for variability in work estimates.

### 3. Earned Value Analysis

Tracks:
- **PV** (Planned Value): Items planned to be complete
- **EV** (Earned Value): Items actually completed
- **SPI** (Schedule Performance Index): EV / PV
- **CPI** (Cost Performance Index): EV / AC

### 4. Flow Metrics

Monitors:
- **Throughput**: Items completed per day
- **Cycle Time**: Hours from "In Progress" → "Done"
- **WIP**: Work in progress

## Example Workflow

```bash
# Week 1: Initialize and sync
forecast init
forecast sync
forecast run

# Week 2: Update and check progress
forecast sync
forecast report --type eva
forecast run

# Week 3: Check if forecast changed
forecast sync
forecast run --confidence 50,85

# Project complete: Add to reference database
forecast reference-class add
```

## JIRA Setup

### Required JIRA Fields

1. **Epic Link** - Group related items
2. **Labels** - Tag item type and size
3. **Status** - Track workflow states

### Example JIRA Issue

```
Summary: Rewrite CodeVerificationInput component
Epic: Phase 1 Refactoring
Labels: type:component, size:M, phase1-refactor
Status: Done
```

### Getting JIRA API Token

1. Go to https://id.atlassian.com/manage-profile/security/api-tokens
2. Click "Create API token"
3. Copy token and either:
   - Set environment variable: `export JIRA_API_TOKEN="your_token"`
   - Save to file: `echo "your_token" > ~/.config/jira/credentials && chmod 600 ~/.config/jira/credentials`

## Reference Class Database

Location: `~/.forecast/reference_classes.db`

Stores historical data from all your projects. Over time, this database provides increasingly accurate predictions for similar work.

### Adding Projects

After completing a project:

```bash
cd /path/to/completed/project
forecast reference-class add
```

This adds all completed items to the reference database.

### Querying Reference Classes

```bash
forecast reference-class list

Output:
Available Reference Classes:

1. React Refactoring (5 projects, 234 items)
   Avg: 2.4 hrs/item, StdDev: 0.9 hrs

2. API Migration (3 projects, 87 items)
   Avg: 4.1 hrs/item, StdDev: 1.8 hrs
```

## Troubleshooting

### "No reference data" errors

Solution: Complete at least 20-30% of items before running Monte Carlo. The tool needs actual cycle time data to make predictions.

### JIRA authentication fails

Check:
1. API token is set (environment variable or credentials file)
2. Email matches your JIRA account
3. Token has correct permissions

### Forecast seems inaccurate

Possible causes:
1. **Insufficient data** - Need 15-20 completed items minimum
2. **Reference class mismatch** - Update reference_class.type in config
3. **Team changes** - Re-run simulation after team size changes

## Development

### Project Structure

```
forecast/
├── cmd/forecast/          # CLI entry point
├── internal/
│   ├── jira/             # JIRA REST API client
│   ├── montecarlo/       # Monte Carlo engine
│   ├── eva/              # Earned Value Analysis
│   ├── referenceclass/   # Reference database
│   ├── report/           # Report generation
│   └── config/           # Configuration
├── pkg/forecast/         # Public types
├── output/               # Generated reports (gitignored)
└── configs/              # Example configs
```

### Building

```bash
# Build
make build

# Test
make test

# Install
make install
```

### Adding New Features

1. **New JIRA fields**: Edit `internal/jira/client.go`
2. **New report types**: Edit `internal/report/markdown.go`
3. **New reference class types**: Edit `internal/referenceclass/database.go`

## Documentation

| Document | Audience | Description |
|----------|----------|-------------|
| [QUICK_START.md](docs/QUICK_START.md) | Team Leads | Setup guide and team onboarding |
| [COMMAND_REFERENCE.md](docs/COMMAND_REFERENCE.md) | All Users | Quick reference for all commands |
| [DEVELOPER_GUIDE.md](docs/DEVELOPER_GUIDE.md) | Developers | Day-to-day workflow guide |
| [METHODOLOGY.md](docs/METHODOLOGY.md) | Everyone | Why probabilistic forecasting works |
| [AI_AGENT_GUIDE.md](docs/AI_AGENT_GUIDE.md) | AI Agents | Instructions for AI assistants |
| [AI_SYSTEM_PROMPT.md](docs/AI_SYSTEM_PROMPT.md) | AI Config | System prompt for AI tools |

## Contributing

Contributions welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Add tests
4. Submit a pull request

## License

MIT License - see LICENSE file for details

## Credits

Based on research in:
- Reference Class Forecasting (Kahneman & Tversky)
- Earned Value Management (PMI)
- #NoEstimates and Probabilistic Forecasting (Troy Magennis)
- Flow Metrics (Vacanti & Reinertsen)
