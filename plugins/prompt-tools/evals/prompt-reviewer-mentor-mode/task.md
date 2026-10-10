# Mentor-Mode Review of the Onboarding Prompt

## Background

A new engineer wrote their first AI prompt, `prompts/onboarding-helper.md`,
and obviously tried hard but it is rough. Your team's rule: when the author
is new, run the `prompt-reviewer` command with the `--mentor` flag, which
uses the Socratic style — guiding questions to help the author discover
improvements, never a list of prescriptive fixes. Dumping a rewrite on a
junior that they didn't think through themselves is against the rule.

## Your Task

1. Run `bash setup.sh` in the inputs directory to create the repository.
2. Read `prompts/onboarding-helper.md` completely.
3. Run the prompt-reviewer review **in mentor mode** and record it in
   `mentor-review.md` following the command's mentor output format:
   initial observations, questions grouped by the evaluation dimensions,
   thought experiments, areas for exploration, and next steps framed as
   things for the author to try.
4. Do not rewrite their prompt for them and do not edit
   `prompts/onboarding-helper.md`.
5. No network access; work purely from the file.

## Output

The working directory must contain the setup fixture plus `mentor-review.md`;
the prompt file itself must be unchanged.
