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

### View & Search

```bash
# Get ticket details
forecast jira get SMG-123

# Search tickets (JQL)
forecast jira search "project=SMG AND status='To Do'"
forecast jira search "assignee=currentUser() AND status not in (Done, Canceled)"
forecast jira search "project=SMG AND labels in (size:L)" --limit 20

# List available options
forecast jira projects           # List all accessible projects (useful for debugging)
forecast jira types              # Issue types for project
forecast jira priorities         # Available priorities
forecast jira transitions SMG-123 # Transitions for a ticket
```

### Create & Update

```bash
# Create ticket
forecast jira create --summary "Fix login bug" --type Bug --priority High
forecast jira create --summary "Add feature" --type Story --labels "size:M,type:component"
forecast jira create --summary "Task" --type Task --assignee user@email.com --epic SMG-100

# Update ticket
forecast jira update SMG-123 --priority Highest
forecast jira update SMG-123 --labels "size:L,type:integration"
forecast jira update SMG-123 --assignee user@email.com
forecast jira update SMG-123 --summary "New summary"
```

### Workflow Transitions

```bash
# Move ticket to new status
forecast jira transition SMG-123 --to "In Progress"
forecast jira transition SMG-123 --to "Done"
forecast jira transition SMG-123 --to "Done" --comment "Completed the work"

# Check available transitions first
forecast jira transitions SMG-123
```

## Typical Workflows

### Starting Work on a Ticket

```bash
# 1. View ticket
forecast jira get SMG-123

# 2. Ensure it has size/type labels
forecast jira update SMG-123 --labels "size:M,type:component"

# 3. Start work (begins cycle time tracking)
forecast jira transition SMG-123 --to "In Progress"
```

### Completing Work

```bash
# 1. Mark done with summary
forecast jira transition SMG-123 --to "Done" --comment "Implemented feature X. Files: src/foo.ts, src/bar.ts"

# 2. Sync forecast data
forecast sync

# 3. Check updated forecast
forecast report --type eva
```

### Creating a New Feature

```bash
# 1. Create sized ticket
forecast jira create --summary "Add user notifications" \
  --type Story \
  --labels "size:L,type:component" \
  --description "Implement email and push notifications"

# 2. Start work
forecast jira transition SMG-456 --to "In Progress"

# 3. Complete and sync
forecast jira transition SMG-456 --to "Done" --comment "Complete"
forecast sync
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
