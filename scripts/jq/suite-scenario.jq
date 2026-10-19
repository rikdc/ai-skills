        def mean: if length == 0 then null else (add / length) end;
        def mean_present: map(select(. != null)) | mean;
        def usage_cost($usage):
            if (($usage.known // false) == true)
               and (($usage.cost_usd | type) == "number")
            then $usage.cost_usd else null end;
        def usage_tokens($usage):
            if (($usage.known // false) == true)
               and (($usage.total_tokens | type) == "number")
            then $usage.total_tokens else null end;
        def sum_present: map(select(. != null)) | if length == 0 then null else add end;
        def usage_stats($rows; $field):
            [$rows[] | (.[$field] // {known: false})] as $usages
            | [$usages[] | usage_cost(.)] as $costs
            | [$usages[] | usage_tokens(.)] as $tokens
            | ($costs | map(select(. != null)) | length) as $cost_runs
            | ($tokens | map(select(. != null)) | length) as $token_runs
            | {
                runs_known: ([$usages[] | select((.known // false) == true)] | length),
                runs_total: ($rows | length),
                cost: {
                    total: ([$costs[] | select(. != null)] | sum_present),
                    runs_known: $cost_runs,
                    mean: (if $cost_runs > 0 then (([$costs[] | select(. != null)] | add) / $cost_runs) else null end)
                },
                tokens: {
                    total: ([$tokens[] | select(. != null)] | sum_present),
                    runs_known: $token_runs,
                    mean: (if $token_runs > 0 then (([$tokens[] | select(. != null)] | add) / $token_runs) else null end)
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
                    mean: (if ($costs | length) > 0 then (($costs | add) / ($costs | length)) else null end)
                },
                tokens: {
                    total: ($tokens | sum_present),
                    runs_known: ($tokens | length),
                    mean: (if ($tokens | length) > 0 then (($tokens | add) / ($tokens | length)) else null end)
                }
            };
        def arm_stats($arm):
            [.[] | select(.arm == $arm)] as $rows
            | {
                score: ([$rows[].score] | mean),
                max: ([$rows[].max] | mean),
                percent: ([$rows[].percent] | mean_present),
                timing: {
                    runs_total: ($rows | length),
                    agent_seconds: ([$rows[].duration_seconds] | map(select(. != null)) | sum_present),
                    judge_seconds: ([$rows[].judge_duration_seconds] | map(select(. != null)) | sum_present),
                    timed_out_runs: ([$rows[] | select(.timed_out == true)] | length)
                },
                usage: {
                    agent: usage_stats($rows; "usage"),
                    judge: usage_stats($rows; "judge_usage"),
                    combined: combined_stats($rows)
                }
            };
        . as $runs
        | {
            plugin: $plugin,
            scenario: $scenario,
            status: (if all($runs[]; .status == "ok") then "ok" else "runner-failed" end),
            runs: $runs,
            arms: {
                with: ($runs | arm_stats("with")),
                without: ($runs | arm_stats("without"))
            },
            pending: ([$runs[].pending] | add // 0),
            judged: (([$runs[] | select(.judged != null)] | length) == ($runs | length)),
            delta: (
                [($runs | arm_stats("with").percent), ($runs | arm_stats("without").percent)]
                | if .[0] == null or .[1] == null then null else (.[0] - .[1]) end
            )
        }
