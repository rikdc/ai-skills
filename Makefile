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
BIN_DIR := $(PROJECT_ROOT)/bin
EVALCTL_BIN := $(BIN_DIR)/evalctl
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
# Per-run agent deadline. evalctl takes a duration (15m, 90s) or a number of
# seconds.
TIMEOUT := 15m
AGENT :=
JUDGE :=
DRY :=
REPORT := all
# Suite directory and resumption. An empty OUT stamps a fresh
# results/eval-suite/<stamp> directory; OUT=<dir> reuses that directory (the
# report goes to <dir>/report). RESUME=1 requires OUT and re-runs only the runs
# without an accepted attempt, reusing everything else. RESUME_POLICY is the
# evalctl --resume-policy value.
OUT :=
RESUME :=
RESUME_POLICY := retry-failed
USAGE_CMD :=
JUDGE_USAGE_CMD :=
# Credential forwarding for real runs. evalctl copies the agent CLI login state
# into each workspace by default (--auth-cli), so `make eval` keeps that
# default; AUTH_CLI=0 runs without credentials. AUTH_PATH holds one or more
# paths relative to the real home. KEYCHAIN links the macOS Keychains directory.
AUTH_CLI := 1
KEYCHAIN :=
AUTH_PATH :=
# Extra arguments for `make evalctl-selftest` (for example SELFTEST_ARGS=--quick).
SELFTEST_ARGS ?=

# `RUNS := 1` above discards an ambient RUNS, and a makefile assignment hides
# it from $(origin RUNS) (which reports "file"). Read the process environment
# itself, and at the top level only, so the discard is not silent and recursive
# $(MAKE) calls do not repeat the warning. Command-line RUNS never reaches the
# environment at parse time, so it warns only for the `RUNS=3 make eval` form.
ifeq ($(MAKELEVEL),0)
AMBIENT_RUNS := $(shell printenv RUNS 2>/dev/null || true)
ifneq ($(AMBIENT_RUNS),)
$(warning make eval: ignoring ambient RUNS=$(AMBIENT_RUNS); pass RUNS on the command line instead: make eval RUNS=$(AMBIENT_RUNS) ...)
endif
endif

# Go toolchain used by the evalctl targets.
GO ?= go

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

##@ evalctl (Milestone A)

.PHONY: evalctl-build
evalctl-build: ## Build the evalctl binary into bin/evalctl
	@echo -e "$(BLUE)🔨 Building evalctl...$(NC)"
	@mkdir -p "$(BIN_DIR)"
	@cd "$(PROJECT_ROOT)" && $(GO) build -trimpath -o "$(EVALCTL_BIN)" ./cmd/evalctl
	@echo -e "  $(GREEN)✅ $(EVALCTL_BIN)$(NC)"

.PHONY: evalctl-test
evalctl-test: ## Run the Go unit and integration tests (no model or network)
	@echo -e "$(BLUE)🧪 Running evalctl Go tests...$(NC)"
	@cd "$(PROJECT_ROOT)" && $(GO) test ./...

.PHONY: evalctl-check
evalctl-check: ## Run gofmt, go vet, and the deterministic evalctl authoring gate
	@echo -e "$(BLUE)🔍 Checking evalctl Go sources...$(NC)"
	@cd "$(PROJECT_ROOT)" && $(GO) vet ./...
	@unformatted="$$(cd "$(PROJECT_ROOT)" && find cmd internal -name '*.go' -type f -print0 | xargs -0 gofmt -l)"; \
	if [[ -n "$$unformatted" ]]; then \
		echo "gofmt needed for:" >&2; \
		echo "$$unformatted" >&2; \
		exit 1; \
	fi
	@$(MAKE) --no-print-directory evalctl-build
	@"$(EVALCTL_BIN)" check
	@echo -e "  $(GREEN)✅ gofmt, go vet, and evalctl check passed$(NC)"

.PHONY: evalctl-selftest
evalctl-selftest: evalctl-build ## Run the deterministic end-to-end evalctl selftest
	@echo -e "$(BLUE)🧪 Running evalctl selftest...$(NC)"
	@"$(EVALCTL_BIN)" selftest $(SELFTEST_ARGS)

.PHONY: check
check: evalctl-check evalctl-selftest ## Run the Go checks and deterministic selftest
	@echo -e "$(GREEN)✅ check passed$(NC)"

##@ Testing

.PHONY: test
test: evalctl-test evalctl-check ## Run complete test suite
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
	@echo -e "$(BLUE)🧪 Running deterministic evalctl authoring gate...$(NC)"
	@"$(EVALCTL_BIN)" check

.PHONY: eval
eval: ## Run eval scenarios with an agent and render the comparison report
	@set -euo pipefail; \
	$(MAKE) --no-print-directory evalctl-build; \
	suite_runner=("$(EVALCTL_BIN)" suite); \
	report_runner=("$(EVALCTL_BIN)" report); \
	unit=""; \
	if [[ -n "$(SKILL)" && -n "$(UNIT)" ]]; then \
		echo "make eval: pass SKILL or UNIT, not both" >&2; \
		exit 2; \
	fi; \
	if [[ -n "$(RESUME)" && "$(RESUME)" != "0" && "$(RESUME)" != "1" ]]; then \
		echo "make eval: RESUME must be 1 or 0" >&2; \
		exit 2; \
	fi; \
	if [[ "$(RESUME)" == "1" ]]; then \
		if [[ -z "$(OUT)" ]]; then \
			echo "make eval: RESUME=1 needs OUT=<suite directory>; a resume reuses the existing suite" >&2; \
			echo "  make eval OUT=results/eval-suite/<stamp> RESUME=1 PLUGIN=git-workflow SKILL=pr AGENT='claude -p'" >&2; \
			exit 2; \
		fi; \
		case "$(RESUME_POLICY)" in \
			retry-failed|keep-failures|rerun-all) ;; \
			*) echo "make eval: RESUME_POLICY must be retry-failed, keep-failures or rerun-all" >&2; exit 2 ;; \
		esac; \
	fi; \
	if [[ -n "$(SKILL)" ]]; then \
		unit="skills/$(SKILL)"; \
	elif [[ -n "$(UNIT)" ]]; then \
		unit="$(UNIT)"; \
	fi; \
	auth_args=(); \
	if [[ -n "$(AUTH_CLI)" && "$(AUTH_CLI)" != "0" && "$(AUTH_CLI)" != "1" ]]; then \
		echo "make eval: AUTH_CLI must be 1 or 0" >&2; \
		exit 2; \
	fi; \
	if [[ -n "$(KEYCHAIN)" && "$(KEYCHAIN)" != "1" ]]; then \
		echo "make eval: KEYCHAIN must be 1 or empty" >&2; \
		exit 2; \
	fi; \
	if [[ "$(KEYCHAIN)" == "1" ]]; then \
		if [[ "$(AUTH_CLI)" == "0" ]]; then \
			echo "make eval: KEYCHAIN=1 requires AUTH_CLI=1" >&2; \
			exit 2; \
		fi; \
	fi; \
	if [[ "$(AUTH_CLI)" != "0" ]]; then \
		auth_args+=(--auth-cli); \
		if [[ "$(KEYCHAIN)" == "1" ]]; then \
			auth_args+=(--keychain); \
		fi; \
	fi; \
	for auth_path in $(AUTH_PATH); do \
		auth_args+=(--auth-path "$$auth_path"); \
	done; \
	suite_args=(); \
	if [[ -n "$(PLUGIN)" ]]; then \
		suite_args+=(--plugin "$(PLUGIN)"); \
	fi; \
	if [[ -n "$$unit" ]]; then \
		suite_args+=(--unit "$$unit"); \
	fi; \
	suite_args+=(--runs "$(RUNS)" --jobs "$(JOBS)"); \
	if [[ "$(RESUME)" == "1" ]]; then \
		suite_args+=(--resume --resume-policy "$(RESUME_POLICY)"); \
	fi; \
	if [[ -n "$(TIMEOUT)" ]]; then \
		suite_args+=(--timeout "$(TIMEOUT)"); \
	fi; \
	if [[ -n "$$USAGE_CMD" ]]; then \
		suite_args+=(--usage-cmd "$$USAGE_CMD"); \
	fi; \
	if [[ -n "$$JUDGE_USAGE_CMD" ]]; then \
		suite_args+=(--judge-usage-cmd "$$JUDGE_USAGE_CMD"); \
	fi; \
	if [[ "$$AGENT" == *'""'* ]]; then \
		echo 'make eval: AGENT contains an empty quoted argument; Make expanded $$(cat) to nothing before the shell saw it' >&2; \
		echo '  Use the double-dollar form so the shell expands the substitution:' >&2; \
		echo '  make eval PLUGIN=git-workflow SKILL=pr AGENT='"'"'opencode run --auto "$$$$(cat)"'"'"'' >&2; \
		exit 2; \
	fi; \
	if [[ -n "$(OUT)" ]]; then \
		if [[ "$(OUT)" == /* ]]; then \
			suite_dir="$(OUT)"; \
		else \
			suite_dir="$(PROJECT_ROOT)/$(OUT)"; \
		fi; \
	else \
		stamp="$$(date -u +%Y%m%dT%H%M%SZ)-$$(git -C "$(PROJECT_ROOT)" rev-parse --short HEAD 2>/dev/null || echo nogit)-$$$$"; \
		suite_dir="$(PROJECT_ROOT)/results/eval-suite/$$stamp"; \
	fi; \
	if [[ "$(DRY)" == "1" ]]; then \
		"$${suite_runner[@]}" --plan "$${suite_args[@]}"; \
		echo; \
		if [[ "$(RESUME)" == "1" ]]; then \
			echo "Resume: $$suite_dir (policy $(RESUME_POLICY))"; \
		fi; \
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
	if [[ "$$judge" == *'""'* ]]; then \
		echo 'make eval: JUDGE contains an empty quoted argument; Make expanded $$(cat) to nothing before the shell saw it' >&2; \
		echo '  Use the double-dollar form so the shell expands the substitution:' >&2; \
		echo '  make eval PLUGIN=git-workflow SKILL=pr JUDGE='"'"'claude -p "$$$$(cat)"'"'"'' >&2; \
		exit 2; \
	fi; \
	suite_args+=(--agent-cmd "$$AGENT" --judge-cmd "$$judge"); \
	if [[ -z "$$USAGE_CMD" ]]; then \
		if [[ "$$AGENT" == *opencode* && "$$AGENT" == *run* && "$$AGENT" != *"--format json"* ]]; then \
			echo "make eval: warning: opencode is not using --format json, so cost will be unknown (set USAGE_CMD or add the flag)" >&2; \
		elif [[ "$$AGENT" == *claude* && ( "$$AGENT" =~ (^|[[:space:]])-p([[:space:]]|$$) || "$$AGENT" =~ (^|[[:space:]])--print([[:space:]]|$$) ) && "$$AGENT" != *"--output-format json"* ]]; then \
			echo "make eval: warning: claude is not using --output-format json, so cost will be unknown (set USAGE_CMD or add the flag)" >&2; \
		fi; \
	fi; \
	if [[ "$(AUTH_CLI)" != "0" ]]; then \
		echo "make eval: copying CLI login state into workspaces (AUTH_CLI=0 to disable)" >&2; \
	else \
		echo "make eval: running without CLI credentials (AUTH_CLI=0)" >&2; \
	fi; \
	echo "make eval: runs per arm: $(RUNS), jobs: $(JOBS), per-run timeout: $(TIMEOUT)" >&2; \
	if [[ "$(RESUME)" == "1" ]]; then \
		echo "make eval: resuming $$suite_dir (policy $(RESUME_POLICY)); only runs without an accepted attempt re-run" >&2; \
	elif [[ -n "$(OUT)" ]]; then \
		echo "make eval: suite directory: $$suite_dir" >&2; \
	fi; \
	mkdir -p "$$suite_dir"; \
	suite_status=0; \
	"$${suite_runner[@]}" "$${suite_args[@]}" $${auth_args[@]+"$${auth_args[@]}"} --out "$$suite_dir" || suite_status=$$?; \
	if [[ -f "$$suite_dir/manifest.json" ]]; then \
		if [[ "$$suite_status" -ne 0 ]]; then \
			echo "make eval: suite exited $$suite_status; rendering the report for the suite as it stands" >&2; \
		fi; \
		report_status=0; \
		"$${report_runner[@]}" "$$suite_dir" --format "$(REPORT)" --out "$$suite_dir/report" || report_status=$$?; \
		if [[ -f "$$suite_dir/report/report.html" ]]; then \
			echo "HTML report: $$suite_dir/report/report.html"; \
		fi; \
		if [[ "$$suite_status" -eq 0 && "$$report_status" -ne 0 ]]; then \
			echo "make eval: report failed (exit $$report_status)" >&2; \
			exit "$$report_status"; \
		fi; \
	else \
		"$${report_runner[@]}" "$$suite_dir" --format "$(REPORT)" --out "$$suite_dir/report" >/dev/null 2>&1 || true; \
	fi; \
	if [[ "$$suite_status" -ne 0 ]]; then \
		exit "$$suite_status"; \
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
	@rm -f "$(EVALCTL_BIN)"
	@rmdir "$(BIN_DIR)" 2>/dev/null || true
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
	@for tool in go shellcheck markdownlint; do \
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
	@echo "  make test              # Run all shell and Go tests"
	@echo "  make validate          # Validate plugin configuration"
	@echo "  make lint              # Run code quality checks"
	@echo "  make evalctl-build     # Build bin/evalctl"
	@echo "  make evalctl-check     # gofmt, go vet, and evalctl check"
	@echo "  make evalctl-selftest  # Deterministic end-to-end evalctl selftest"
	@echo "  make install           # Make hook scripts executable"
	@echo "  make sync-codex-skills # Sync .codex/skills symlinks with plugins/*/skills"
	@echo "  make eval PLUGIN=git-workflow SKILL=pr RUNS=3 AGENT='claude -p --output-format json'"
	@echo "  make eval PLUGIN=git-workflow SKILL=pr RUNS=3 JOBS=3 TIMEOUT=20m AGENT='claude -p'"
	@echo "  (RUNS/JOBS/TIMEOUT must be on the make command line: TIMEOUT is a duration or seconds)"
	@echo '  make eval PLUGIN=git-workflow UNIT=commands/review AGENT='"'"'opencode run --auto --format json "$$$$(cat)"'"'"''
	@echo "  (cost needs structured output or USAGE_CMD; Make consumes \$$(...), so use the double-dollar form above)"
	@echo "  make eval PLUGIN=git-workflow RUNS=1 DRY=1"
	@echo "  make eval PLUGIN=git-workflow RUNS=1 AUTH_CLI=0 ...           # no credentials"
	@echo "  make eval OUT=results/eval-suite/<dir> PLUGIN=git-workflow SKILL=pr AGENT=...  # reuse a suite dir"
	@echo "  make eval OUT=results/eval-suite/<dir> RESUME=1 PLUGIN=git-workflow SKILL=pr AGENT=...  # resume: rerun only runs without an accepted attempt (RESUME_POLICY=retry-failed)"
	@echo "  (a suite with runner failures still renders its report and exits non-zero; a partial suite is resumed, not re-run)"
	@echo "  make clean             # Clean up artifacts"
	@echo

# Ensure scripts directory exists for utility scripts
$(SCRIPTS_DIR):
	@mkdir -p "$(SCRIPTS_DIR)"
