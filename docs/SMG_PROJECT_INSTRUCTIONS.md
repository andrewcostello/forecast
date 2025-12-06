# SMG Project - AI Agent Instructions

**Add this to your AI agent's context when working on the SMG project.**

---

## Team Culture

**IMPORTANT:** We work as a team. Follow these guidelines:

1. **No individual credit** - Do not take credit for work or attribute it to yourself
2. **No AI attribution** - Do not reference Claude, GPT, or any AI agent in commits, comments, or documentation
3. **Team ownership** - All work belongs to the team, not individuals or tools

**In commit messages:**
- Do NOT include "Generated with Claude Code" or similar
- Do NOT include "Co-Authored-By: Claude" or any AI attribution
- Keep commits focused on what changed, not who/what did it

---

## Project Details

- **JIRA URL:** https://smgames.atlassian.net
- **Project Key:** SMG
- **Project Name:** Super Money Golf

## Active Projects/Initiatives

| Project | Key | Epic | Description |
|---------|-----|------|-------------|
| Monorepo & Deploy Policy | `monorepo` | SMG-1688 | Convert to monorepo, new deployment policy |

### Checking Project Status

```bash
# Dashboard of all projects
forecast dashboard

# Detailed view of specific project
forecast dashboard --project monorepo
```

## Creating Tickets for a Project

All tickets MUST include the `--epic` flag for the relevant project:

```bash
# For Monorepo initiative
forecast jira create \
  --summary "Your task" \
  --type Task \
  --labels "size:M,type:component" \
  --epic SMG-1688
```

## SMG Workflow Statuses

**IMPORTANT:** SMG uses custom status names. Use these exact values:

| Action | Use This Status |
|--------|-----------------|
| Start work | `"In Development"` (NOT "In Progress") |
| Complete work | `"Done"` |
| Blocked | `"Is Blocked"` |
| On hold | `"On Hold"` |
| Cancel | `"Canceled"` |
| Internal QA | `"In Internal QA"` |
| External QA | `"In External QA"` |
| Ready to deploy | `"Awaiting Dev Deployment"` |

## Creating Tickets

```bash
# Standard ticket under the monorepo epic
forecast jira create \
  --summary "Your ticket summary" \
  --type Task \
  --labels "size:M,type:component" \
  --epic SMG-1688

# Bug fix
forecast jira create \
  --summary "Fix something" \
  --type Bug \
  --labels "size:S,type:fix" \
  --epic SMG-1688
```

## Workflow Commands

```bash
# Start work (use "In Development" not "In Progress")
forecast jira transition SMG-XXXX --to "In Development"

# Complete work
forecast jira transition SMG-XXXX --to "Done" --comment "Summary of work"

# Mark blocked
forecast jira transition SMG-XXXX --to "Is Blocked" --comment "Blocked by..."

# Send to QA
forecast jira transition SMG-XXXX --to "In Internal QA"
```

## Available Issue Types

| Type | Use For |
|------|---------|
| Task | Standard work items |
| Bug | Defects and fixes |
| Story | User-facing features |
| Epic | Large initiatives (grouping) |
| Test | Test-related work |
| Investigate | Spike/research work |

## Size Labels

Always add a size label:

| Label | Hours | Use For |
|-------|-------|---------|
| `size:S` | 2-4h | Config changes, simple fixes |
| `size:M` | 4-12h | Standard features, components |
| `size:L` | 12-24h | Complex features, multi-module |
| `size:XL` | 24h+ | Break down if possible |

## Type Labels

Always add a type label:

- `type:component` - New component/module
- `type:fix` - Bug fix
- `type:migration` - Code/data migration
- `type:integration` - External integrations
- `type:refactor` - Refactoring work
- `type:infrastructure` - Build/deploy/tooling
- `type:documentation` - Docs

## Example: Complete Workflow

```bash
# 1. Create ticket under monorepo epic
forecast jira create \
  --summary "Set up Nx workspace configuration" \
  --type Task \
  --labels "size:L,type:infrastructure" \
  --epic SMG-1688
# Returns: SMG-1689

# 2. Start work (note: "In Development" not "In Progress")
forecast jira transition SMG-1689 --to "In Development"

# 3. Do the work...

# 4. Complete with summary
forecast jira transition SMG-1689 --to "Done" --comment "Configured Nx workspace with:
- nx.json base config
- workspace.json with initial projects
- Shared tsconfig paths
- Build and test targets"

# 5. Sync forecast data
forecast sync
```

## Forecasting This Initiative

To forecast just the monorepo epic:

```bash
# Update .forecast/config.yaml to filter by epic
# epic: SMG-1688

# Then sync and forecast
forecast sync
forecast run --confidence 50,70,85,95
forecast report --type full
```

## Key Reminders for AI Agents

1. **Always use `--epic SMG-1688`** for monorepo-related tickets
2. **Use `"In Development"`** to start work (not "In Progress")
3. **Always add size AND type labels**
4. **Run `forecast sync`** after completing work
5. **Check available transitions** with `forecast jira transitions SMG-XXXX` if unsure
