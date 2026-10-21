package model

// Mode selects the arm structure of an eval scenario.
type Mode string

const (
	// ModeComparison runs a without arm and a with arm and reports a delta.
	ModeComparison Mode = "comparison"
	// ModeFunctional runs the with arm only and reports an absolute score.
	ModeFunctional Mode = "functional"
)

// OrDefault returns the scenario mode, defaulting to comparison.
func (m Mode) OrDefault() Mode {
	if m == "" {
		return ModeComparison
	}
	return m
}

// Valid reports whether m is a known mode.
func (m Mode) Valid() bool {
	return m == ModeComparison || m == ModeFunctional
}

// ExecutionBackend selects how a scenario is executed.
type ExecutionBackend string

const (
	// ExecutionAgent invokes a command agent (milestone A backend).
	ExecutionAgent ExecutionBackend = "agent"
	// ExecutionHook runs hook cases directly (milestone C).
	ExecutionHook ExecutionBackend = "hook"
)

// OrDefault returns the backend, defaulting to agent.
func (b ExecutionBackend) OrDefault() ExecutionBackend {
	if b == "" {
		return ExecutionAgent
	}
	return b
}

// Valid reports whether b is a known execution backend.
func (b ExecutionBackend) Valid() bool {
	return b == ExecutionAgent || b == ExecutionHook
}

// Execution describes the scenario's execution backend and optional cases
// file for direct hook execution.
type Execution struct {
	Backend ExecutionBackend `json:"backend,omitempty"`
	Cases   string           `json:"cases,omitempty"`
}

// Scenario is the scenario.json contract.
type Scenario struct {
	SchemaVersion int        `json:"schema_version,omitempty"`
	Description   string     `json:"description"`
	Include       []string   `json:"include"`
	Unit          string     `json:"unit,omitempty"`
	Units         []string   `json:"units,omitempty"`
	Mode          Mode       `json:"mode,omitempty"`
	Execution     *Execution `json:"execution,omitempty"`
}

// DeclaredUnits returns the unit list a scenario declares. As in the legacy
// harness, "units" wins when both fields are present.
func (s Scenario) DeclaredUnits() []string {
	if len(s.Units) > 0 {
		return append([]string(nil), s.Units...)
	}
	if s.Unit != "" {
		return []string{s.Unit}
	}
	return nil
}

// ModeOrDefault returns the effective scenario mode.
func (s Scenario) ModeOrDefault() Mode {
	return s.Mode.OrDefault()
}

// ExecutionBackend returns the effective execution backend.
func (s Scenario) ExecutionBackend() ExecutionBackend {
	if s.Execution == nil {
		return ExecutionAgent
	}
	return s.Execution.Backend.OrDefault()
}
