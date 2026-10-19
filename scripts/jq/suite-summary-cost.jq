            def money: if . == null then "—"
                       else "$" + (((. * 10000 | round) / 10000) | tostring) end;
            def cov($u): ($u.runs_known | tostring) + "/" + ($u.runs_total | tostring);
            [ .plugin, .scenario,
              (.arms.with.usage.agent.cost.total | money),
              (.arms.without.usage.agent.cost.total | money),
              (.arms.with.usage.judge.cost.total | money),
              (.arms.without.usage.judge.cost.total | money),
              (.arms.with.usage.combined.cost.total | money),
              (.arms.without.usage.combined.cost.total | money),
              (cov(.arms.with.usage.combined) + " with, " +
               cov(.arms.without.usage.combined) + " without") ]
            | "| " + join(" | ") + " |"
