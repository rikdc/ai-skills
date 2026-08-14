#!/usr/bin/env bash

# Sync Codex Skill Symlinks
#
# .codex/skills/<name> exposes each plugins/<plugin>/skills/<name> directory
# to Codex. This script keeps those symlinks in sync with the plugin skills
# actually on disk: creating missing links, fixing links that point at the
# wrong plugin, and removing links for skills that no longer exist.
#
# Usage:
#   scripts/sync-codex-skills.sh          # apply changes
#   scripts/sync-codex-skills.sh --check  # report drift, change nothing

# Deliberately no -e: this script's contract is to evaluate every skill and
# report every problem in one run, not abort on the first one.
set -uo pipefail
shopt -s nullglob

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly PROJECT_ROOT
cd "$PROJECT_ROOT" || exit 1

readonly PLUGINS_DIR="plugins"
readonly CODEX_SKILLS_DIR=".codex/skills"

CHECK_MODE=false
case "${1:-}" in
    --check)
        CHECK_MODE=true
        ;;
    -h | --help)
        echo "Usage: $0 [--check]"
        echo "  --check   Report drift without modifying $CODEX_SKILLS_DIR (exit 1 if any is found)"
        exit 0
        ;;
    "") ;;
    *)
        echo "Unknown option: $1" >&2
        exit 1
        ;;
esac

ACTIONS=0
ERRORS=0

# Records an action taken (or, in --check mode, an action that would be
# taken) and prints it, so the same code path drives both modes.
note() {
    if [[ "$CHECK_MODE" == true ]]; then
        echo "  would $1"
    else
        echo "  $1"
    fi
    ACTIONS=$((ACTIONS + 1))
}

# --- Discover the desired symlink set from plugins/*/skills/* --------------

declare -A desired_target
declare -A desired_plugin

for skills_dir in "$PLUGINS_DIR"/*/skills; do
    plugin="$(basename "$(dirname "$skills_dir")")"
    for skill_dir in "$skills_dir"/*/; do
        name="$(basename "$skill_dir")"
        if [[ -n "${desired_plugin[$name]:-}" && "${desired_plugin[$name]}" != "$plugin" ]]; then
            echo "ERROR: skill '$name' is defined by both '${desired_plugin[$name]}' and '$plugin'" >&2
            ERRORS=$((ERRORS + 1))
            continue
        fi
        desired_target[$name]="../../$PLUGINS_DIR/$plugin/skills/$name"
        desired_plugin[$name]="$plugin"
    done
done

if [[ $ERRORS -gt 0 ]]; then
    echo "Aborting: resolve the skill name collision(s) above before syncing." >&2
    exit 1
fi

if [[ ! -d "$CODEX_SKILLS_DIR" ]]; then
    if [[ "$CHECK_MODE" == true ]]; then
        note "create directory: $CODEX_SKILLS_DIR"
    else
        mkdir -p "$CODEX_SKILLS_DIR"
    fi
fi

# --- Create or fix a link for every desired skill ---------------------------

for name in "${!desired_target[@]}"; do
    link_path="$CODEX_SKILLS_DIR/$name"
    target="${desired_target[$name]}"

    if [[ -L "$link_path" ]]; then
        current_target="$(readlink "$link_path")"
        if [[ "$current_target" != "$target" ]]; then
            note "fix: $link_path -> $target (was $current_target)"
            if [[ "$CHECK_MODE" == false ]]; then
                rm -f "$link_path"
                ln -s "$target" "$link_path"
            fi
        fi
    elif [[ -e "$link_path" ]]; then
        echo "ERROR: $link_path exists and is not a symlink; refusing to overwrite" >&2
        ERRORS=$((ERRORS + 1))
    else
        note "create: $link_path -> $target"
        [[ "$CHECK_MODE" == true ]] || ln -s "$target" "$link_path"
    fi
done

# --- Remove links for skills that no longer exist ---------------------------

for link_path in "$CODEX_SKILLS_DIR"/*; do
    name="$(basename "$link_path")"
    [[ -n "${desired_target[$name]:-}" ]] && continue

    if [[ -L "$link_path" ]]; then
        note "remove: $link_path"
        [[ "$CHECK_MODE" == true ]] || rm -f "$link_path"
    else
        echo "WARNING: $link_path is not a symlink; leaving it in place" >&2
    fi
done

if [[ $ERRORS -gt 0 ]]; then
    exit 1
fi

if [[ "$CHECK_MODE" == true ]]; then
    if [[ $ACTIONS -gt 0 ]]; then
        echo
        echo "$ACTIONS change(s) needed. Run: scripts/sync-codex-skills.sh" >&2
        exit 1
    fi
    echo "✅ $CODEX_SKILLS_DIR is in sync with $PLUGINS_DIR/*/skills"
    exit 0
fi

if [[ $ACTIONS -gt 0 ]]; then
    echo "✅ $CODEX_SKILLS_DIR synced ($ACTIONS change(s))"
else
    echo "✅ $CODEX_SKILLS_DIR already in sync"
fi
