# Probabilistic Forecasting for Software Projects

## The Problem with Time Estimates

Traditional software estimation asks developers: "How long will this take?"

This question has fundamental problems:

1. **Pressure to commit** - Developers feel pressured to give a number, even with insufficient information
2. **Optimism bias** - We consistently underestimate complexity and ignore unexpected obstacles
3. **Planning fallacy** - We focus on the best-case scenario and discount past experience
4. **False precision** - "3 days" implies certainty that doesn't exist
5. **Anchoring** - Once stated, estimates become commitments that warp decisions

**Result:** Projects run late, developers are blamed for "bad estimates," and everyone is demoralized.

## A Better Way: Probabilistic Forecasting

Instead of asking "How long will this take?", we ask:

1. **"How big is this?"** (S, M, L, XL) - Relative complexity, not time
2. **"How long did similar items take?"** - Use actual historical data
3. **"What's the range of outcomes?"** - Express uncertainty as probability

This approach is based on **Reference Class Forecasting**, which won the Nobel Prize in Economics (Daniel Kahneman, 2002).

## Core Principles

### 1. Break Work Into Similar-Sized Items

Instead of estimating each task in hours, categorize by relative size:

- **S (Small)**: Simple, well-understood changes
- **M (Medium)**: Standard feature work
- **L (Large)**: Complex features requiring investigation
- **XL (Extra Large)**: Major architectural changes

Sizes are **relative to your team's context**, not absolute time.

### 2. Track Cycle Time

**Cycle Time** = Time from "Start Work" to "Done"

- **Start Work**: When someone begins coding (moves to "In Progress")
- **Done**: When code is merged and deployed

Track this for every completed item. This is your **actual historical data**.

### 3. Use Historical Data to Forecast

Once you have 10-20 completed items:

- Calculate the **distribution** of cycle times for each size
- Use **Monte Carlo simulation** to project completion dates
- Express forecasts as **probabilities**: "70% chance by March 15, 95% chance by April 1"

### 4. Reference Class Forecasting

Compare your current project to similar **past projects** (reference classes):

- "Last time we refactored authentication, it took 40 items over 8 weeks"
- "Mobile UI rewrites typically take 20% longer than estimated scope"
- "Backend service migrations average 65 hours of cycle time per 100 endpoints"

Build a database of completed projects to improve predictions.

## Benefits

### For Developers

✅ **No more "How long?" pressure** - Just assess relative size
✅ **No blame for "wrong estimates"** - Data drives forecasts, not individuals
✅ **Learn from actual data** - See what actually takes time
✅ **Sustainable pace** - Forecasts account for actual throughput, not wishful thinking

### For Managers

✅ **Probabilistic forecasts** - "70% by Q2, 95% by Q3" enables better decision-making
✅ **Early warning signals** - See when cycle times increase before deadlines slip
✅ **Data-driven planning** - Remove guesswork and politics from scheduling
✅ **Trend analysis** - Identify bottlenecks and process improvements

### For Organizations

✅ **Realistic commitments** - Stop overpromising based on optimistic estimates
✅ **Better resource allocation** - Know true capacity, not theoretical velocity
✅ **Risk management** - Quantify uncertainty and plan accordingly
✅ **Continuous improvement** - Historical data reveals what actually improves delivery

## How It Works: Earned Value Analysis

Track three metrics:

1. **Planned Value (PV)**: How much work you planned to complete by now
2. **Earned Value (EV)**: How much work is actually done
3. **Actual Cost (AC)**: How much time/effort you've spent

Calculate:

- **Schedule Performance Index (SPI)** = EV / PV
  - SPI < 1.0: Behind schedule
  - SPI = 1.0: On track
  - SPI > 1.0: Ahead of schedule

- **Cost Performance Index (CPI)** = EV / AC
  - CPI < 1.0: Over budget (taking longer than expected)
  - CPI = 1.0: On budget
  - CPI > 1.0: Under budget (faster than expected)

## How It Works: Monte Carlo Simulation

Given:
- 30 items remaining
- Historical cycle times: [8h, 12h, 6h, 15h, 9h, 11h, ...]

Simulation:
1. Run 10,000 simulations
2. Each simulation randomly samples from historical cycle times
3. Calculate completion date for each simulation
4. Sort results to get probability distribution

Output:
```
50% confidence: 45 days
70% confidence: 52 days
85% confidence: 61 days
95% confidence: 73 days
```

This gives stakeholders **informed choices about risk vs. timeline**.

## Comparison to Traditional Estimation

| Traditional Estimates | Probabilistic Forecasting |
|----------------------|---------------------------|
| "This will take 5 days" | "This is a Medium item" |
| "Project will finish March 1" | "70% chance by March 15, 95% by April 1" |
| Blame developers when wrong | Improve process when data changes |
| Political negotiation over dates | Data-driven conversation about scope/time/risk |
| Estimates made once, upfront | Forecasts updated continuously with new data |
| Ignores historical performance | Uses actual historical data |
| False precision ("3.5 days") | Honest uncertainty ("50-70% likely") |

## Common Questions

### "Don't we still need to tell stakeholders a date?"

Yes, but give them **probability ranges**:

- "We have a 70% chance of completing by March 15"
- "If we need 95% confidence, we should plan for April 1"

This enables better business decisions. If the conference is March 20, stakeholders can decide:
- Accept 70% risk
- Cut scope to increase confidence
- Move the conference date
- Add resources (if possible)

### "What if we have no historical data?"

Start with:
1. **External reference classes**: "Similar teams doing similar work averaged X days per item"
2. **Expert judgment** (temporarily): Experienced developers can ballpark initial sizes
3. **Start tracking immediately**: After 2 weeks you'll have some data; after 4 weeks it's useful

### "Doesn't this just move the guessing from time to size?"

Yes, but **sizing is much easier** because:
- Relative comparison ("bigger than X, smaller than Y") is more natural than absolute time
- Less emotional weight - "it's a Large" doesn't feel like a commitment
- Less anchoring - if you realize it's actually XL, you just recategorize
- Developers quickly calibrate on what S/M/L/XL means for their context

### "How do we handle different types of work?"

Track cycle times **per item type**:
- Backend features: avg 12h cycle time for Medium
- UI components: avg 8h cycle time for Medium
- Bug fixes: avg 4h cycle time for Medium
- Infrastructure: avg 20h cycle time for Medium

Run separate forecasts for each type, or weight the simulation accordingly.

### "What about dependencies and unknowns?"

1. **Dependencies**: Track "blocked" time separately from cycle time
2. **Unknowns**: Use XL size + spike work to gather information
3. **Learning**: First few items of new tech have longer cycle times (expected)

The forecast naturally accounts for these because they're in your historical data.

## Getting Started

### Week 1: Setup
1. Install the forecast tool: `go install bitbucket.org/supermoneygames/forecast/cmd/forecast@latest`
2. Run `forecast init` in your project directory
3. Configure JIRA integration in `.forecast/config.yaml`
4. Add labels to JIRA for sizes (size:S, size:M, size:L, size:XL) and types

### Week 2-4: Calibration
1. Start sizing new work as S/M/L/XL
2. Track cycle times for completed work
3. Hold a weekly calibration: "Was that really a Medium? Should it have been Large?"
4. Adjust sizing guidelines based on actual data

### Week 5+: Forecasting
1. Run `forecast sync` daily to pull latest JIRA data
2. Run `forecast report` weekly to review SPI/CPI
3. Run `forecast run` when stakeholders ask for projections
4. Update reference class database when projects complete

## Success Metrics

After 8 weeks, you should see:

1. **Sizing consistency**: 80%+ of items don't need re-sizing after completion
2. **Forecast accuracy**: 70% confidence forecasts hit 65-75% of the time
3. **Less rework**: Fewer "I thought it would be quick" surprises
4. **Better planning**: Stakeholders make informed risk/scope tradeoffs
5. **Developer satisfaction**: Less stress about "wrong estimates"

## Further Reading

- *Thinking, Fast and Slow* by Daniel Kahneman - Chapter on the Planning Fallacy
- *How Big Things Get Done* by Bent Flyvbjerg - Reference Class Forecasting in practice
- *The Mythical Man-Month* by Fred Brooks - Classic on why software estimation is hard
- Allen Holub's talks on #NoEstimates movement
- Troy Magennis's Monte Carlo forecasting work
