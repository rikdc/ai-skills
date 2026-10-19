# Makefile for AI Skills
# Provides a simple interface for common operations

# Configuration
SHELL := /bin/bash
.ONESHELL:
.SHELLFLAGS := -euo pipefail -c

# Directories
PROJECT_ROOT := $(shell pwd)
TESTS_DIR := $(PROJECT_ROOT)/tests
SCRIPTS_DIR := $(PROJECT_ROOT)/scripts
CLAUDE_DIR := $(PROJECT_ROOT)/.claude
PLUGIN_DIR := $(PROJECT_ROOT)/plugins/security-hooks
HOOKS_DIR := $(PLUGIN_DIR)/hooks

# Scripts
SCANNER_SCRIPT := $(HOOKS_DIR)/mcp-security-scanner.sh
PROTECT_BRANCH_SCRIPT := $(HOOKS_DIR)/protect-main-branch.sh
MANIFEST_FILE := $(PLUGIN_DIR)/.claude-plugin/plugin.json
TEST_SCRIPT := $(TESTS_DIR)/test-scanner.sh
TEST_PROTECT_BRANCH_SCRIPT := $(TESTS_DIR)/test-protect-main-branch.sh
TEST_MARKETPLACE_SCRIPT := $(TESTS_DIR)/test-marketplace.sh
TEST_CODEX_SKILLS_SCRIPT := $(TESTS_DIR)/test-codex-skills.sh
TEST_EVALS_SCRIPT := $(TESTS_DIR)/test-evals.sh
TEST_EVAL_SUITE_SCRIPT := $(TESTS_DIR)/test-eval-suite.sh
TEST_EVAL_REPORT_SCRIPT := $(TESTS_DIR)/test-eval-report.sh
TEST_EVAL_USAGE_SCRIPT := $(TESTS_DIR)/test-eval-usage.sh
EVAL_SUITE_SCRIPT := $(SCRIPTS_DIR)/eval-suite.sh
EVAL_REPORT_SCRIPT := $(SCRIPTS_DIR)/eval-report.sh
VALIDATE_SCRIPT := $(SCRIPTS_DIR)/validate-config.sh
SYNC_CODEX_SKILLS_SCRIPT := $(SCRIPTS_DIR)/sync-codex-skills.sh

# `make eval` selection (empty means all plugins/scenarios). Plain := rather
# than ?= deliberately ignores ambient environment variables of the same name;
# command-line assignments still override.
PLUGIN :=
SKILL :=
UNIT :=
RUNS := 1
JOBS := 1
AGENT :=
JUDGE :=
DRY :=
REPORT := all
USAGE_CMD :=
JUDGE_USAGE_CMD :=

# AGENT/JUDGE/USAGE_CMD may contain a shell substitution like $(cat). Export
# them so the recipe can read the value as data instead of re-parsing it as
# shell code.
export AGENT JUDGE USAGE_CMD JUDGE_USAGE_CMD

# Pinned tool versions
CLAUDELINT_VERSION := 0.7.1

# Colors for output
BLUE := \033[0;34m
GREEN := \033[0;32m
YELLOW := \033[1;33m
RED := \033[0;31m
NC := \033[0m # No Color

# Default target
.DEFAULT_GOAL := help

##@ Testing

.PHONY: test
test: ## Run complete test suite
	@echo -e "$(BLUE)🧪 Running security scanner tests...$(NC)"
	@$(TEST_SCRIPT)
	@echo
	@echo -e "$(BLUE)🧪 Running protected branch hook tests...$(NC)"
	@$(TEST_PROTECT_BRANCH_SCRIPT)
	@echo
	@echo -e "$(BLUE)🧪 Running marketplace consistency tests...$(NC)"
	@$(TEST_MARKETPLACE_SCRIPT)
	@echo
	@echo -e "$(BLUE)🧪 Running Codex skill symlink tests...$(NC)"
	@$(TEST_CODEX_SKILLS_SCRIPT)
	@echo
	@echo -e "$(BLUE)🧪 Running eval and skill quality gate...$(NC)"
	@$(TEST_EVALS_SCRIPT)
	@echo
	@echo -e "$(BLUE)🧪 Running eval harness tests...$(NC)"
	@$(TEST_EVAL_SUITE_SCRIPT)
	@echo
	@echo -e "$(BLUE)🧪 Running eval usage tests...$(NC)"
	@$(TEST_EVAL_USAGE_SCRIPT)
	@echo
	@echo -e "$(BLUE)🧪 Running eval report tests...$(NC)"
	@$(TEST_EVAL_REPORT_SCRIPT)

.PHONY: eval
eval: ## Run eval scenarios with an agent and render the comparison report
	@set -euo pipefail; \
	unit=""; \
	if [[ -n "$(SKILL)" && -n "$(UNIT)" ]]; then \
		echo "make eval: pass SKILL or UNIT, not both" >&2; \
		exit 2; \
	fi; \
	if [[ -n "$(SKILL)" ]]; then \
		unit="skills/$(SKILL)"; \
	elif [[ -n "$(UNIT)" ]]; then \
		unit="$(UNIT)"; \
	fi; \
	suite_args=(); \
	if [[ -n "$(PLUGIN)" ]]; then \
		suite_args+=(--plugin "$(PLUGIN)"); \
	fi; \
	if [[ -n "$$unit" ]]; then \
		suite_args+=(--unit "$$unit"); \
	fi; \
	suite_args+=(--runs "$(RUNS)" --jobs "$(JOBS)"); \
	if [[ -n "$$USAGE_CMD" ]]; then \
		suite_args+=(--usage-cmd "$$USAGE_CMD"); \
	fi; \
	if [[ -n "$$JUDGE_USAGE_CMD" ]]; then \
		suite_args+=(--judge-usage-cmd "$$JUDGE_USAGE_CMD"); \
	fi; \
	stamp="$$(date -u +%Y%m%dT%H%M%SZ)-$$(git -C "$(PROJECT_ROOT)" rev-parse --short HEAD 2>/dev/null || echo nogit)-$$$$"; \
	suite_dir="$(PROJECT_ROOT)/results/eval-suite/$$stamp"; \
	if [[ "$(DRY)" == "1" ]]; then \
		"$(EVAL_SUITE_SCRIPT)" --plan "$${suite_args[@]}"; \
		echo; \
		echo "Report destination: $$suite_dir/report"; \
		exit 0; \
	fi; \
	if [[ -z "$$AGENT" ]]; then \
		echo "make eval: AGENT is required for a real run" >&2; \
		echo "  make eval PLUGIN=git-workflow SKILL=pr RUNS=3 AGENT='claude -p'" >&2; \
		echo '  make eval PLUGIN=git-workflow UNIT=commands/review AGENT='"'"'opencode run --auto "$$$$(cat)"'"'"'' >&2; \
		echo "Make consumes the substitution syntax itself. Use the double-dollar form shown in the second example so the shell receives the agent command intact." >&2; \
		echo "Use DRY=1 to inspect the plan without running a model." >&2; \
		exit 2; \
	fi; \
	judge="$$JUDGE"; \
	if [[ -z "$$judge" ]]; then \
		judge="$$AGENT"; \
	fi; \
	suite_args+=(--agent-cmd "$$AGENT" --judge-cmd "$$judge"); \
	if [[ -z "$$USAGE_CMD" ]]; then \
		if [[ "$$AGENT" == *opencode* && "$$AGENT" == *run* && "$$AGENT" != *"--format json"* ]]; then \
			echo "make eval: warning: opencode is not using --format json, so cost will be unknown (set USAGE_CMD or add the flag)" >&2; \
		elif [[ "$$AGENT" == *claude* && ( "$$AGENT" =~ (^|[[:space:]])-p([[:space:]]|$$) || "$$AGENT" =~ (^|[[:space:]])--print([[:space:]]|$$) ) && "$$AGENT" != *"--output-format json"* ]]; then \
			echo "make eval: warning: claude is not using --output-format json, so cost will be unknown (set USAGE_CMD or add the flag)" >&2; \
		fi; \
	fi; \
	mkdir -p "$$suite_dir"; \
	"$(EVAL_SUITE_SCRIPT)" "$${suite_args[@]}" --out "$$suite_dir"; \
	"$(EVAL_REPORT_SCRIPT)" "$$suite_dir" --format "$(REPORT)" --out "$$suite_dir/report"; \
	if [[ -f "$$suite_dir/report/report.html" ]]; then \
		echo "HTML report: $$suite_dir/report/report.html"; \
	fi

.PHONY: validate
validate: ## Validate plugin manifest, hook scripts, and dependencies
	@$(VALIDATE_SCRIPT)

##@ Quality Assurance

.PHONY: lint
lint: ## Run ShellCheck on shell scripts and markdownlint on Markdown files
	@echo -e "$(BLUE)🔍 Running linting checks...$(NC)"
	@echo
	@echo "Shell scripts:"
	@if command -v shellcheck >/dev/null 2>&1; then \
		find . -name "*.sh" -type f -perm +111 | grep -v node_modules | xargs shellcheck --rcfile=.github/linters/.shellcheckrc && \
		echo -e "  $(GREEN)✅ ShellCheck passed$(NC)"; \
	else \
		echo -e "  $(YELLOW)⚠️  ShellCheck not installed - skipping shell script linting$(NC)"; \
	fi
	@echo
	@echo "Markdown files:"
	@if command -v markdownlint >/dev/null 2>&1; then \
		markdownlint --config .github/linters/.markdown-lint.yml **/*.md && \
		echo -e "  $(GREEN)✅ markdownlint passed$(NC)"; \
	else \
		echo -e "  $(YELLOW)⚠️  markdownlint not installed - skipping Markdown linting$(NC)"; \
	fi
	@echo
	@$(MAKE) --no-print-directory lint-claude

.PHONY: lint-claude
lint-claude: ## Run claudelint on Claude Code configuration
	@echo "Claude configuration:"
	@if command -v npx >/dev/null 2>&1; then \
		npx --yes claude-code-lint@$(CLAUDELINT_VERSION) check-all && \
		echo -e "  $(GREEN)✅ claudelint passed$(NC)"; \
	else \
		echo -e "  $(YELLOW)⚠️  npx not installed - skipping Claude configuration linting$(NC)"; \
	fi

##@ Installation and Setup

.PHONY: install
install: ## Install hooks to current project
	@echo -e "$(BLUE)📦 Installing hooks...$(NC)"
	@chmod +x "$(SCANNER_SCRIPT)"
	@chmod +x "$(PROTECT_BRANCH_SCRIPT)"
	@echo -e "$(GREEN)✅ Installation complete$(NC)"

##@ Maintenance

.PHONY: clean
clean: ## Remove test artifacts and logs
	@echo -e "$(BLUE)🧹 Cleaning up test artifacts...$(NC)"
	@find "$(CLAUDE_DIR)" -name "*.log" -type f -delete 2>/dev/null || true
	@find "$(PLUGIN_DIR)" -name "*.log" -type f -delete 2>/dev/null || true
	@find "$(TESTS_DIR)" -name "*.log" -type f -delete 2>/dev/null || true
	@echo -e "$(GREEN)✅ Cleanup complete$(NC)"

.PHONY: sync-codex-skills
sync-codex-skills: ## Sync .codex/skills symlinks with plugins/*/skills
	@$(SYNC_CODEX_SKILLS_SCRIPT)

##@ Development

.PHONY: check-tools
check-tools: ## Check for required and optional tools
	@echo -e "$(BLUE)🔧 Checking tool availability...$(NC)"
	@echo
	@echo "Required tools:"
	@for tool in jq grep awk mktemp; do \
		if command -v $$tool >/dev/null 2>&1; then \
			echo -e "  ✅ $$tool"; \
		else \
			echo -e "  ❌ $$tool (required)"; \
		fi; \
	done
	@echo
	@echo "Development tools:"
	@for tool in shellcheck markdownlint; do \
		if command -v $$tool >/dev/null 2>&1; then \
			echo -e "  ✅ $$tool"; \
		else \
			echo -e "  ⚪ $$tool (recommended for linting)"; \
		fi; \
	done
	@echo
	@echo "Optional security tools:"
	@for tool in gitleaks trufflehog git-secrets; do \
		if command -v $$tool >/dev/null 2>&1; then \
			echo -e "  ✅ $$tool"; \
		else \
			echo -e "  ⚪ $$tool (optional)"; \
		fi; \
	done

##@ Information

.PHONY: status
status: ## Show current status and configuration
	@echo -e "$(BLUE)📊 AI Skills Status$(NC)"
	@echo
	@echo "Configuration:"
	@echo "  Project root: $(PROJECT_ROOT)"
	@echo "  Plugin directory: $(PLUGIN_DIR)"
	@echo "  Scanner script: $(SCANNER_SCRIPT)"
	@echo
	@echo "Files:"
	@ls -la "$(SCANNER_SCRIPT)" 2>/dev/null || echo -e "  $(RED)❌ Scanner script not found$(NC)"
	@ls -la "$(PROTECT_BRANCH_SCRIPT)" 2>/dev/null || echo -e "  $(RED)❌ Protected branch script not found$(NC)"
	@ls -la "$(MANIFEST_FILE)" 2>/dev/null || echo -e "  $(RED)❌ Plugin manifest not found$(NC)"
	@$(MAKE) check-tools

.PHONY: help
help: ## Display this help
	@echo -e "$(BLUE)AI Skills - Make Targets$(NC)"
	@echo
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
	@echo
	@echo -e "$(BLUE)Examples:$(NC)"
	@echo "  make test              # Run all tests"
	@echo "  make validate          # Validate plugin configuration"
	@echo "  make lint              # Run code quality checks"
	@echo "  make install           # Make hook scripts executable"
	@echo "  make sync-codex-skills # Sync .codex/skills symlinks with plugins/*/skills"
	@echo "  make eval PLUGIN=git-workflow SKILL=pr RUNS=3 AGENT='claude -p --output-format json'"
	@echo '  make eval PLUGIN=git-workflow UNIT=commands/review AGENT='"'"'opencode run --auto --format json "$$$$(cat)"'"'"''
	@echo "  (cost needs structured output or USAGE_CMD; Make consumes \$$(...), so use the double-dollar form above)"
	@echo "  make eval PLUGIN=git-workflow RUNS=1 DRY=1"
	@echo "  make clean             # Clean up artifacts"
	@echo

# Ensure scripts directory exists for utility scripts
$(SCRIPTS_DIR):
	@mkdir -p "$(SCRIPTS_DIR)"
