#!/usr/bin/env bash

# Marketplace Consistency Tests
#
# Each plugin's own .claude-plugin/plugin.json is the source of truth.
# marketplace.json and the docs are derivative, so every check below reports
# drift against the plugin manifest rather than the other way round.

# Deliberately no -e: report every failure in one run, not just the first.
set -uo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT" || exit 1

readonly MARKETPLACE=".claude-plugin/marketplace.json"
readonly README="README.md"
readonly MARKETPLACE_DOC="docs/MARKETPLACE.md"

PASSED=0
FAILED=0

pass() {
    echo "  ✓ $1"
    PASSED=$((PASSED + 1))
}

fail() {
    echo "  ✗ $1"
    FAILED=$((FAILED + 1))
}

echo "🧪 Marketplace Consistency Tests"
echo "================================"

if ! command -v jq >/dev/null 2>&1; then
    echo "❌ jq is required but not installed"
    exit 1
fi

if ! jq -e . "$MARKETPLACE" >/dev/null 2>&1; then
    echo "❌ $MARKETPLACE is missing or not valid JSON"
    exit 1
fi

registered=$(jq -r '.plugins[].name' "$MARKETPLACE" | sort)

echo
echo "🚀 Registry covers the filesystem..."

on_disk=$(find plugins -mindepth 1 -maxdepth 1 -type d | sed 's|plugins/||' | sort)

unregistered=$(comm -23 <(echo "$on_disk") <(echo "$registered"))
if [[ -z "$unregistered" ]]; then
    pass "every plugin directory is registered"
else
    fail "unregistered plugin(s): $(echo "$unregistered" | tr '\n' ' ')"
fi

orphaned=$(comm -13 <(echo "$on_disk") <(echo "$registered"))
if [[ -z "$orphaned" ]]; then
    pass "every registered plugin exists on disk"
else
    fail "registered but missing from disk: $(echo "$orphaned" | tr '\n' ' ')"
fi

echo
echo "🚀 Registry entries match their plugin manifests..."

while IFS= read -r name; do
    [[ -z "$name" ]] && continue

    source_path=$(jq -r --arg n "$name" '.plugins[]|select(.name==$n)|.source' "$MARKETPLACE")
    manifest="$source_path/.claude-plugin/plugin.json"

    if [[ ! -f "$manifest" ]]; then
        fail "$name: source '$source_path' has no plugin.json"
        continue
    fi

    for field in version description; do
        registry_value=$(jq -r --arg n "$name" --arg f "$field" \
            '.plugins[]|select(.name==$n)|.[$f]' "$MARKETPLACE")
        manifest_value=$(jq -r --arg f "$field" '.[$f]' "$manifest")

        if [[ "$registry_value" == "$manifest_value" ]]; then
            pass "$name: $field matches manifest"
        else
            fail "$name: $field is '$registry_value' but manifest says '$manifest_value'"
        fi
    done

    # A manifest that points at a directory removed by a refactor still parses
    # cleanly, so check the declared component paths resolve to real content.
    for component in commands skills; do
        while IFS= read -r declared; do
            [[ -z "$declared" ]] && continue
            resolved="$source_path/${declared#./}"
            if [[ -d "$resolved" ]] && [[ -n "$(ls -A "$resolved")" ]]; then
                pass "$name: $component path '$declared' exists"
            else
                fail "$name: $component path '$declared' is missing or empty"
            fi
        done < <(jq -r --arg c "$component" '.[$c][]? // empty' "$manifest")
    done

    # hooks is an event-to-matcher object, not a path list: check that each
    # configured command resolves to an executable script in the plugin.
    while IFS= read -r hook_command; do
        [[ -z "$hook_command" ]] && continue
        script=$(echo "$hook_command" | tr -d '"' | sed "s|\${CLAUDE_PLUGIN_ROOT}|$source_path|")
        script="${script%% *}"
        if [[ -x "$script" ]]; then
            pass "$name: hook script '$(basename "$script")' is executable"
        else
            fail "$name: hook script '$script' is missing or not executable"
        fi
    done < <(jq -r '.hooks? // {} | .[]? | .[]? | .hooks[]? | .command' "$manifest")
done <<<"$registered"

echo
echo "🚀 Skills are well-formed..."

while IFS= read -r skill; do
    [[ -z "$skill" ]] && continue

    dir_name=$(basename "$(dirname "$skill")")
    frontmatter=$(awk '/^---$/{c++; next} c==1' "$skill")
    skill_name=$(echo "$frontmatter" | grep '^name:' | head -1 | sed 's/^name:[[:space:]]*//')
    skill_desc=$(echo "$frontmatter" | grep '^description:' | head -1 | sed 's/^description:[[:space:]]*//')

    if [[ "$skill_name" == "$dir_name" ]]; then
        pass "$dir_name: frontmatter name matches directory"
    else
        fail "$dir_name: frontmatter name is '$skill_name'"
    fi

    if [[ -n "$skill_desc" ]]; then
        pass "$dir_name: has a description"
    else
        fail "$dir_name: description is missing or empty"
    fi
done < <(find plugins -path '*/skills/*/SKILL.md' -type f | sort)

echo
echo "🚀 Docs list every registered plugin..."

while IFS= read -r name; do
    [[ -z "$name" ]] && continue

    if grep -q "plugins/$name" "$README"; then
        pass "$name: appears in $README"
    else
        fail "$name: missing from $README"
    fi

    if grep -qE "^### [0-9]+\. $name\$" "$MARKETPLACE_DOC"; then
        pass "$name: has a section in $MARKETPLACE_DOC"
    else
        fail "$name: missing from $MARKETPLACE_DOC"
    fi
done <<<"$registered"

echo
total=$((PASSED + FAILED))
echo "📊 Tests: $total, Passed: $PASSED, Failed: $FAILED"

if [[ "$FAILED" -eq 0 ]]; then
    echo "✅ All tests passed!"
    exit 0
else
    echo "❌ $FAILED test(s) failed!"
    exit 1
fi
