# Developer Guide: Day-to-Day Workflow

## Daily Workflow

### 1. Starting Your Day

View your current tickets:
```bash
jira issue list --status "In Progress" -a$(jira me) --plain --columns KEY,SUMMARY,UPDATED
jira issue list --status "To Do" -a$(jira me) --plain --columns KEY,SUMMARY,PRIORITY
```

### 2. Starting Work on a Ticket

**Before you start coding:**

1. **Review the ticket** - Read the description, acceptance criteria, and any linked documents
2. **Check if it's sized** - Look for a `size:S`, `size:M`, `size:L`, or `size:XL` label
3. **Validate the size** - Does this feel right? If not, discuss with the team

**If it's not sized, size it now:**
```bash
# Add the appropriate size label
jira issue edit SMG-123 --label "size:M"

# Also add a type label if missing
jira issue edit SMG-123 --label "type:component"
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
jira issue move SMG-123 "In Progress"
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
# Add a comment explaining the blocker
jira issue comment add SMG-123 "Blocked waiting for API documentation from Platform team"

# Move to "Blocked" status if your team uses it
jira issue move SMG-123 "Blocked"
```

**If you realize the size is wrong:**
```bash
# Remove old size label
jira issue edit SMG-123 --remove-label "size:M"

# Add correct size label
jira issue edit SMG-123 --label "size:L"

# Add comment explaining why
jira issue comment add SMG-123 "Re-sized from M to L - discovered we need to migrate 3 additional components"
```

This is **normal and expected**. We learn as we work. The data will show the actual cycle time.

### 4. Completing Work

When your code is merged and deployed:

```bash
# Add a completion comment
jira issue comment add SMG-123 "Completed:
- Implemented user authentication flow
- Added unit and integration tests
- Updated API documentation
- Deployed to staging

Files modified: auth/login.ts, auth/session.ts, tests/auth.test.ts"

# Move to Done
jira issue move SMG-123 "Done"
```

This stops the **cycle time clock**. The forecast tool will now include this item in future predictions.

**Update forecast data:**
```bash
forecast sync
```

### 5. End of Day

Optional but recommended:
```bash
# Add a quick log of what you worked on
jira issue comment add SMG-123 "EOD: Completed authentication flow, tests passing. Tomorrow: Deploy to staging and verify."
```

## Weekly Habits

### Monday: Review the Week

```bash
# Sync latest JIRA data
forecast sync

# View project status
forecast report

# Check your tickets
jira issue list -a$(jira me) --plain
```

### Friday: Reflect and Update

```bash
# Sync forecast data
forecast sync

# Generate report for standup
forecast report

# Review completed tickets - were the sizes accurate?
jira issue list --status "Done" -a$(jira me) \
  --updated-after -7d \
  --plain --columns KEY,SUMMARY,CREATED,UPDATED
```

Ask yourself:
- Were any items mis-sized? Why?
- What took longer than expected?
- What can we learn for next week?

## Sizing Calibration Sessions

**Hold weekly 15-minute calibration sessions:**

1. **Review last week's completed tickets**
   ```bash
   jira issue list --status "Done" \
     --updated-after -7d \
     --plain --columns KEY,SUMMARY,LABELS
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
# 1. Pick up a ticket
jira issue view SMG-345
# (Review description, acceptance criteria)

# 2. Size it if not already sized
jira issue edit SMG-345 --label "size:M" --label "type:component"

# 3. Assign to yourself
jira issue assign SMG-345 $(jira me)

# 4. Move to In Progress (starts cycle time)
jira issue move SMG-345 "In Progress"

# 5. Do the work...
# (code, test, review)

# 6. Complete it
jira issue comment add SMG-345 "Completed user profile component with avatar upload"
jira issue move SMG-345 "Done"

# 7. Update forecast
forecast sync
```

## Helpful Aliases

Add to your `.zshrc` or `.bashrc`:

```bash
# Quick ticket status
alias jira-my="jira issue list -a\$(jira me) --plain"

# Start work on a ticket
function jira-start() {
  jira issue assign "$1" $(jira me)
  jira issue move "$1" "In Progress"
  echo "Started work on $1"
}

# Complete a ticket
function jira-done() {
  jira issue move "$1" "Done"
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
jira-done SMG-345
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
