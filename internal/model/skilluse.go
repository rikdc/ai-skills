package model

// Skill-use states recorded for the with arm of a skill scenario.
const (
	// SkillInvoked means the agent log shows the skill being used.
	SkillInvoked = "invoked"
	// SkillNotInvoked means the log format was understood and shows no use.
	SkillNotInvoked = "not-invoked"
	// SkillUnknown means the log is missing or in a format that cannot prove
	// either answer. It is never reported as not-invoked.
	SkillUnknown = "unknown"
)

// SkillUse records whether the agent actually used the skill units installed
// for a with-arm run. State aggregates Skills: invoked when any installed
// skill was used, not-invoked when every skill was provably unused, unknown
// otherwise.
type SkillUse struct {
	State  string            `json:"state"`
	Skills map[string]string `json:"skills,omitempty"`
}

// SkillCoverage summarises SkillUse over the with-arm runs of one scenario.
type SkillCoverage struct {
	Invoked    int `json:"invoked"`
	NotInvoked int `json:"not_invoked"`
	Unknown    int `json:"unknown"`
	// Exercised is false only when every with-arm run provably left the skill
	// unused; unknown runs never make a scenario not-exercised.
	Exercised bool `json:"exercised"`
}
