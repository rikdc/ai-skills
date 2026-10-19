        def mean: if length == 0 then null else (add / length) end;
        def mean_present: map(select(. != null)) | mean;
        def scored_map($run; $names):
            ((($run.mechanical.items // []) + ($run.judged.items // []))
             | map(select(.name as $n | $names | index($n)))
             | reduce .[] as $item ({}; .[$item.name] = $item.score));
        def pending_points($scored; $rubric):
            ([$rubric[] | select(.name as $n | ($scored | has($n) | not)) | .max_score] | add // 0);
        def pending_count($scored; $rubric):
            [$rubric[] | select(.name as $n | ($scored | has($n) | not))] | length;
        def arm_stats($rows):
            ([$rows[] | select(.percent != null)] | map(.percent)) as $percents
            | {
                runs: ($rows | length),
                score_mean: ([$rows[].score] | mean_present),
                percent_mean: ($percents | mean),
                percent_min: (if ($percents | length) == 0 then null else ($percents | min) end),
                percent_max: (if ($percents | length) == 0 then null else ($percents | max) end),
                pending_points: ([$rows[].pending_points] | add // 0),
                failures: ([$rows[] | select(.status != "ok")] | length),
                timing: {
                    agent_seconds: ([$rows[].duration_seconds] | map(select(. != null)) | add // null),
                    judge_seconds: ([$rows[].judge_duration_seconds] | map(select(. != null)) | add // null),
                    timed_out_runs: ([$rows[] | select(.timed_out == true)] | length)
                }
            };
        ($crit[0].checklist) as $rubric
        | ([$rubric[].max_score] | add // 0) as $denominator
        | ($rubric | map(.name)) as $names
        | . as $input_runs
        | [$input_runs[]
           | . as $run
           | scored_map($run; $names) as $scored
           | . + {
               score: (if .status == "ok" then ([$scored[]] | add // 0) else null end),
               percent: (if .status != "ok" then null
                         elif $denominator > 0 then (([$scored[]] | add // 0) * 100 / $denominator)
                         else null end),
               pending_points: pending_points($scored; $rubric),
               pending_count: pending_count($scored; $rubric),
               scored: $scored
           }] as $runs
        | [$runs[] | select(.arm == "with")] as $with_runs
        | [$runs[] | select(.arm == "without")] as $without_runs
        | [ $with_runs[] as $with_run
            | ($without_runs | map(select(.run == $with_run.run)) | first) as $without_run
            | select($without_run != null)
            | {run: $with_run.run, with: $with_run, without: $without_run} ] as $pairs
        | [ $rubric[] as $item
            | $item.name as $name
            | [ $pairs[]
                | select((.with.scored | has($name)) and (.without.scored | has($name)))
                | {w: .with.scored[$name], o: .without.scored[$name]} ] as $values
            | {
                name: $name,
                max: $item.max_score,
                with: ([$values[].w] | mean),
                without: ([$values[].o] | mean),
                delta: (if ($values | length) > 0 then ([$values[] | .w - .o] | mean) else null end),
                paired_runs: ($values | length),
                status: (
                    if any($runs[]; (.scored | has($name) | not)) then "pending"
                    elif ($values | length) > 0 then "comparable"
                    else "not comparable"
                    end
                )
            } ] as $criteria
        | (arm_stats($with_runs)) as $with_stats
        | (arm_stats($without_runs)) as $without_stats
        | (if ($with_stats.runs > 0 and $without_stats.runs > 0)
           then ($with_stats.percent_mean - $without_stats.percent_mean)
           else null end) as $delta
        | (if ([$runs[] | select(.status != "ok")] | length) > 0 then "runner-failed"
           elif ($with_stats.runs == 0 or $without_stats.runs == 0) then "not-comparable"
           else "ok" end) as $status
        | {
            plugin: $plugin,
            scenario: $scenario,
            units: ($units | split(" ") | map(select(. != ""))),
            denominator: $denominator,
            criteria_count: ($rubric | length),
            configured_runs: $configured_runs,
            status: $status,
            runs: $runs,
            arms: {with: $with_stats, without: $without_stats},
            delta: $delta,
            pending_points: ([$runs[].pending_points] | add // 0),
            pending_criteria: ([$runs[].pending_count] | add // 0),
            criteria: $criteria,
            regressions: [$criteria[] | select(.delta != null and .delta < -0.0001)]
        }
