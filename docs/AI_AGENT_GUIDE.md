# AI Agent Guide: Probabilistic Forecasting

## Instructions for AI Assistants

When working on a software project that uses probabilistic forecasting and the forecast tool, follow these guidelines.

## Core Principles

1. **Never give time estimates** - Use relative sizes (S/M/L/XL) instead
2. **Always add size and type labels** - Every ticket needs `size:X` and `type:X` labels
3. **Update JIRA properly** - Follow the workflow: transition to "In Progress" → complete → transition to "Done"
4. **Sync forecast data** - Run `forecast sync` after completing work
5. **Use actual data** - Reference historical cycle times, not estimates

## When Starting Work on a Ticket

**CRITICAL: When you implement features, YOU MUST actually execute these forecast commands, not just show them as examples.**

**Step-by-step workflow:**

```bash
# 1. View the ticket details
forecast jira get {TICKET-KEY}

# 2. Check if it has size and type labels
# If missing, add them:
forecast jira update {TICKET-KEY} --labels "size:M,type:component"

# 3. Move to "In Progress" (starts cycle time tracking) - EXECUTE THIS
forecast jira transition {TICKET-KEY} --to "In Progress"

# 4. Do the work...
# (implement feature, write tests, etc.)

# 5. When complete, move to Done with summary comment - EXECUTE THIS
forecast jira transition {TICKET-KEY} --to "Done" --comment "Completed: [summary of work]

Files modified:
- [list of files]

Tests: [status]"

# 6. Sync forecast data - EXECUTE THIS
forecast sync
```

**When to execute these commands:**

- ✅ **DO execute** when implementing a feature the user requested
- ✅ **DO execute** when user says "work on ticket SMG-123"
- ✅ **DO execute** when you complete any work
- ❌ **DON'T execute** when just answering questions or explaining concepts
- ❌ **DON'T execute** when user is asking "how do I..." or requesting documentation

## Sizing Guidelines for AI Agents

When you need to size a ticket, use these guidelines:

### S (Small) - 2-4 hours focused work

- Bug fixes in isolated code
- Configuration changes
- Copy/text updates
- Well-defined, simple features
- Examples:
  - "Fix typo in error message"
  - "Add new environment variable"
  - "Update button color to match design"

### M (Medium) - 4-12 hours focused work

- Standard feature implementation
- New component with clear requirements
- Refactoring a single module
- Standard integration work
- Examples:
  - "Implement user profile page"
  - "Add email validation to signup form"
  - "Extract authentication logic to separate service"

### L (Large) - 12-24 hours focused work

- Complex features requiring investigation
- Changes spanning multiple modules
- Integration with unfamiliar external system
- Significant refactoring
- Examples:
  - "Implement real-time notifications system"
  - "Migrate from REST to GraphQL"
  - "Add multi-language support"

### XL (Extra Large) - 24+ hours focused work

- Major architectural changes
- Requires significant research/spike work
- High uncertainty or complexity
- **Recommendation:** Break into smaller tickets
- Examples:
  - "Redesign authentication system"
  - "Migrate entire codebase to TypeScript"
  - "Implement real-time collaborative editing"

### When in Doubt

1. **Default to Medium** if unclear
2. **Size up** if there's uncertainty
3. **Ask the user** if the scope is ambiguous

## Responding to Estimation Requests

### ❌ Wrong: Giving Time Estimates

```
User: "How long will it take to implement user authentication?"
AI: "That will take about 3-5 days"
```

### ✅ Correct: Using Relative Sizing

```
User: "How long will it take to implement user authentication?"
AI: "Let me break this down into sized items:

1. User registration flow - M (size:M, type:component)
2. Login/logout functionality - M (size:M, type:component)
3. Password reset flow - M (size:M, type:component)
4. Session management - S (size:S, type:component)
5. Email verification - S (size:S, type:integration)

That's 3 Medium and 2 Small items total.

To get a probabilistic forecast, I'll create these tickets and run:
```bash
forecast sync
forecast run
```

Would you like me to create these tickets in JIRA?"
```

## Providing Project Forecasts

When asked about timelines or project completion:

```bash
# 1. Sync latest data
forecast sync

# 2. Run Monte Carlo simulation
forecast run --confidence 50,70,85,95

# 3. Generate report
forecast report
```

**Then interpret the results for the user:**

```
Based on the current forecast data:

Remaining work: 23 items (65% complete)
Average cycle time: 8.5 hours per item

Probabilistic forecast:
- 50% confidence: 12 working days
- 70% confidence: 16 working days
- 85% confidence: 21 working days
- 95% confidence: 28 working days

This means:
- We have a 70% chance of completing by [DATE]
- For high confidence (95%), plan for [DATE]

Current velocity: 2.3 items/day
Schedule Performance Index: 0.92 (slightly behind plan)

Recommendations:
- [based on SPI/CPI metrics]
```

## Handling "When Will This Be Done?" Questions

### User asks: "When will the authentication feature be done?"

**Your response process:**

1. **Check if tickets exist and are sized**

   ```bash
   forecast jira search "project=SMG AND summary ~ 'auth*'"
   ```

2. **If tickets don't exist, break down the work first**

   - Create sized tickets (don't estimate time)
   - Add to JIRA
   - Then run forecast

3. **Run forecast with current data**

   ```bash
   forecast sync
   forecast run
   ```

4. **Provide probabilistic answer**

   ```
   Based on the current data:

   - 70% confidence: November 15
   - 95% confidence: November 22

   This accounts for 5 Medium items and 2 Small items, based on your team's
   historical cycle time of ~9 hours per Medium item.

   Would you like to see the detailed breakdown?
   ```

## When Completing Work

**YOU MUST execute these commands after implementing any feature.**

After finishing implementation:

```bash
# Move to Done with completion comment - EXECUTE THIS
forecast jira transition {TICKET-KEY} --to "Done" --comment "Implementation complete:

Changes:
- Implemented user authentication flow
- Added JWT token handling
- Created login/logout endpoints
- Added password hashing with bcrypt

Files modified:
- src/auth/login.ts
- src/auth/session.ts
- src/middleware/auth.ts
- tests/auth.test.ts

Tests: All passing (12 new tests added)
Coverage: 94% on auth module"

# Sync forecast data - EXECUTE THIS
forecast sync

# Show updated forecast - EXECUTE THIS
forecast report
```

## Integration with Existing Workflows

### Complete Real-World Example: EP2-3

This example shows the complete workflow from ticket creation through completion with follow-up tasks.

**User Request**: "Extract auth and session logic to libs/core"

**Step 1: Create and size the ticket**

```bash
forecast jira create --summary "Extract Auth/Session to libs/core" \
  --type Task \
  --description "Move Auth and Session logic to shared library." \
  --labels "size:M,type:refactor"
# Created: EP2-3
```

**Step 2: Create implementation plan, get approval**

- Create `implementation_plan.md` artifact
- Use `notify_user` to request approval with plan path

**Step 3: Move to In Progress**

```bash
forecast jira transition EP2-3 --to "In Progress"
```

**Step 4: Implement** → Create SessionManager, tests, ApiClient integration

**Step 5: Complete**

```bash
forecast jira transition EP2-3 --to "Done" --comment "Complete!
- 25 tests, 100% coverage
- Build successful
See walkthrough.md"
```

**Step 6: Create follow-up tasks**

```bash
forecast jira create --summary "Refactor StoreContextProvider to use SessionManager" \
  --type Task \
  --labels "size:S,type:refactor"
```

**Step 7: Sync forecast**

```bash
forecast sync
```

When showing forecast results, always explain:

1. **What the numbers mean**

   ```
   "70% confidence means if we ran this project 10 times, we'd finish
   by this date in 7 out of 10 cases"
   ```

2. **The uncertainty range**

   ```
   "The range from 70% to 95% shows our uncertainty. Wider range =
   more variability in historical cycle times"
   ```

3. **What affects the forecast**

   ```
   "This forecast assumes:
   - Similar complexity to past items
   - No major blockers
   - Current team size and availability
   - Historical cycle time of X hours per item"
   ```

4. **How to improve confidence**
   ```
   "To increase confidence or move the date earlier:
   - Reduce scope (remove items)
   - Address bottlenecks (current SPI is 0.85)
   - Add more completed items (we only have 8, need 15+ for accuracy)"
   ```

## Implementation Plan Workflow

When tackling complex tickets that require planning:

### 1. Create Implementation Plan Artifact

```markdown
# EP2-XXX: Feature Name

## Overview

Brief description of the work

## Proposed Changes

List files and modifications by component

## Verification Plan

How you'll test and validate

## Dependencies/Risks

Any concerns or blockers
```

### 2. Request User Approval

Use `notify_user` tool with:

- `PathsToReview`: ["/path/to/implementation_plan.md"]
- `BlockedOnUser`: true
- `ConfidenceScore`: 0.0-1.0 based on plan certainty

### 3. Upon Approval - Start Work

```bash
forecast jira transition EP2-XXX --to "In Progress"
```

### 4. Complete Implementation & Update Ticket

```bash
# Move to Done with deliverables summary
forecast jira transition EP2-XXX --to "Done" --comment "Implementation complete!

Results:
- SessionManager: 14 tests, 100% coverage
- Build successful
- All modules exported

See walkthrough.md for details."

# Sync with forecast tool
forecast sync
```

### 5. Create Follow-Up Tasks for Next Steps

If implementation reveals additional work:

```bash
# Create new tasks for follow-up work
forecast jira create --summary "Follow-up task name" \
  --type Task \
  --description "Detailed description with dependencies" \
  --labels "size:S,type:refactor"
```

## Dos and Don'ts

### ✅ Do:

- Break work into S/M/L/XL sized items
- Update JIRA status as you work
- Run `forecast sync` after completing items
- Provide probabilistic forecasts with confidence levels
- Reference historical data from the forecast tool
- Explain uncertainty honestly

### ❌ Don't:

- Give time estimates in hours or days for individual items
- Promise specific completion dates without running forecast
- Update ticket status without actually doing the work
- Skip adding size labels
- Ignore the forecast tool's output
- Give false precision ("exactly 3.5 days")

## Useful Commands Reference

```bash
# View your tickets
forecast jira search "assignee=currentUser() AND status not in (Done, Canceled)"

# View specific ticket
forecast jira get {TICKET-KEY}

# Create ticket with size
forecast jira create --summary "Task name" \
  --type Task \
  --labels "size:M,type:component"

# Start work
forecast jira transition {TICKET-KEY} --to "In Progress"

# Complete work
forecast jira transition {TICKET-KEY} --to "Done" --comment "Completion summary"

# Update ticket labels
forecast jira update {TICKET-KEY} --labels "size:M,type:component"

# List available transitions
forecast jira transitions {TICKET-KEY}

# Forecast commands
forecast sync                    # Sync from JIRA
forecast report                  # Show project status
forecast run                     # Run Monte Carlo simulation
forecast run --confidence 70,95  # Specific confidence levels
```

## Example Conversation Flow

```
User: "We need to add user notifications. How long will that take?"

AI: "Let me break down user notifications into sized items:

1. **Email notifications** - M (type:integration)
   - Configure email service
   - Create templates
   - Send on user actions

2. **In-app notifications** - M (type:component)
   - Notification UI component
   - Real-time updates
   - Mark as read functionality

3. **Push notifications** - L (type:integration)
   - Mobile push setup
   - Service worker for web
   - Notification preferences

That's 2 Medium and 1 Large item.

Let me check your current forecast:

```bash
forecast sync && forecast report
```

Current project status:
- Completed: 18 items
- Remaining: 23 items
- Avg cycle time: 9.2 hours per Medium item

Adding these 3 items (2M + 1L = ~35 hours of cycle time):

```bash
forecast run
```

Forecast with notifications included:
- 70% confidence: December 15 (was December 10)
- 95% confidence: December 28 (was December 22)

**This adds approximately 5-6 working days to the project.**

Would you like me to:
A) Create these tickets in JIRA
B) Break down the Large item into smaller pieces
C) Show alternative scoping options?"
```

## Critical Reminders

1. **The goal is NOT to predict the future perfectly** - It's to make uncertainty visible and enable better decisions

2. **Forecasts improve over time** - More historical data = better predictions

3. **Always express uncertainty** - "70% confidence" not "will be done by"

4. **Focus on helping users make informed decisions** - Not defending a number

5. **Update data frequently** - Run `forecast sync` often to keep forecasts current
