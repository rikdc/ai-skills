#!/usr/bin/env bash

# MCP Security Scanner Hook
# Scans MCP requests for sensitive data before sending to external services
# Compatible with Linux and macOS
#
# Exit Code 0 = All OK
# Exit Code 2 = Block the request.

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR

PROJECT_CLAUDE_DIR="$(dirname "$SCRIPT_DIR")"
readonly PROJECT_CLAUDE_DIR

LOG_FILE="${PROJECT_CLAUDE_DIR}/security-scan.log"
readonly LOG_FILE

CONFIG_FILE="${PROJECT_CLAUDE_DIR}/security-patterns.conf"
readonly CONFIG_FILE

# Logging function
log() {
    local timestamp
    timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    echo "[$timestamp] $*" >> "$LOG_FILE"
}

# Check if external security tools are available
check_security_tools() {
    local tools=()

    # Check for truffleHog
    if command -v trufflehog >/dev/null 2>&1; then
        tools+=("trufflehog")
    fi

    # Check for gitleaks
    if command -v gitleaks >/dev/null 2>&1; then
        tools+=("gitleaks")
    fi

    # Check for git-secrets
    if command -v git-secrets >/dev/null 2>&1; then
        tools+=("git-secrets")
    fi

    echo "${tools[@]:-}"
}

# Scan content using external tools
scan_with_tools() {
    local content="$1"
    local temp_file
    temp_file=$(mktemp)
    echo "$content" > "$temp_file"

    local tools
    IFS=' ' read -ra tools <<< "$(check_security_tools)"

    for tool in "${tools[@]}"; do
        case "$tool" in
            "trufflehog")
                if trufflehog filesystem --no-update --fail "$temp_file" 2>/dev/null; then
                    rm -f "$temp_file"
                    return 2  # Found secrets
                fi
                ;;
            "gitleaks")
                if ! gitleaks detect --source "$temp_file" --no-git 2>/dev/null; then
                    rm -f "$temp_file"
                    return 2  # Found secrets
                fi
                ;;
            "git-secrets")
                if ! git-secrets --scan "$temp_file" 2>/dev/null; then
                    rm -f "$temp_file"
                    return 2  # Found secrets
                fi
                ;;
            *)
                # Unknown tool - log but continue
                log "Unknown security tool: $tool"
                ;;
        esac
    done

    rm -f "$temp_file"
    return 0  # No secrets found
}

# Scan content using regex patterns
scan_with_patterns() {
    local content="$1"
    local violations=()

    # Log scanning operation
    log "Scanning content with patterns from $CONFIG_FILE"

    # Load patterns from config
    while IFS='=' read -r pattern_name pattern_regex; do
        # Skip comments and empty lines
        [[ "$pattern_name" =~ ^#.*$ ]] && continue
        [[ -z "$pattern_name" ]] && continue
        [[ -z "$pattern_regex" ]] && continue

        # -e is required: patterns such as the private key header begin with
        # dashes, which grep would otherwise parse as command-line options.
        if echo "$content" | grep -qi -e "$pattern_regex" 2>/dev/null; then
            log "Pattern match found: $pattern_name"
            violations+=("$pattern_name")
        fi
    done < "$CONFIG_FILE"

    if [[ ${#violations[@]} -gt 0 ]]; then
        log "Pattern violations found: ${violations[*]}"
        return 1
    fi

    return 0
}

# Main scanning function
scan_content() {
    local content="$1"
    local scan_method="$2"

    case "$scan_method" in
        "tools")
            scan_with_tools "$content"
            ;;
        "patterns")
            scan_with_patterns "$content"
            ;;
        "both")
            if ! scan_with_tools "$content"; then
                return 1
            fi
            scan_with_patterns "$content"
            ;;
        *)
            # Default to patterns only if unknown method
            log "Unknown scan method: $scan_method. Using patterns only."
            scan_with_patterns "$content"
            ;;
    esac
}

# Check if tool is MCP-related
is_mcp_tool() {
    local tool_name="$1"
    case "$tool_name" in
        mcp__*) return 0 ;;
        *) return 1 ;;
    esac
}

# Main hook logic
main() {
    local input
    input=$(cat)

    # Parse JSON input
    local hook_event tool_name tool_input
    hook_event=$(echo "$input" | jq -r '.hook_event_name // empty')
    tool_name=$(echo "$input" | jq -r '.tool_name // empty')
    tool_input=$(echo "$input" | jq -r '.tool_input // empty')

    # Only process PreToolUse events for MCP tools
    if [[ "$hook_event" != "PreToolUse" ]] || ! is_mcp_tool "$tool_name"; then
        exit 0
    fi

    if [[ -z "$tool_input" ]] || [[ "$tool_input" == "null" ]]; then
        log "No content to scan for tool: $tool_name"
        exit 0
    fi

    # Determine scan method based on available tools
    local available_tools scan_method
    IFS=' ' read -ra available_tools <<< "$(check_security_tools)"

    if [[ ${#available_tools[@]} -gt 0 ]]; then
        scan_method="both"  # Use both tools and patterns
        log "Using security tools: ${available_tools[*]} + patterns"
    else
        scan_method="patterns"  # Fall back to patterns only
        log "Using pattern-based scanning only"
    fi

    # Perform security scan
    if ! scan_content "$tool_input" "$scan_method"; then
        log "SECURITY VIOLATION: Sensitive data detected in MCP request to $tool_name"

        cat >&2 << EOF
{
  "decision": "block",
  "reason": "Sensitive data detected in MCP request to $tool_name. The request contains potentially sensitive information that should not be sent to external services. Please review the content and remove any: API keys, tokens, secrets, database connection strings, passwords, authentication credentials, private keys, certificates, or personal identifiable information (PII). Configure patterns in: $CONFIG_FILE. View scan logs in: $LOG_FILE"
}
EOF
        exit 2  # Block the request
    fi

    log "Security scan passed for tool: $tool_name"
    exit 0  # Allow the request to proceed
}

trap 'log "Script error on line $LINENO"' ERR

main "$@"
