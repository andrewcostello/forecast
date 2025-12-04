# Quick Start Guide

## For Team Leads

### 1. Install Forecast Tool (5 minutes)

```bash
# Clone and build the forecast tool
git clone https://bitbucket.org/supermoneygames/forecast.git
cd forecast
go build -o forecast cmd/forecast/main.go

# Or install directly
go install bitbucket.org/supermoneygames/forecast/cmd/forecast@latest
```

### 2. Setup JIRA Credentials

```bash
# Create credentials directory
mkdir -p ~/.config/jira

# Save your JIRA API token
echo "your_api_token_here" > ~/.config/jira/credentials
chmod 600 ~/.config/jira/credentials
```

To get your JIRA API token:
1. Go to https://id.atlassian.com/manage-profile/security/api-tokens
2. Click "Create API token"
3. Copy and save the token

### 3. Setup Project (10 minutes)

```bash
# Navigate to your project directory
cd /path/to/your/project

# Initialize forecast
forecast init

# Edit .forecast/config.yaml
# - Set project_key to your JIRA project
# - Set epic key for tracking
# - Configure label mappings
```

Example `.forecast/config.yaml`:
```yaml
jira:
  url: https://yourcompany.atlassian.net
  email: your.email@company.com
  api_token_file: ~/.config/jira/credentials
  project_key: PROJ
  epic: PROJ-123
  labels:
    - phase1

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
      - jira: "size:L"
        forecast: "L"
      - jira: "size:XL"
        forecast: "XL"
```

### 4. Team Onboarding (30 minutes)

**Present to the team:**

1. **"We're changing how we plan"**
   - No more time estimates
   - Size work as S/M/L/XL instead
   - Use actual data to forecast

2. **"Why this is better for you"**
   - No pressure to commit to timelines
   - No blame for "wrong estimates"
   - Learn what actually takes time
   - Data-driven planning

3. **"The new workflow"**
   - Size every ticket (S/M/L/XL)
   - Add type labels (component, fix, etc.)
   - Move to "In Progress" when you start
   - Move to "Done" when complete
   - Run `forecast sync` after completing work

4. **Share documentation**
   - `docs/METHODOLOGY.md` - Why we're doing this
   - `docs/DEVELOPER_GUIDE.md` - Day-to-day workflow
   - Hold Q&A session

### 5. Initial Data Collection (2-4 weeks)

**Week 1-2: Size everything**
```bash
# Search for untagged tickets
forecast jira search "project=PROJ AND status='To Do' AND labels is EMPTY"

# Add size and type labels to each ticket
forecast jira update PROJ-123 --labels "size:M,type:component"
```

**Week 2-4: Track cycle times**
- Team works normally
- Update JIRA status as work progresses
- Run `forecast sync` daily
- Hold weekly calibration sessions

**Goal:** 15-20 completed items with cycle time data

### 6. First Forecast (Week 4+)

```bash
# Sync latest data
forecast sync

# Generate report
forecast report

# Run forecast
forecast run --confidence 50,70,85,95
```

**Share results with stakeholders:**
- Show probability ranges, not single dates
- Explain confidence levels
- Discuss scope vs. timeline tradeoffs

## For Developers

### Daily Workflow

```bash
# Morning: Check your tickets
forecast jira search "assignee=currentUser() AND status='In Progress'"

# Starting work - transition the ticket
forecast jira transition PROJ-123 --to "In Progress"

# ... do the work ...

# Completing work
forecast jira transition PROJ-123 --to "Done" --comment "Implemented feature X"
forecast sync
```

### Sizing Cheat Sheet

```bash
# S (Small) - 2-4 hours
# - Bug fixes, config changes, simple features

# M (Medium) - 4-12 hours
# - Standard features, new components, refactoring

# L (Large) - 12-24 hours
# - Complex features, multi-module changes

# XL (Extra Large) - 24+ hours
# - Break into smaller items if possible

# When in doubt: Size up or use M
```

### Helpful Aliases

Add to `.zshrc` or `.bashrc`:

```bash
# Quick ticket lookup
alias jira-get="forecast jira get"

# Search my tickets
alias jira-my="forecast jira search 'assignee=currentUser() AND status not in (Done, Canceled)'"

# Start work on a ticket
function jira-start() {
  forecast jira transition "$1" --to "In Progress"
  echo "✓ Started $1"
}

# Complete work on a ticket
function jira-complete() {
  forecast jira transition "$1" --to "Done" --comment "$2"
  forecast sync
  echo "✓ Completed $1"
}

# Forecast status
alias forecast-status="forecast sync && forecast report"
```

Usage:
```bash
jira-start PROJ-123
# ... do work ...
jira-complete PROJ-123 "Implemented the feature"
```

## For AI Assistants

### System Prompt Setup

1. Copy the contents of `docs/AI_SYSTEM_PROMPT.md`
2. Add to your AI assistant's custom instructions
3. Test with: "Help me implement user authentication"
4. Verify the AI sizes work (S/M/L/XL) instead of estimating time

### Integration Example

When working with an AI coding assistant:

```
You: "I need to add password reset functionality"

AI (correctly trained):
"I'll implement password reset and track it in JIRA.

Breaking this down:
1. Password reset flow - M (size:M, type:component)
2. Email template - S (size:S, type:component)
3. Token validation - S (size:S, type:component)

Creating JIRA tickets..."
```

## Common Issues & Solutions

### "Sizing feels arbitrary"

**Solution:** Hold calibration sessions weekly. After 2-3 weeks, patterns emerge:
- "Auth changes are always Large"
- "UI components are usually Small-Medium"
- "External integrations start as Large"

### "Team resists change"

**Solution:** Start small:
1. One project/epic only
2. Voluntary participation first
3. Share results after 4 weeks
4. Let data convince them

### "Stakeholders demand dates"

**Solution:** Give probability ranges:
- "70% chance by March 15, 95% chance by April 1"
- Let them choose risk level
- Show how cutting scope changes dates

### "Forecast is wildly inaccurate"

**Likely causes:**
1. Not enough data (need 15+ completed items)
2. Inconsistent sizing (hold calibration sessions)
3. Work types are too different (track separately)
4. Blocked time not tracked separately

**Solution:**
- Wait for more data
- Review sizing accuracy
- Segment by type if needed

### "Developers don't update JIRA"

**Solution:**
1. Make it part of daily workflow
2. Use aliases/scripts to reduce friction
3. Show how their data helps forecasting
4. Automate with git hooks if needed

## Weekly Rituals

### Monday: Planning

```bash
# Team lead runs forecast
forecast sync
forecast report
forecast run

# Share in standup:
# - Current completion %
# - SPI/CPI metrics
# - Updated forecast dates
# - Bottlenecks to address
```

### Wednesday: Calibration

```bash
# Review last week's completed work
forecast jira search "project=PROJ AND status=Done AND updated >= -7d"

# Discuss:
# - Were sizes accurate?
# - What took longer than expected?
# - What patterns are emerging?
```

### Friday: Reflection

```bash
# Sync and report
forecast sync
forecast report

# Share progress:
# - Items completed this week
# - Cycle time trends
# - Forecast updates
```

## Success Metrics (After 8 Weeks)

Track these to measure success:

1. **Sizing Consistency**
   - Target: 80%+ items don't need re-sizing
   - Measure: Weekly calibration reviews

2. **Forecast Accuracy**
   - Target: 70% confidence hits 65-75% of the time
   - Measure: Actual completion vs. forecast date

3. **Developer Satisfaction**
   - Target: Reduced stress about estimates
   - Measure: Anonymous survey

4. **Stakeholder Satisfaction**
   - Target: Better informed decision-making
   - Measure: Qualitative feedback

5. **Planning Overhead**
   - Target: <5% of total time
   - Measure: Time spent sizing and forecasting

## Next Steps

1. ✅ Read `METHODOLOGY.md` to understand the approach
2. ✅ Share `DEVELOPER_GUIDE.md` with your team
3. ✅ Configure `AI_SYSTEM_PROMPT.md` for AI assistants
4. ✅ Initialize your project with `forecast init`
5. ✅ Hold team onboarding session
6. ✅ Start sizing tickets and tracking cycle times
7. ✅ Run first forecast after 15-20 completed items
8. ✅ Hold weekly calibration sessions

## Resources

- **Docs:** `docs/` directory
- **Examples:** `examples/` directory (if created)
- **Issues:** https://bitbucket.org/supermoneygames/forecast/issues
- **Reading:**
  - *Thinking, Fast and Slow* by Daniel Kahneman
  - *How Big Things Get Done* by Bent Flyvbjerg
  - Troy Magennis's forecasting work

## Getting Help

### For Team Leads
Read: `METHODOLOGY.md` → Explains the "why"

### For Developers
Read: `DEVELOPER_GUIDE.md` → Day-to-day workflow

### For AI Integration
Read: `AI_AGENT_GUIDE.md` and `AI_SYSTEM_PROMPT.md`

### For Troubleshooting
Check: Common Issues section above

---

**Remember: This is a culture change, not just a tool change. Give it 8 weeks.**
