# Developer Guide: Day-to-Day Workflow

## Daily Workflow

### 1. Starting Your Day

View your current tickets:
```bash
# View tickets assigned to you
forecast jira search "assignee=currentUser() AND status='In Progress'"
forecast jira search "assignee=currentUser() AND status='To Do'"
```

### 2. Starting Work on a Ticket

**Before you start coding:**

1. **Review the ticket** - Read the description, acceptance criteria, and any linked documents
2. **Check if it's sized** - Look for a `size:S`, `size:M`, `size:L`, or `size:XL` label
3. **Validate the size** - Does this feel right? If not, discuss with the team

**If it's not sized, size it now:**
```bash
# Add the appropriate size label
forecast jira update SMG-123 --labels "size:M"

# Or add multiple labels at once
forecast jira update SMG-123 --labels "size:M,type:component"
```

**Sizing Guide:**
- **S (Small)**: < 4 hours of focused work
  - Simple bug fixes
  - Copy changes
  - Config updates
  - Well-understood, isolated changes

- **M (Medium)**: 4-12 hours of focused work
  - Standard feature implementation
  - New component with clear requirements
  - Refactoring a single module

- **L (Large)**: 12-24 hours of focused work
  - Complex feature requiring investigation
  - Changes spanning multiple modules
  - New integration with external system

- **XL (Extra Large)**: > 24 hours of focused work
  - Consider breaking into smaller items
  - Major architectural changes
  - Requires significant research/design

**Move ticket to "In Progress":**
```bash
forecast jira transition SMG-123 --to "In Progress"
```

This starts the **cycle time clock**. From this moment until "Done", we're measuring how long work actually takes.

### 3. During Development

**Focus on getting to "Done":**
- Write code
- Write/update tests
- Update documentation
- Review your own code
- Run tests locally

**If you get blocked:**
```bash
# Move to "Blocked" status if your team uses it
forecast jira transition SMG-123 --to "Blocked" --comment "Waiting for API documentation from Platform team"
```

**If you realize the size is wrong:**
```bash
# Update the labels (this replaces all labels, so include the ones you want to keep)
forecast jira update SMG-123 --labels "size:L,type:component"
```

This is **normal and expected**. We learn as we work. The data will show the actual cycle time.

### 4. Completing Work

When your code is merged and deployed:

```bash
# Move to Done with a completion comment
forecast jira transition SMG-123 --to "Done" --comment "Completed:
- Implemented user authentication flow
- Added unit and integration tests
- Updated API documentation
- Deployed to staging"
```

This stops the **cycle time clock**. The forecast tool will now include this item in future predictions.

**Update forecast data:**
```bash
forecast sync
```

### 5. End of Day

Optional but recommended - add a progress comment:
```bash
# You can add comments without transitioning using the JIRA web interface
# or check the ticket status
forecast jira get SMG-123
```

## Weekly Habits

### Monday: Review the Week

```bash
# Sync latest JIRA data
forecast sync

# View project status
forecast report

# Check your tickets
forecast jira search "assignee=currentUser() AND status not in (Done, Canceled)"
```

### Friday: Reflect and Update

```bash
# Sync forecast data
forecast sync

# Generate report for standup
forecast report

# Review completed tickets - were the sizes accurate?
forecast jira search "assignee=currentUser() AND status=Done AND updated >= -7d"
```

Ask yourself:
- Were any items mis-sized? Why?
- What took longer than expected?
- What can we learn for next week?

## Sizing Calibration Sessions

**Hold weekly 15-minute calibration sessions:**

1. **Review last week's completed tickets**
   ```bash
   forecast jira search "project=SMG AND status=Done AND updated >= -7d"
   ```

2. **Discuss surprises**
   - "SMG-234 was sized M but took 3 days - should have been L"
   - "SMG-235 was L but finished in 6 hours - should have been M"

3. **Update sizing guidelines**
   - "When integrating with external API, default to L not M"
   - "UI components in the design system are usually S"

4. **Re-size upcoming tickets based on learnings**

This is how teams calibrate over time. After 3-4 weeks, your sizing becomes very accurate.

## What NOT to Do

❌ **Don't estimate in hours** - Just size as S/M/L/XL
❌ **Don't pressure others for "better" sizes** - Size based on complexity, not desired timeline
❌ **Don't commit to deadlines based on sizes** - Let the forecast tool calculate probabilities
❌ **Don't skip adding size labels** - The forecast tool needs this data
❌ **Don't leave tickets in "In Progress" when blocked** - Use "Blocked" status to track separately
❌ **Don't move to "Done" before actually done** - This corrupts cycle time data

## JIRA Label Conventions

### Size Labels (required)
- `size:S` - Small
- `size:M` - Medium (default if unsure)
- `size:L` - Large
- `size:XL` - Extra Large

### Type Labels (required)
- `type:component` - New UI component or module
- `type:fix` - Bug fix
- `type:migration` - Data or code migration
- `type:integration` - External system integration
- `type:test` - Test infrastructure or additional test coverage
- `type:extraction` - Extract code to shared library
- `type:documentation` - Documentation work

### Optional Labels
- `tech-debt` - Technical debt paydown
- `spike` - Investigation/research work (measure separately)
- `priority:high` - For filtering, doesn't affect forecast

## Example: Full Ticket Lifecycle

```bash
# 1. Find and review a ticket
forecast jira get SMG-345

# 2. Size it if not already sized
forecast jira update SMG-345 --labels "size:M,type:component"

# 3. Move to In Progress (starts cycle time)
forecast jira transition SMG-345 --to "In Progress"

# 4. Do the work...
# (code, test, review)

# 5. Complete it
forecast jira transition SMG-345 --to "Done" --comment "Completed user profile component with avatar upload"

# 6. Update forecast
forecast sync
```

## Helpful Aliases

Add to your `.zshrc` or `.bashrc`:

```bash
# Quick ticket lookup
alias jira-get="forecast jira get"

# Search my open tickets
alias jira-my="forecast jira search 'assignee=currentUser() AND status not in (Done, Canceled)'"

# Search in-progress tickets
alias jira-wip="forecast jira search 'assignee=currentUser() AND status=\"In Progress\"'"

# Start work on a ticket
function jira-start() {
  forecast jira transition "$1" --to "In Progress"
  echo "Started work on $1"
}

# Complete a ticket
function jira-done() {
  local comment="${2:-Completed}"
  forecast jira transition "$1" --to "Done" --comment "$comment"
  echo "Completed $1 - syncing forecast data..."
  forecast sync
}

# Weekly status
alias forecast-status="forecast sync && forecast report"
```

Usage:
```bash
jira-start SMG-345
# ... do work ...
jira-done SMG-345 "Implemented the feature with tests"
```

## Tips for Accurate Sizing

### Ask These Questions:

1. **Have we done something similar before?** Use that as reference
2. **What's the scope of changes?** One file = S, one module = M, multiple modules = L
3. **How well do we understand it?** Unknown tech or requirements → size up
4. **Are there dependencies?** External teams or systems → size up
5. **Is there precedent?** First time doing X → size up; Tenth time → size down

### Red Flags for Under-Sizing:

- "This should be quick" (famous last words)
- "I know exactly how to do this" (hubris)
- "It's just like X, but..." (the "but" matters)
- Stakeholder pressure to make it smaller
- No acceptance criteria defined

### When in Doubt:

**Size up.** It's better to be pleasantly surprised than consistently underestimate.

If something repeatedly finishes faster than its size suggests, that's great data! Lower the size next time.

## Questions?

**"What if I don't know which size?"**
→ Default to M, and adjust if needed as you learn more

**"What if I pick up someone else's ticket?"**
→ Validate the size first. If it feels wrong, re-size it

**"What if something takes much longer than expected?"**
→ Add a comment explaining why, and re-size if needed. This is valuable learning

**"Do I need to update JIRA every day?"**
→ At minimum: Move to "In Progress" when starting, move to "Done" when complete. Everything else is optional but helpful.

**"What about spike work / investigations?"**
→ Size the spike itself (usually S or M for time-boxed investigation), not the work that might result from it
