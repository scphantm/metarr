package handlers

import (
	"fmt"

	"Metarr/internal/shared/appconfig"
)

// GetLoggingConfig, UpsertLoggingConfig, SetAgentLogLevel and GetLogTail all
// migrated to gRPC-Web — see metarr.v1.LoggingService
// (internal/server/services/logging.go) and metarr.v1.AgentService.SetLogLevel
// (internal/server/services/agents.go), mounted via connectServices in
// cmd/metarr-server/main.go.

// ValidateLogLevel is exported so the gRPC-Web services package (SetAgentLogLevel,
// LoggingService.UpdateLoggingConfig) can reuse the same rule.
func ValidateLogLevel(level string) error {
	switch level {
	case appconfig.LogLevelInfo, appconfig.LogLevelDebug:
		return nil
	default:
		return fmt.Errorf("log_level must be %q or %q", appconfig.LogLevelInfo, appconfig.LogLevelDebug)
	}
}
