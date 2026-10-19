def diff($a; $b): if $a == null or $b == null then null else ($a - $b) end;
def avg($xs): if ($xs | length) == 0 then null else (($xs | add) / ($xs | length)) end;
def metric($usage; $key):
    if (($usage.known // false) == true) and (($usage[$key] | type) == "number")
    then $usage[$key] else null end;
def run_cost($run):
    if metric($run.usage; "cost_usd") != null
       and metric($run.judge_usage; "cost_usd") != null
    then (metric($run.usage; "cost_usd") + metric($run.judge_usage; "cost_usd"))
    else null end;
def run_combined($run; $key):
    metric($run.usage; $key) as $a
    | metric($run.judge_usage; $key) as $j
    | if $a == null or $j == null then null else ($a + $j) end;
def arm_cost($scenario; $arm):
    [$scenario.runs[] | select(.arm == $arm) | run_cost(.)]
    | map(select(. != null)) | avg(.);
def arm_combined($scenario; $arm; $key):
    [$scenario.runs[] | select(.arm == $arm) | run_combined(.; $key)]
    | map(select(. != null)) | avg(.);
def pair_sum($report):
    ($report.cost.with.combined.cost.mean) as $w
    | ($report.cost.without.combined.cost.mean) as $o
    | if $w == null or $o == null then null else ($w + $o) end;
def pair_tokens($report):
    ($report.cost.with.combined.tokens.mean) as $w
    | ($report.cost.without.combined.tokens.mean) as $o
    | if $w == null or $o == null then null else ($w + $o) end;
. as $current
| $base[0] as $baseline
| $current + {comparison: {
    baseline: {
        suite_dir: $baseline.metadata.suite_dir,
        created_at: $baseline.metadata.created_at,
        git_sha: $baseline.metadata.git_sha,
        n: $baseline.metadata.n,
        verdict: $baseline.verdict.state,
        with: $baseline.totals.with,
        without: $baseline.totals.without,
        delta: $baseline.totals.delta,
        cost: {
            agent_per_run: $baseline.cost.with.agent.cost.mean,
            judge_per_run: $baseline.cost.with.judge.cost.mean,
            with_per_run: $baseline.cost.with.combined.cost.mean,
            without_per_run: $baseline.cost.without.combined.cost.mean,
            pair_per_run: pair_sum($baseline)
        },
        tokens: {
            with_input_per_run: $baseline.cost.with.combined.tokens.input.mean,
            with_output_per_run: $baseline.cost.with.combined.tokens.output.mean,
            with_total_per_run: $baseline.cost.with.combined.tokens.mean,
            without_input_per_run: $baseline.cost.without.combined.tokens.input.mean,
            without_output_per_run: $baseline.cost.without.combined.tokens.output.mean,
            without_total_per_run: $baseline.cost.without.combined.tokens.mean,
            pair_input_per_run: (($baseline.cost.with.combined.tokens.input.mean) as $w
                | ($baseline.cost.without.combined.tokens.input.mean) as $o
                | if $w == null or $o == null then null else ($w + $o) end),
            pair_output_per_run: (($baseline.cost.with.combined.tokens.output.mean) as $w
                | ($baseline.cost.without.combined.tokens.output.mean) as $o
                | if $w == null or $o == null then null else ($w + $o) end),
            pair_total_per_run: pair_tokens($baseline)
        },
        timing: $baseline.timing
    },
    current: {
        suite_dir: $current.metadata.suite_dir,
        created_at: $current.metadata.created_at,
        git_sha: $current.metadata.git_sha,
        n: $current.metadata.n,
        verdict: $current.verdict.state,
        with: $current.totals.with,
        without: $current.totals.without,
        delta: $current.totals.delta,
        cost: {
            agent_per_run: $current.cost.with.agent.cost.mean,
            judge_per_run: $current.cost.with.judge.cost.mean,
            with_per_run: $current.cost.with.combined.cost.mean,
            without_per_run: $current.cost.without.combined.cost.mean,
            pair_per_run: pair_sum($current)
        },
        tokens: {
            with_input_per_run: $current.cost.with.combined.tokens.input.mean,
            with_output_per_run: $current.cost.with.combined.tokens.output.mean,
            with_total_per_run: $current.cost.with.combined.tokens.mean,
            without_input_per_run: $current.cost.without.combined.tokens.input.mean,
            without_output_per_run: $current.cost.without.combined.tokens.output.mean,
            without_total_per_run: $current.cost.without.combined.tokens.mean,
            pair_input_per_run: (($current.cost.with.combined.tokens.input.mean) as $w
                | ($current.cost.without.combined.tokens.input.mean) as $o
                | if $w == null or $o == null then null else ($w + $o) end),
            pair_output_per_run: (($current.cost.with.combined.tokens.output.mean) as $w
                | ($current.cost.without.combined.tokens.output.mean) as $o
                | if $w == null or $o == null then null else ($w + $o) end),
            pair_total_per_run: pair_tokens($current)
        },
        timing: $current.timing
    },
    delta: {
        with_score: diff($current.totals.with; $baseline.totals.with),
        without_score: diff($current.totals.without; $baseline.totals.without),
        effect: diff($current.totals.delta; $baseline.totals.delta),
        agent_cost_per_run: diff($current.cost.with.agent.cost.mean; $baseline.cost.with.agent.cost.mean),
        judge_cost_per_run: diff($current.cost.with.judge.cost.mean; $baseline.cost.with.judge.cost.mean),
        with_cost_per_run: diff($current.cost.with.combined.cost.mean; $baseline.cost.with.combined.cost.mean),
        without_cost_per_run: diff($current.cost.without.combined.cost.mean; $baseline.cost.without.combined.cost.mean),
        pair_cost_per_run: diff(pair_sum($current); pair_sum($baseline)),
        with_input_tokens_per_run: diff($current.cost.with.combined.tokens.input.mean; $baseline.cost.with.combined.tokens.input.mean),
        with_output_tokens_per_run: diff($current.cost.with.combined.tokens.output.mean; $baseline.cost.with.combined.tokens.output.mean),
        with_total_tokens_per_run: diff($current.cost.with.combined.tokens.mean; $baseline.cost.with.combined.tokens.mean),
        without_input_tokens_per_run: diff($current.cost.without.combined.tokens.input.mean; $baseline.cost.without.combined.tokens.input.mean),
        without_output_tokens_per_run: diff($current.cost.without.combined.tokens.output.mean; $baseline.cost.without.combined.tokens.output.mean),
        without_total_tokens_per_run: diff($current.cost.without.combined.tokens.mean; $baseline.cost.without.combined.tokens.mean),
        wall_seconds: diff($current.timing.wall_seconds; $baseline.timing.wall_seconds),
        agent_seconds: diff($current.timing.agent_seconds; $baseline.timing.agent_seconds),
        judge_seconds: diff($current.timing.judge_seconds; $baseline.timing.judge_seconds)
    },
    comparable_scenarios: ([
        $current.scenarios[] as $scenario
        | ($baseline.scenarios[]
           | select(.plugin == $scenario.plugin and .scenario == $scenario.scenario))
        | $scenario
    ] | length),
    current_scenarios: ($current.scenarios | length),
    baseline_scenarios: ($baseline.scenarios | length),
    scenarios: [
        $current.scenarios[] as $scenario
        | ($baseline.scenarios[]
           | select(.plugin == $scenario.plugin and .scenario == $scenario.scenario)) as $old
        | {
            plugin: $scenario.plugin,
            scenario: $scenario.scenario,
            current: {
                with: $scenario.arms.with.percent_mean,
                without: $scenario.arms.without.percent_mean,
                delta: $scenario.delta,
                cost_with_per_run: arm_cost($scenario; "with"),
                cost_without_per_run: arm_cost($scenario; "without"),
                input_with_per_run: arm_combined($scenario; "with"; "input_tokens"),
                output_with_per_run: arm_combined($scenario; "with"; "output_tokens"),
                input_without_per_run: arm_combined($scenario; "without"; "input_tokens"),
                output_without_per_run: arm_combined($scenario; "without"; "output_tokens")
            },
            baseline: {
                with: $old.arms.with.percent_mean,
                without: $old.arms.without.percent_mean,
                delta: $old.delta,
                cost_with_per_run: arm_cost($old; "with"),
                cost_without_per_run: arm_cost($old; "without"),
                input_with_per_run: arm_combined($old; "with"; "input_tokens"),
                output_with_per_run: arm_combined($old; "with"; "output_tokens"),
                input_without_per_run: arm_combined($old; "without"; "input_tokens"),
                output_without_per_run: arm_combined($old; "without"; "output_tokens")
            },
            delta: {
                with: diff($scenario.arms.with.percent_mean; $old.arms.with.percent_mean),
                without: diff($scenario.arms.without.percent_mean; $old.arms.without.percent_mean),
                effect: diff($scenario.delta; $old.delta),
                cost_with_per_run: diff(arm_cost($scenario; "with"); arm_cost($old; "with")),
                cost_without_per_run: diff(arm_cost($scenario; "without"); arm_cost($old; "without")),
                input_with_per_run: diff(arm_combined($scenario; "with"; "input_tokens"); arm_combined($old; "with"; "input_tokens")),
                output_with_per_run: diff(arm_combined($scenario; "with"; "output_tokens"); arm_combined($old; "with"; "output_tokens")),
                input_without_per_run: diff(arm_combined($scenario; "without"; "input_tokens"); arm_combined($old; "without"; "input_tokens")),
                output_without_per_run: diff(arm_combined($scenario; "without"; "output_tokens"); arm_combined($old; "without"; "output_tokens"))
            }
        }
    ]
}}
