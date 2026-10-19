def diff($a; $b): if $a == null or $b == null then null else ($a - $b) end;
def avg($xs): if ($xs | length) == 0 then null else (($xs | add) / ($xs | length)) end;
def present_mean($xs): [$xs[] | select(. != null)] | avg(.);
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

# A scenario is only comparable when it is the same scenario and its rubric
# has the same criteria and maxima. A reused name with an edited rubric is a
# different measurement, not a delta.
def rubric($scenario): [$scenario.criteria[] | [.name, .max]] | sort;
def same_measurement($current; $baseline):
    ($current.plugin == $baseline.plugin)
    and ($current.scenario == $baseline.scenario)
    and (rubric($current) == rubric($baseline));

def scenario_arm_mean($scenarios; $arm):
    present_mean([$scenarios[] | .arms[$arm].percent_mean]);
def scenario_delta_mean($scenarios):
    present_mean([$scenarios[] | .delta]);
# Cost and token comparisons are per-run means over every matched run, matching
# the suite aggregate. Averaging per-scenario means would reweight scenarios
# when the two suites used different --runs counts.
def matched_arm_cost($scenarios; $arm):
    [$scenarios[] as $scenario
     | $scenario.runs[]
     | select(.arm == $arm)
     | run_cost(.)]
    | map(select(. != null)) | avg(.);
def matched_agent_cost($scenarios; $arm):
    [$scenarios[] as $scenario
     | $scenario.runs[]
     | select(.arm == $arm)
     | metric(.usage; "cost_usd")]
    | map(select(. != null)) | avg(.);
def matched_judge_cost($scenarios; $arm):
    [$scenarios[] as $scenario
     | $scenario.runs[]
     | select(.arm == $arm)
     | metric(.judge_usage; "cost_usd")]
    | map(select(. != null)) | avg(.);
def matched_pair_cost($scenarios):
    (matched_arm_cost($scenarios; "with")) as $w
    | (matched_arm_cost($scenarios; "without")) as $o
    | if $w == null or $o == null then null else ($w + $o) end;
def matched_arm_token($scenarios; $arm; $key):
    [$scenarios[] as $scenario
     | $scenario.runs[]
     | select(.arm == $arm)
     | run_combined(.; $key)]
    | map(select(. != null)) | avg(.);
def matched_pair_token($scenarios; $key):
    (matched_arm_token($scenarios; "with"; $key)) as $w
    | (matched_arm_token($scenarios; "without"; $key)) as $o
    | if $w == null or $o == null then null else ($w + $o) end;
def timing_total($scenarios; $key):
    [$scenarios[]
     | (.arms.with.timing[$key], .arms.without.timing[$key])
     | select(. != null)] as $values
    | if ($values | length) == 0 then null else ($values | add) end;
# Wall clock belongs to the whole suite run, not to a scenario, so it is only
# comparable when every scenario in the report is part of the matched set.
def matched_wall($matched; $report):
    if ($matched | length) == ($report.scenarios | length)
    then $report.timing.wall_seconds else null end;

. as $current
| $base[0] as $baseline
| [
    $current.scenarios[] as $scenario
    | (first($baseline.scenarios[]
        | select(.plugin == $scenario.plugin and .scenario == $scenario.scenario)) // null) as $old
    | select($old != null)
    | select(same_measurement($scenario; $old))
    | select($scenario.status == "ok" and $old.status == "ok")
    | {current: $scenario, baseline: $old}
  ] as $matches
| [$matches[].current] as $matched_current
| [$matches[].baseline] as $matched_baseline
| (($matches | length) > 0) as $comparable
| ([
     $current.scenarios[] as $scenario
     | (first($baseline.scenarios[]
        | select(.plugin == $scenario.plugin and .scenario == $scenario.scenario)) // null) as $old
     | if $old == null then
           {side: "current", plugin: $scenario.plugin, scenario: $scenario.scenario,
            reason: "missing from baseline"}
       elif (rubric($scenario) != rubric($old)) then
           {side: "current", plugin: $scenario.plugin, scenario: $scenario.scenario,
            reason: "rubric changed"}
       elif ($scenario.status != "ok") then
           {side: "current", plugin: $scenario.plugin, scenario: $scenario.scenario,
            reason: "not comparable in current"}
       elif ($old.status != "ok") then
           {side: "current", plugin: $scenario.plugin, scenario: $scenario.scenario,
            reason: "not comparable in baseline"}
       else empty end
   ] + [
     $baseline.scenarios[] as $old
     | select(any($current.scenarios[];
         .plugin == $old.plugin and .scenario == $old.scenario) | not)
     | {side: "baseline", plugin: $old.plugin, scenario: $old.scenario,
        reason: "missing from current"}
   ]) as $excluded
| {
    suite_dir: $baseline.metadata.suite_dir,
    created_at: $baseline.metadata.created_at,
    git_sha: $baseline.metadata.git_sha,
    n: $baseline.metadata.n,
    verdict: $baseline.verdict.state,
    with: scenario_arm_mean($matched_baseline; "with"),
    without: scenario_arm_mean($matched_baseline; "without"),
    delta: scenario_delta_mean($matched_baseline),
    cost: {
        agent_per_run: matched_agent_cost($matched_baseline; "with"),
        judge_per_run: matched_judge_cost($matched_baseline; "with"),
        with_per_run: matched_arm_cost($matched_baseline; "with"),
        without_per_run: matched_arm_cost($matched_baseline; "without"),
        pair_per_run: matched_pair_cost($matched_baseline)
    },
    tokens: {
        with_input_per_run: matched_arm_token($matched_baseline; "with"; "input_tokens"),
        with_output_per_run: matched_arm_token($matched_baseline; "with"; "output_tokens"),
        with_total_per_run: matched_arm_token($matched_baseline; "with"; "total_tokens"),
        without_input_per_run: matched_arm_token($matched_baseline; "without"; "input_tokens"),
        without_output_per_run: matched_arm_token($matched_baseline; "without"; "output_tokens"),
        without_total_per_run: matched_arm_token($matched_baseline; "without"; "total_tokens"),
        pair_input_per_run: matched_pair_token($matched_baseline; "input_tokens"),
        pair_output_per_run: matched_pair_token($matched_baseline; "output_tokens"),
        pair_total_per_run: matched_pair_token($matched_baseline; "total_tokens")
    },
    timing: {
        wall_seconds: matched_wall($matched_baseline; $baseline),
        agent_seconds: timing_total($matched_baseline; "agent_seconds"),
        judge_seconds: timing_total($matched_baseline; "judge_seconds")
    }
  } as $baseline_block
| {
    suite_dir: $current.metadata.suite_dir,
    created_at: $current.metadata.created_at,
    git_sha: $current.metadata.git_sha,
    n: $current.metadata.n,
    verdict: $current.verdict.state,
    with: scenario_arm_mean($matched_current; "with"),
    without: scenario_arm_mean($matched_current; "without"),
    delta: scenario_delta_mean($matched_current),
    cost: {
        agent_per_run: matched_agent_cost($matched_current; "with"),
        judge_per_run: matched_judge_cost($matched_current; "with"),
        with_per_run: matched_arm_cost($matched_current; "with"),
        without_per_run: matched_arm_cost($matched_current; "without"),
        pair_per_run: matched_pair_cost($matched_current)
    },
    tokens: {
        with_input_per_run: matched_arm_token($matched_current; "with"; "input_tokens"),
        with_output_per_run: matched_arm_token($matched_current; "with"; "output_tokens"),
        with_total_per_run: matched_arm_token($matched_current; "with"; "total_tokens"),
        without_input_per_run: matched_arm_token($matched_current; "without"; "input_tokens"),
        without_output_per_run: matched_arm_token($matched_current; "without"; "output_tokens"),
        without_total_per_run: matched_arm_token($matched_current; "without"; "total_tokens"),
        pair_input_per_run: matched_pair_token($matched_current; "input_tokens"),
        pair_output_per_run: matched_pair_token($matched_current; "output_tokens"),
        pair_total_per_run: matched_pair_token($matched_current; "total_tokens")
    },
    timing: {
        wall_seconds: matched_wall($matched_current; $current),
        agent_seconds: timing_total($matched_current; "agent_seconds"),
        judge_seconds: timing_total($matched_current; "judge_seconds")
    }
  } as $current_block
| $current + {comparison: {
    status: (if $comparable then "comparable" else "not comparable" end),
    comparable: $comparable,
    comparable_scenarios: ($matches | length),
    current_scenarios: ($current.scenarios | length),
    baseline_scenarios: ($baseline.scenarios | length),
    excluded_scenarios: ($excluded | length),
    excluded: $excluded,
    baseline: $baseline_block,
    current: $current_block,
    delta: {
        with_score: diff($current_block.with; $baseline_block.with),
        without_score: diff($current_block.without; $baseline_block.without),
        effect: diff($current_block.delta; $baseline_block.delta),
        agent_cost_per_run: diff($current_block.cost.agent_per_run; $baseline_block.cost.agent_per_run),
        judge_cost_per_run: diff($current_block.cost.judge_per_run; $baseline_block.cost.judge_per_run),
        with_cost_per_run: diff($current_block.cost.with_per_run; $baseline_block.cost.with_per_run),
        without_cost_per_run: diff($current_block.cost.without_per_run; $baseline_block.cost.without_per_run),
        pair_cost_per_run: diff($current_block.cost.pair_per_run; $baseline_block.cost.pair_per_run),
        with_input_tokens_per_run: diff($current_block.tokens.with_input_per_run; $baseline_block.tokens.with_input_per_run),
        with_output_tokens_per_run: diff($current_block.tokens.with_output_per_run; $baseline_block.tokens.with_output_per_run),
        with_total_tokens_per_run: diff($current_block.tokens.with_total_per_run; $baseline_block.tokens.with_total_per_run),
        without_input_tokens_per_run: diff($current_block.tokens.without_input_per_run; $baseline_block.tokens.without_input_per_run),
        without_output_tokens_per_run: diff($current_block.tokens.without_output_per_run; $baseline_block.tokens.without_output_per_run),
        without_total_tokens_per_run: diff($current_block.tokens.without_total_per_run; $baseline_block.tokens.without_total_per_run),
        wall_seconds: diff($current_block.timing.wall_seconds; $baseline_block.timing.wall_seconds),
        agent_seconds: diff($current_block.timing.agent_seconds; $baseline_block.timing.agent_seconds),
        judge_seconds: diff($current_block.timing.judge_seconds; $baseline_block.timing.judge_seconds)
    },
    scenarios: [
        $matches[] as $match
        | $match.current as $scenario
        | $match.baseline as $old
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
