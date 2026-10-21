package cli

import (
	"github.com/rikdc/ai-skills/internal/provider/command"
)

// executorCredentials holds the credential and environment options shared by
// every CLI-backed executor. The agent and judge executors are built from one
// value so a credentialed run cannot silently drift back to uncredentialed
// agents or judges.
type executorCredentials struct {
	auth    command.AuthOptions
	passEnv []string
}

// newExecutor builds a command executor with the shared credential options
// applied. The caller supplies only the executor-specific settings (command,
// timeout, usage hook, pricing, model).
func (c executorCredentials) newExecutor(cfg command.Config) (*command.Executor, error) {
	cfg.Auth = c.auth
	cfg.PassEnv = c.passEnv
	return command.New(cfg)
}
