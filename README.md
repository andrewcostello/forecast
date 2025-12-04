###Forecast - Probabilistic Software Project Forecasting

**Forecast** uses Reference Class Forecasting, Earned Value Analysis, and Monte Carlo simulation to provide probabilistic completion forecasts for software projects.

## Features

- **JIRA Integration** - Automatically pull cycle time data from JIRA
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
  api_token: ${JIRA_API_TOKEN}
  project_key: PROJ
  epic: PROJ-123
  labels:
    - phase1-refactor
```

Set your JIRA API token:

```bash
export JIRA_API_TOKEN="your_token_here"
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
3. Copy token and set environment variable:
   ```bash
   export JIRA_API_TOKEN="your_token"
   ```

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
1. JIRA_API_TOKEN environment variable is set
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
│   ├── jira/             # JIRA API client
│   ├── montecarlo/       # Monte Carlo engine
│   ├── eva/              # Earned Value Analysis
│   ├── referenceclass/   # Reference database
│   ├── report/           # Report generation
│   └── config/           # Configuration
├── pkg/forecast/         # Public types
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
