package model

// Schema and version constants shared by every on-disk contract.
const (
	// SchemaVersion is the v3 on-disk schema version.
	SchemaVersion = 3
	// ManifestVersion is the suite manifest version written by evalctl.
	ManifestVersion = SchemaVersion
	// Version is the evalctl release version.
	Version = "0.1.0-dev"
	// GraderVersion changes whenever validation rules or the grader contract
	// change.
	GraderVersion = "evalctl/" + Version + "+schema.3+validate.1"
)
