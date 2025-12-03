# AI Agent Guide: Probabilistic Forecasting

## Instructions for AI Assistants

When working on a software project that uses probabilistic forecasting and the forecast tool, follow these guidelines.

## Core Principles

1. **Never give time estimates** - Use relative sizes (S/M/L/XL) instead
2. **Always add size and type labels** - Every ticket needs `size:X` and `type:X` labels
3. **Update JIRA properly** - Follow the workflow: assign → move to "In Progress" → complete → move to "Done"
4. **Sync forecast data** - Run `forecast sync` after completing work
5. **Use actual data** - Reference historical cycle times, not estimates

## When Starting Work on a Ticket

**CRITICAL: When you implement features, YOU MUST actually execute these JIRA commands, not just show them as examples.**

**Step-by-step workflow:**

```bash
# 1. View the ticket details
jira issue view {TICKET-KEY}

# 2. Check if it has size and type labels
# If missing, add them:
jira issue edit {TICKET-KEY} --label "size:M" --label "type:component"

# 3. Assign to yourself (use the configured user) - EXECUTE THIS
jira issue assign {TICKET-KEY} $(jira me)

# 4. Move to "In Progress" (starts cycle time tracking) - EXECUTE THIS
jira issue move {TICKET-KEY} "In Progress"

# 5. Do the work...
# (implement feature, write tests, etc.)

# 6. When complete, add summary comment - EXECUTE THIS
jira issue comment add {TICKET-KEY} "Completed: [summary of work]

Files modified:
- [list of files]

Tests: [status]

"

# 7. Move to "Done" (stops cycle time tracking) - EXECUTE THIS
jira issue move {TICKET-KEY} "Done"

# 8. Sync forecast data - EXECUTE THIS
forecast sync
```

**When to execute these JIRA commands:**

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

````
User: "How long will it take to implement user authentication?"
AI: "Let me break this down into sized items:

1. User registration flow - M (size:M, type:component)
2. Login/logout functionality - M (size:M, type:component)
3. Password reset flow - M (size:M, type:component)
4. Session management - S (size:S, type:component)
5. Email verification - S (size:S, type:integration)

That's 3 Medium and 2 Small items total.

Based on your project's .forecast/config.yaml, this epic would be:
SMG-XXX with labels: auth, phase1-refactor

To get a probabilistic forecast, I'll create these tickets and run:
```bash
forecast sync
forecast run
````

Would you like me to create these tickets in JIRA?"

````

## Providing Project Forecasts

When asked about timelines or project completion:

```bash
# 1. Sync latest data
forecast sync

# 2. Run Monte Carlo simulation
forecast run --confidence 50,70,85,95

# 3. Generate report
forecast report
````

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
   jira issue list --jql "project = SMG AND summary ~ 'auth*'" --plain
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
# Add completion comment with details - EXECUTE THIS
jira issue comment add {TICKET-KEY} "Implementation complete:

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

Tests: ✅ All passing (12 new tests added)
Coverage: 94% on auth module
"

# Move to Done - EXECUTE THIS
jira issue move {TICKET-KEY} "Done"

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
jira issue create -p EP2 -t Task \
  -s "Extract Auth/Session to libs/core" \
  -b "Move Auth and Session logic to shared library." \
  -l "size:M" -l "type:refactor" \
  --no-input
# Created: EP2-3
```

**Step 2: Create implementation plan, get approval**

- Create `implementation_plan.md` artifact
- Use `notify_user` to request approval with plan path

**Step 3: Update ticket body with full details**

```bash
jira issue edit EP2-3 -b "Extract Auth and Session logic to libs/core.

## Implementation Overview
Platform-agnostic auth modules for web and native reusability

### Components Delivered
- SessionManager class (session state management)
- ApiClient integration (auto-inject auth tokens)
- Auth TypeScript types
- Comprehensive unit tests (100% coverage)

### Files Modified
- libs/core/src/lib/auth/session-manager.ts (NEW)
- libs/core/src/lib/api/api-client.ts (MODIFIED)
- libs/core/src/index.ts (MODIFIED)

### Verification
✅ Unit tests (25 total, 100% coverage)
✅ Build successful

## Status
✅ Plan approved
🚧 In progress"
# Press Enter at prompts
```

**Step 4: Assign and start**

```bash
jira issue assign EP2-3 $(jira me)
jira issue move EP2-3 "In Progress"
```

**Step 5: Implement** → Create SessionManager, tests, ApiClient integration

**Step 6: Complete**

```bash
jira issue comment add EP2-3 "✅ Complete!
- 25 tests, 100% coverage
- Build successful
See walkthrough.md"

jira issue move EP2-3 "Done"
```

**Step 7: Create follow-up tasks**

```bash
jira issue create -p EP2 -t Task \
  -s "Refactor StoreContextProvider to use SessionManager" \
  -b "..." -l "size:S" -l "type:refactor" --no-input
jira issue link EP2-17 EP2-3 "Relates"

# Repeat for other follow-up tasks
```

**Step 8: Sync forecast**

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

### 3. Upon Approval - Update JIRA Ticket Body

**CRITICAL: Update the ticket BODY/DESCRIPTION with implementation details, NOT just comments.**

```bash
# Update ticket description with full implementation details
jira issue edit EP2-XXX -b "Extract Auth/Session logic to libs/core

## Implementation Overview
Platform-agnostic auth modules for web and native reusability

### Components
- SessionManager class (session state management)
- ApiClient integration (auto-inject auth tokens)
- Auth TypeScript types
- Comprehensive unit tests (100% coverage target)

### Files to Modify
- libs/core/src/lib/auth/session-manager.ts (NEW)
- libs/core/src/lib/api/api-client.ts (MODIFY)
- libs/core/src/index.ts (MODIFY - exports)

### Verification Plan
- Unit tests for SessionManager (target: 14+ tests)
- ApiClient integration tests
- Build verification (npx nx build core)
- Coverage report (target: 100%)

## Status
✅ Implementation plan approved
🚧 Implementation in progress

See implementation_plan.md artifact for full details."
```

**Note:** The `jira issue edit` command will prompt interactively:

1. It may ask to confirm the summary - press Enter to keep existing
2. It will show "What's next? Submit" - press Enter to confirm

**Alternative if prompts are problematic:** Add details as a comment initially, then manually update the description via JIRA web UI.

### 4. Assign and Move to In Progress

```bash
jira issue assign EP2-XXX $(jira me)
jira issue move EP2-XXX "In Progress"
```

### 5. Create Follow-Up Tasks (Recommended Approach)

**IMPORTANT**: Most JIRA projects don't support sub-tasks under Task types. Instead, create separate linked tasks.

**Why separate tasks are better:**

- ✅ Each task tracks its own cycle time independently
- ✅ More granular data for probabilistic forecasting
- ✅ Can be worked on in parallel if needed
- ✅ Clearer completion tracking

```bash
# Create follow-up task with proper description
jira issue create -p EP2 -t Task \
  -s "Refactor StoreContextProvider to use libs/core SessionManager" \
  -b "Update native app React Context to use SessionManager from libs/core.

## Dependencies
- EP2-3 (completed)

## Changes
- Import SessionManager from @workspace/core
- Replace local session state
- Update login/logout methods

## Verification
- All auth flows still work
- Session persists across restarts" \
  -l "size:S" -l "type:refactor" \
  --no-input

# Link to parent task
jira issue link EP2-17 EP2-3 "Relates"
```

**Available link types**: 'Blocks', 'Relates', 'Duplicate', etc.

- Use "Blocks" if task must complete before another
- Use "Relates" for general relationship

### 6. Complete Implementation & Update Ticket

```bash
# Add completion comment with deliverables
jira issue comment add EP2-XXX "✅ Implementation complete!

**Results:**
- SessionManager: 14 tests, 100% coverage
- Build successful
- All modules exported

See walkthrough.md for details."

# Move to Done (ends cycle time tracking)
jira issue move EP2-XXX "Done"

# Sync with forecast tool
forecast sync
```

### 7. Create Follow-Up Tasks for Next Steps

If implementation reveals additional work:

```bash
# Create new tasks for follow-up work
jira issue create -p EP2 -t Task \
  -s "Follow-up task name" \
  -b "Detailed description with dependencies" \
  -l "size:S" -l "type:refactor" \
  --no-input

# Link to completed parent
jira issue link EP2-NEW EP2-XXX "Relates"
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
jira issue list -a$(jira me) --plain

# View specific ticket
jira issue view {TICKET-KEY}

# Create ticket with size
jira issue create --project SMG --type Task \
  --summary "Task name" \
  --label "size:M" --label "type:component"

# Start work
jira issue assign {TICKET-KEY} $(jira me)
jira issue move {TICKET-KEY} "In Progress"

# Complete work
jira issue comment add {TICKET-KEY} "Completion summary"
jira issue move {TICKET-KEY} "Done"

# Forecast commands
forecast sync                    # Sync from JIRA
forecast report                  # Show project status
forecast run                     # Run Monte Carlo simulation
forecast run --confidence 70,95  # Specific confidence levels

# View epic status
jira issue list --parent {EPIC-KEY} --plain
```

## Example Conversation Flow

````
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
````

## Critical Reminders

1. **The goal is NOT to predict the future perfectly** - It's to make uncertainty visible and enable better decisions

2. **Forecasts improve over time** - More historical data = better predictions

3. **Always express uncertainty** - "70% confidence" not "will be done by"

4. **Focus on helping users make informed decisions** - Not defending a number

5. **Update data frequently** - Run `forecast sync` often to keep forecasts current
