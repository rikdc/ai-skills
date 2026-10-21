# Eval report: skills/fixture

- Suite: `testdata/golden/suite`
- Agent: `claude -p --model fixture-model`
- Model: `fixture-model`
- Git: `deadbee`  ·  Created: 2026-01-02T03:04:05Z  ·  n: 2
- Agent invocations: 6
- Fixed denominator: the rubric sum; unjudged criteria remain pending and are never renormalised.

## Verdict: incomplete

runner failures or unjudged criteria remain; a verdict would be made up

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
| judge | $0.044 | $0.004 | $0.000 |
| total | $0.165 | $0.029 | -$0.004 |
| input tokens | 3.0k | 2.0k | 0 |
| output tokens | 360 | 240 | 0 |
| reasoning tokens | — | — | — |
| cache read | — | — | — |
| total tokens | 3.4k | 2.2k | 0 |

Coverage: agent 8/8, judge 6/8, total 6/8 (partial). Unknown usage is never counted as zero.
Efficiency: — (partial cost coverage or no positive score delta).

Cost totals include functional runs; delta and efficiency use comparison scenarios only.

## Timing

- Wall clock: 43s
- Agent: 1m
- Judge: —
- Runs: 8, timed out: 0

Raw JSON: `report.json`  ·  HTML: `report.html`
