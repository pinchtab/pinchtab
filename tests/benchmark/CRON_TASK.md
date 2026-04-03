# PinchTab Optimization Cron Task

This task runs every 25 minutes to continuously improve PinchTab.

## Task for Agent

Run the PinchTab benchmark optimization loop:

1. **Setup**
   - cd ~/dev/pinchtab/tests/benchmark
   - Ensure on feat/benchmark branch
   - Run ./run-optimization.sh to initialize

2. **Run Baseline Benchmark**
   - Execute BENCHMARK_TASKS.md (36 steps)
   - Record with ./record-step.sh
   - Use report from run-optimization.sh output

3. **Run Agent Benchmark**
   - Execute AGENT_TASKS.md (21 tasks)
   - Figure out commands from SKILL.md only
   - Record with ./record-agent-step.sh
   - Log all commands executed

4. **Analyze Results**
   - Compare pass rates
   - Review agent_commands.log
   - Identify failures and root causes
   - Check if agent used wrong endpoint/approach

5. **Propose 1 Improvement** (priority order)
   a) Fix PinchTab API/CLI bug
   b) Improve skill documentation
   c) Add verification to test case
   d) Add new test case

6. **Implement Change**
   - Make the improvement
   - Commit with clear message
   - Push to feat/benchmark

7. **Log Run**
   - Append summary to results/optimization_log.md
   - Include: pass rates, analysis, change made, commit hash

## Output Format

After completing, report:
- Baseline pass rate
- Agent pass rate
- Change made (or "No change needed")
- Commit hash (if change made)
- Suggestion for next run
