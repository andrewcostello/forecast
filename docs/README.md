# Forecast Documentation

## Overview

Forecast is a probabilistic project forecasting tool based on Reference Class Forecasting and Monte Carlo simulation. Instead of asking "How long will this take?", it uses actual historical cycle time data to provide probability-based completion forecasts.

## Documentation Structure

### 📚 For Understanding the Approach
**[METHODOLOGY.md](./METHODOLOGY.md)**
- Why traditional estimates fail
- Introduction to Reference Class Forecasting
- How Monte Carlo simulation works
- Benefits for developers, managers, and organizations
- Comparison to traditional estimation

**Read this first** if you want to understand the "why" behind probabilistic forecasting.

---

### 👨‍💻 For Developers
**[DEVELOPER_GUIDE.md](./DEVELOPER_GUIDE.md)**
- Daily workflow with JIRA
- How to size tickets (S/M/L/XL)
- JIRA label conventions
- Helpful aliases and commands
- Tips for accurate sizing

**Read this** for day-to-day usage instructions.

---

### 🤖 For AI Integration
**[AI_AGENT_GUIDE.md](./AI_AGENT_GUIDE.md)**
- Comprehensive guide for AI assistants
- Sizing guidelines for AI
- Example conversation flows
- JIRA CLI integration examples
- Common patterns and anti-patterns

**[AI_SYSTEM_PROMPT.md](./AI_SYSTEM_PROMPT.md)**
- Copy-paste system prompt for AI assistants
- Concise rules and guidelines
- Quick reference for AI configuration

**Read these** to integrate AI coding assistants with the forecast workflow.

---

### 🚀 For Getting Started
**[QUICK_START.md](./QUICK_START.md)**
- Installation instructions
- Team onboarding guide
- First forecast setup
- Common issues and solutions
- Weekly rituals

**Read this** to get your team up and running quickly.

---

## Quick Navigation

**I want to...**

- **Understand why we're doing this** → Read [METHODOLOGY.md](./METHODOLOGY.md)
- **Learn the daily workflow** → Read [DEVELOPER_GUIDE.md](./DEVELOPER_GUIDE.md)
- **Set up AI assistants** → Read [AI_SYSTEM_PROMPT.md](./AI_SYSTEM_PROMPT.md)
- **Get started quickly** → Read [QUICK_START.md](./QUICK_START.md)
- **Integrate AI coding tools** → Read [AI_AGENT_GUIDE.md](./AI_AGENT_GUIDE.md)

---

## Key Concepts

### Relative Sizing
Work is sized as **S, M, L, or XL** based on complexity, not time.
- S (Small): 2-4 hours
- M (Medium): 4-12 hours
- L (Large): 12-24 hours
- XL (Extra Large): 24+ hours

### Cycle Time
The time from "Start Work" (In Progress) to "Done". This is your **actual data** that drives forecasts.

### Probabilistic Forecasting
Instead of "Will be done by March 1", we say:
- "70% chance by March 15"
- "95% chance by April 1"

This makes uncertainty visible and enables better decisions.

### Monte Carlo Simulation
Runs 10,000 simulations using historical cycle times to calculate probability distributions for completion dates.

### Reference Class Forecasting
Compare current work to similar past projects to improve predictions. Build a database of completed projects as reference classes.

---

## Tools Required

1. **JIRA CLI** - For JIRA integration
   ```bash
   brew install jira-cli
   jira init
   ```

2. **Forecast Tool** - This tool
   ```bash
   go install github.com/andrewcostello/forecast/cmd/forecast@latest
   ```

3. **JIRA Configuration**
   - Size labels: `size:S`, `size:M`, `size:L`, `size:XL`
   - Type labels: `type:component`, `type:fix`, etc.
   - Epic linkage for tracking

---

## Core Workflow

```mermaid
graph LR
    A[Size Ticket] --> B[Start Work]
    B --> C[Move to In Progress]
    C --> D[Complete Work]
    D --> E[Move to Done]
    E --> F[Sync Forecast]
    F --> G[Updated Predictions]
```

1. **Size** every ticket (S/M/L/XL)
2. **Start** work by moving to "In Progress"
3. **Complete** work and move to "Done"
4. **Sync** forecast data: `forecast sync`
5. **View** updated predictions: `forecast report`

---

## Example Commands

```bash
# Daily developer workflow
jira issue list -a$(jira me) --plain     # View your tickets
jira issue move SMG-123 "In Progress"    # Start work
jira issue move SMG-123 "Done"           # Complete work
forecast sync                             # Update data

# Forecasting
forecast sync                             # Pull latest JIRA data
forecast report                           # Project status
forecast run                              # Monte Carlo simulation
forecast run --confidence 70,95           # Specific confidence levels

# Sizing tickets
jira issue edit SMG-123 --label "size:M" --label "type:component"
```

---

## Success Metrics

After 8 weeks, you should see:

1. **80%+ sizing consistency** - Items don't need re-sizing
2. **Accurate forecasts** - 70% confidence hits 65-75% of time
3. **Reduced stress** - Developers not blamed for "wrong estimates"
4. **Better planning** - Stakeholders make informed risk decisions
5. **Faster delivery** - Less rework from incorrect assumptions

---

## Philosophy

> **"The goal is not to predict the future perfectly. It's to make uncertainty visible and enable better decisions."**

This approach:
- Removes pressure to commit to impossible deadlines
- Uses actual data instead of wishful thinking
- Expresses uncertainty as probability ranges
- Improves over time with more data
- Focuses on relative complexity, not absolute time

---

## Further Reading

### Books
- *Thinking, Fast and Slow* by Daniel Kahneman - Chapter on Planning Fallacy
- *How Big Things Get Done* by Bent Flyvbjerg - Reference Class Forecasting
- *The Mythical Man-Month* by Fred Brooks - Why estimation is hard

### Online Resources
- Allen Holub's #NoEstimates talks
- Troy Magennis's Monte Carlo forecasting work
- Jira's Probabilistic Forecasting features

---

## Getting Help

1. Check the [QUICK_START.md](./QUICK_START.md) troubleshooting section
2. Review common issues and solutions
3. Open an issue at https://github.com/andrewcostello/forecast/issues

---

## Contributing

Found an issue? Have a suggestion?

1. Read the docs first
2. Check existing issues
3. Open a new issue with details

---

**Version:** 1.0
**Last Updated:** 2025-11-21
**License:** MIT
