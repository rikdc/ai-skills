# Refuse an Invalid Release

## Background

The audit-checker repository maintains `CHANGELOG.md` in Keep a Changelog
format. The current release is `2.1.0`, and the changelog has an
`## [Unreleased]` section holding two entries.

The repository lives in the working directory. Run the fixture setup script
(`bash inputs/setup.sh`, or `bash setup.sh` if it sits at the working-directory
root) to create it.

## Your Task

The maintainer has asked for a release as version `2.1.0`. Handle that
request the way the team's changelog process calls for.

## Output Specification

- `CHANGELOG.md` — the outcome the team's process calls for
- `workflow.md` in the working directory recording what you did or did not
  change, and why
