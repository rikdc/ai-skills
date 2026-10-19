    def mean: if length == 0 then null else (add / length) end;
    def mean_present: map(select(. != null)) | mean;
    def mean_of($values): if ($values | length) == 0 then null else (($values | add) / ($values | length)) end;
    def sum_present: map(select(. != null)) | if length == 0 then null else add end;
    def usage_cost($usage):
        if (($usage.known // false) == true) and (($usage.cost_usd | type) == "number")
        then $usage.cost_usd else null end;
    def usage_tokens($usage):
        if (($usage.known // false) == true) and (($usage.total_tokens | type) == "number")
        then $usage.total_tokens else null end;
    def field_stats($rows; $field):
        [$rows[] | (.[$field] // {known: false})] as $usages
        | [$usages[] | usage_cost(.)] as $costs
        | [$usages[] | usage_tokens(.)] as $tokens
        | ([$costs[] | select(. != null)]) as $known_costs
        | ([$tokens[] | select(. != null)]) as $known_tokens
        | {
            runs_known: ([$usages[] | select((.known // false) == true)] | length),
            runs_total: ($rows | length),
            cost: {
                total: ($known_costs | sum_present),
                runs_known: ($known_costs | length),
                runs_total: ($rows | length),
                mean: mean_of($known_costs)
            },
            tokens: {
                total: ($known_tokens | sum_present),
                runs_known: ($known_tokens | length),
                runs_total: ($rows | length),
                mean: mean_of($known_tokens)
            }
        };
    def combined_stats($rows):
        [$rows[] | {a: .usage, j: .judge_usage}] as $pairs
        | [$pairs[] | select(usage_cost(.a) != null and usage_cost(.j) != null)
            | (usage_cost(.a) + usage_cost(.j))] as $costs
        | [$pairs[] | select(usage_tokens(.a) != null and usage_tokens(.j) != null)
            | (usage_tokens(.a) + usage_tokens(.j))] as $tokens
        | {
            runs_known: ($costs | length),
            runs_total: ($rows | length),
            cost: {
                total: ($costs | sum_present),
                runs_known: ($costs | length),
                runs_total: ($rows | length),
                mean: mean_of($costs)
            },
            tokens: {
                total: ($tokens | sum_present),
                runs_known: ($tokens | length),
                runs_total: ($rows | length),
                mean: mean_of($tokens)
            }
        };
    def cost_stats($rows):
        {
            runs_total: ($rows | length),
            agent: field_stats($rows; "usage"),
            judge: field_stats($rows; "judge_usage"),
            combined: combined_stats($rows)
        };
    def diff($a; $b): if $a == null or $b == null then null else ($a - $b) end;
    def eps: 0.0001;
    . as $scenarios
    | [ $scenarios[] as $scenario
        | $scenario.criteria[]
        | . + {plugin: $scenario.plugin, scenario: $scenario.scenario} ] as $criteria
    | (([$scenarios[].criteria_count] | add // 0) * $configured_runs
       * (if $arm == "both" then 2 else 1 end)) as $expected_cells
    | ([ $scenarios[].runs[] | (.scored | length) ] | add // 0) as $scored_cells
    | ([ $scenarios[] | select(.status == "not-comparable") ] | length) as $not_comparable
    | ([ $scenarios[] | select(.status == "runner-failed") ] | length) as $failed_scenarios
    | ([ $scenarios[].runs[] | select(.status != "ok") ] | length) as $failed_runs
    | (($suite_json[0].counts.runner_failures // 0)) as $suite_failures
    | ([ $results[] | select((.status // "ok") != "ok") ] | length) as $results_failures
    | (if $suite_failures > $failed_runs then $suite_failures else $failed_runs end) as $runner_failures
    | (if $results_failures > $runner_failures then $results_failures else $runner_failures end) as $runner_failures
    | ([$criteria[] | select(.delta != null and .delta > eps)] | length) as $won
    | ([$criteria[] | select(.delta != null and .delta < -eps)] | length) as $lost
    | ([$criteria[] | select(.delta != null and (.delta | if . < 0 then -. else . end) <= eps)] | length) as $tied
    | ([$criteria[] | select(.delta != null and .delta < -eps)] | sort_by(.delta)) as $regressions
    | {
        with: ([$scenarios[].arms.with.percent_mean] | mean_present),
        without: ([$scenarios[].arms.without.percent_mean] | mean_present),
        delta: ([$scenarios[].delta | select(. != null)] | mean),
        n: $configured_runs,
        pending_points: ([$scenarios[].pending_points] | add // 0),
        pending_criteria: ([$scenarios[].pending_criteria] | add // 0),
        runner_failures: $runner_failures,
        failed_scenarios: $failed_scenarios,
        failed_runs: $failed_runs,
        results_failures: $results_failures,
        not_comparable: $not_comparable,
        with_min: ([$scenarios[] | select(.arms.with.percent_min != null) | .arms.with.percent_min] | min),
        with_max: ([$scenarios[] | select(.arms.with.percent_max != null) | .arms.with.percent_max] | max),
        without_min: ([$scenarios[] | select(.arms.without.percent_min != null) | .arms.without.percent_min] | min),
        without_max: ([$scenarios[] | select(.arms.without.percent_max != null) | .arms.without.percent_max] | max),
        coverage_percent: (if $expected_cells > 0 then ($scored_cells * 100 / $expected_cells) else 0 end),
        scored_criteria_cells: $scored_cells,
        expected_criteria_cells: $expected_cells,
        won: $won,
        lost: $lost,
        tied: $tied,
        regressions: ($regressions | length)
    } as $totals
    | (if $totals.delta == null then null else ($totals.delta | if . < 0 then -. else . end) end) as $abs_delta
    | (($totals.with_min != null and $totals.with_max != null
        and $totals.without_min != null and $totals.without_max != null)
       and ($totals.with_min <= $totals.without_max)
       and ($totals.without_min <= $totals.with_max)) as $ranges_overlap
    | (if $totals.delta == null then
           {state: "incomplete",
            reason: "no scenario has a paired with/without result; both arms must run"}
       elif $totals.runner_failures > 0 or $totals.failed_runs > 0 or $totals.pending_points > 0 or $totals.not_comparable > 0 then
           {state: "incomplete",
            reason: "runner failures or unjudged criteria remain; a verdict would be made up"}
       elif $totals.n == 1 then
           {state: "indicative", reason: "one run per arm; variance cannot be estimated"}
       elif $abs_delta <= eps then
           {state: "no measurable effect",
            reason: "delta is approximately zero; likely a task leak or a non-discriminating scenario"}
       elif $ranges_overlap then
           {state: "inconclusive", reason: "delta falls inside the overlapping per-arm min/max ranges"}
       elif $totals.delta > 0 and $totals.regressions == 0 then
           {state: "helps", reason: "positive mean delta with no regressions"}
       elif $totals.delta < 0 then
           {state: "hurts", reason: "negative mean delta"}
       else
           {state: "inconclusive", reason: "positive mean but at least one criterion regressed"}
       end) as $verdict
    | ([$scenarios[].runs[]]) as $all_runs
    | ([$all_runs[] | select(.arm == "with")]) as $with_runs
    | ([$all_runs[] | select(.arm == "without")]) as $without_runs
    | (cost_stats($with_runs)) as $cost_with
    | (cost_stats($without_runs)) as $cost_without
    | (diff($cost_with.combined.cost.mean; $cost_without.combined.cost.mean)) as $cost_delta
    | (if ($cost_with.combined.cost.runs_known == $cost_with.runs_total)
          and ($cost_without.combined.cost.runs_known == $cost_without.runs_total)
          and $cost_with.runs_total > 0 and $cost_without.runs_total > 0
          and $totals.delta != null and $totals.delta > eps
          and $cost_delta != null and $cost_delta > 0
       then ($cost_delta / $totals.delta)
       else null end) as $cost_per_delta_point
    | ({
        currency: "USD",
        with: $cost_with,
        without: $cost_without,
        delta: {
            agent_cost: diff($cost_with.agent.cost.mean; $cost_without.agent.cost.mean),
            judge_cost: diff($cost_with.judge.cost.mean; $cost_without.judge.cost.mean),
            combined_cost: $cost_delta,
            agent_tokens: diff($cost_with.agent.tokens.mean; $cost_without.agent.tokens.mean),
            judge_tokens: diff($cost_with.judge.tokens.mean; $cost_without.judge.tokens.mean),
            combined_tokens: diff($cost_with.combined.tokens.mean; $cost_without.combined.tokens.mean)
        },
        cost_per_delta_point: $cost_per_delta_point
    }) as $cost
    | ({
        currency: "USD",
        with: $cost_with,
        without: $cost_without,
        delta: $cost.delta
    }) as $usage
    | {
        version: 1,
        metadata: {
            suite_dir: $suite,
            label: $label,
            units: ([$scenarios[].units[]] | unique),
            agent_cmd: $agent,
            judge_cmd: $judge,
            model: (if $model == "" then null else $model end),
            git_sha: $git_sha,
            git_dirty: $git_dirty,
            created_at: $created_at,
            arm: $arm,
            n: $configured_runs,
            agent_invocations: $invocations,
            counts: $suite_json[0].counts
        },
        totals: $totals,
        cost: $cost,
        usage: $usage,
        verdict: $verdict,
        criteria: $criteria,
        scenarios: $scenarios,
        links: {
            markdown: "report.md",
            json: "report.json",
            html: "report.html",
            evidence: [$scenarios[] | {plugin: .plugin, scenario: .scenario,
                path: ("../runs/" + .plugin + "/" + .scenario)}]
        }
    }
