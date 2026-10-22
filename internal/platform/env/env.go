// Package env builds the sanitized process environments that setup scripts,
// checks, the agent, and the judge run with.
//
// Sanitization keeps a caller-supplied allowlist plus every EVAL_-prefixed
// name and drops credential-shaped names (Denied) so API keys are not
// propagated by accident. It is not isolation: an explicit pass-env opt-in
// always wins over the deny list.
package env

import "strings"

// DefaultPath is used when the base environment has no PATH.
const DefaultPath = "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

// Denied reports whether name looks like a credential that must not be
// propagated by accident.
func Denied(name string) bool {
	switch {
	case strings.HasPrefix(name, "OPENAI_"),
		strings.HasPrefix(name, "EVALCTL_"),
		strings.HasSuffix(name, "_KEY"),
		strings.HasSuffix(name, "_TOKEN"),
		strings.Contains(name, "_SECRET"),
		strings.Contains(name, "_PASSWORD"):
		return true
	default:
		return false
	}
}

// Filter returns the values from base whose names survive sanitization.
//
// A name is kept when it is listed in passEnv, or when it is in allowed or
// carries the EVAL_ prefix while not being Denied. passEnv is checked first,
// so a pass-env name is authoritative: it is an explicit request, and the deny
// list only exists to stop accidental propagation.
func Filter(base []string, allowed map[string]bool, passEnv []string) map[string]string {
	optIn := make(map[string]bool, len(passEnv))
	for _, name := range passEnv {
		optIn[name] = true
	}

	values := make(map[string]string, len(allowed)+len(optIn))
	for _, entry := range base {
		i := strings.IndexByte(entry, '=')
		if i <= 0 {
			continue
		}
		name, value := entry[:i], entry[i+1:]
		if optIn[name] || (!Denied(name) && (allowed[name] || strings.HasPrefix(name, "EVAL_"))) {
			values[name] = value
		}
	}
	return values
}

// List renders values as NAME=value entries in unspecified order.
func List(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for name, value := range values {
		out = append(out, name+"="+value)
	}
	return out
}
