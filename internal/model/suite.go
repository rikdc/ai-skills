package model

// SuiteManifest is the immutable manifest of a suite directory.
type SuiteManifest struct {
	Version   int               `json:"version"`
	CreatedAt string            `json:"created_at"`
	Tool      ToolInfo          `json:"tool"`
	Repo      RepoInfo          `json:"repo"`
	Config    SuiteConfig       `json:"config"`
	Scenarios []PlannedScenario `json:"scenarios"`
}

// ToolInfo records the harness that created the suite.
type ToolInfo struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	SchemaVersion int    `json:"schema_version"`
	GraderVersion string `json:"grader_version"`
}

// RepoInfo records the repository state at suite creation.
type RepoInfo struct {
	Root     string `json:"root"`
	GitSHA   string `json:"git_sha"`
	GitDirty bool   `json:"git_dirty"`
}

// SuiteConfig is the execution configuration a resumed suite must match.
type SuiteConfig struct {
	Backend              string      `json:"backend"`
	Model                string      `json:"model,omitempty"`
	BaseURL              string      `json:"base_url,omitempty"`
	Temperature          string      `json:"temperature,omitempty"`
	Seed                 *int64      `json:"seed,omitempty"`
	MaxSteps             int         `json:"max_steps"`
	MaxTotalTokens       int64       `json:"max_total_tokens"`
	MaxOutputTokens      int         `json:"max_output_tokens"`
	ContextWindow        int         `json:"context_window"`
	KeepRecentGroups     int         `json:"keep_recent_groups"`
	AgentCmd             string      `json:"agent_cmd,omitempty"`
	Arm                  string      `json:"arm"`
	Runs                 int         `json:"runs"`
	TimeoutSeconds       int         `json:"timeout_seconds"`
	SetupTimeoutSeconds  int         `json:"setup_timeout_seconds"`
	ChecksTimeoutSeconds int         `json:"checks_timeout_seconds"`
	JudgeTimeoutSeconds  int         `json:"judge_timeout_seconds"`
	SkillDir             string      `json:"skill_dir"`
	AuthCLI              bool        `json:"auth_cli"`
	Keychain             bool        `json:"keychain"`
	AuthPaths            []string    `json:"auth_paths"`
	Units                []string    `json:"units"`
	Judge                JudgeConfig `json:"judge"`
	UsageCmd             string      `json:"usage_cmd,omitempty"`
	JudgeUsageCmd        string      `json:"judge_usage_cmd,omitempty"`
	PricingSHA256        string      `json:"pricing_sha256,omitempty"`
	AttemptArtifacts     string      `json:"attempt_artifacts"`
	HarnessVersion       string      `json:"harness_version"`
}

// JudgeConfig is the configured judge backend.
type JudgeConfig struct {
	Backend        string `json:"backend"`
	Model          string `json:"model,omitempty"`
	BaseURL        string `json:"base_url,omitempty"`
	Cmd            string `json:"cmd,omitempty"`
	IdentitySHA256 string `json:"identity_sha256,omitempty"`
}

// PlannedScenario is one scenario selected into a suite plan.
type PlannedScenario struct {
	Plugin      string   `json:"plugin"`
	Scenario    string   `json:"scenario"`
	Fingerprint string   `json:"fingerprint"`
	Units       []string `json:"units"`
	Mode        string   `json:"mode"`
	Execution   string   `json:"execution"`
}
