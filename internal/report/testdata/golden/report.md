# Eval report: skills/fixture

- Suite: `testdata/golden/suite`
- Agent: `claude -p --model fixture-model`
- Model: `fixture-model`
- Git: `deadbee`  ·  Created: 2026-01-02T03:04:05Z  ·  n: 2
- Agent invocations: 6
- Fixed denominator: the rubric sum; unjudged criteria remain pending and are never renormalised.

## Verdict: incomplete

runner failures or unjudged criteria remain; a verdict would be made up

## Warnings

- **low-n**: n=2: fewer than 3 runs per arm, so run-to-run variance cannot be estimated reliably; treat the delta as indicative, not conclusive.

| With | Without | Delta | Won | Lost | Tied | Pending | Coverage | Failures |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 73.5 | 67.2 | +6.2 | 1 | 0 | 2 | 96 | 66.7% | 0 |

## Regressions

None.

## Scenarios

| Scenario | With | Without | Delta | Pending | Status |
| --- | --- | --- | --- | --- | --- |
| demo/compare | 95.0 | 82.5 | +12.5 | 0 pts | ok |
| demo/pending | 52.0 | 52.0 | 0.0 | 96 pts | ok |
| demo/functional | 85.0 | — | — | 0 pts | functional |

## Criteria

| Scenario | Criterion | With | Without | Delta | Status |
| --- | --- | --- | --- | --- | --- |
| demo/compare | B | 40.0 | 40.0 | = 0.0 | comparable |
| demo/pending | Scored | 52.0 | 52.0 | = 0.0 | comparable |
| demo/compare | A | 55.0 | 42.5 | ▲ +12.5 | comparable |
| demo/functional | A | 85.0 | — | — — | scored |
| demo/pending | Unjudged | — | — | — — | pending |

## Cost

| Cost | With | Without | Delta |
| --- | --- | --- | --- |
| agent | $0.131 | $0.035 | -$0.004 |
| judge | — | — | — |
| total | — | — | — |
| input tokens | — | — | — |
| output tokens | — | — | — |
| reasoning tokens | — | — | — |
| cache read | — | — | — |
| total tokens | — | — | — |

Coverage: agent 8/8, judge 6/8, total 6/8 (partial). Unknown usage is never counted as zero.
Efficiency: — (partial cost coverage or no positive score delta).

Cost totals include functional runs; delta and efficiency use comparison scenarios only.

## Expenditure

| Attempts | Agent | Hook | Judge | Known | Unknown | Not applicable | Known total |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 4 | 2 | 0 | 2 | 2 | 1 | 1 | $0.012 |

The known total is a lower bound: at least one applicable attempt has unknown usage or did not report every component. Missing usage is never counted as zero.

## Timing

- Wall clock: 43s
- Agent: 1m
- Judge: —
- Runs: 8, timed out: 0

Raw JSON: `report.json`  ·  HTML: `report.html`
