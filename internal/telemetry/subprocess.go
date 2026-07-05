package telemetry

import (
	"os"
	"strings"
)

// buildGTResourceAttrs builds the OTEL_RESOURCE_ATTRIBUTES value from MS context
// vars present in the current process environment.
// Returns "" when no MS vars are found.
func buildGTResourceAttrs() string {
	var attrs []string
	if v := os.Getenv("MS_ROLE"); v != "" {
		attrs = append(attrs, "ms.role="+v)
	}
	if v := os.Getenv("MS_RIG"); v != "" {
		attrs = append(attrs, "ms.rig="+v)
	}
	if v := os.Getenv("BD_ACTOR"); v != "" {
		attrs = append(attrs, "ms.actor="+v)
	}
	// Miner and crew carry their agent name in different vars.
	if v := os.Getenv("MS_MINER"); v != "" {
		attrs = append(attrs, "ms.agent="+v)
	} else if v := os.Getenv("MS_CREW"); v != "" {
		attrs = append(attrs, "ms.agent="+v)
	}
	if v := os.Getenv("MS_SESSION"); v != "" {
		attrs = append(attrs, "ms.session="+v)
	}
	if v := os.Getenv("MS_RUN"); v != "" {
		attrs = append(attrs, "ms.run_id="+v)
	}
	// Work context — set by ms prime via injectWorkContext; identifies the rig,
	// bead, and molecule the agent is currently processing.
	if v := os.Getenv("MS_WORK_RIG"); v != "" {
		attrs = append(attrs, "ms.work_rig="+v)
	}
	if v := os.Getenv("MS_WORK_BEAD"); v != "" {
		attrs = append(attrs, "ms.work_bead="+v)
	}
	if v := os.Getenv("MS_WORK_MOL"); v != "" {
		attrs = append(attrs, "ms.work_mol="+v)
	}
	return strings.Join(attrs, ",")
}

// SetProcessOTELAttrs sets OTEL-related variables in the current process
// environment so that all bd subprocesses spawned via exec.Command inherit
// them automatically — no per-call injection needed.
//
// Sets:
//   - OTEL_RESOURCE_ATTRIBUTES — MS context labels (ms.role, ms.rig, …)
//   - BD_OTEL_METRICS_URL      — bd's own metrics var (mirrors MS_OTEL_METRICS_URL)
//   - BD_OTEL_LOGS_URL         — bd's own logs var   (mirrors MS_OTEL_LOGS_URL)
//
// Called once at ms startup (Execute) when telemetry is active.
// No-op when MS_OTEL_METRICS_URL is not set.
func SetProcessOTELAttrs() {
	metricsURL := os.Getenv(EnvMetricsURL)
	if metricsURL == "" {
		return
	}
	if attrs := buildGTResourceAttrs(); attrs != "" {
		_ = os.Setenv("OTEL_RESOURCE_ATTRIBUTES", attrs)
	}
	// Mirror MS vars into bd's own var names so bd subprocesses
	// emit their metrics to the same VictoriaMetrics instance.
	_ = os.Setenv("BD_OTEL_METRICS_URL", metricsURL)
	if logsURL := os.Getenv(EnvLogsURL); logsURL != "" {
		_ = os.Setenv("BD_OTEL_LOGS_URL", logsURL)
	}
}

// OTELEnvForSubprocess returns OTEL environment variables to inject into bd
// subprocesses when cmd.Env is built explicitly (overriding os.Environ).
//
// Complements SetProcessOTELAttrs for callers that construct cmd.Env manually
// (beads.go run, mail/bd.go runBdCommand) so the vars aren't lost when the
// explicit env slice is built from scratch instead of os.Environ().
//
// Returns nil when MS telemetry is not active (MS_OTEL_METRICS_URL not set).
func OTELEnvForSubprocess() []string {
	metricsURL := os.Getenv(EnvMetricsURL)
	if metricsURL == "" {
		return nil
	}
	var env []string
	if attrs := buildGTResourceAttrs(); attrs != "" {
		env = append(env, "OTEL_RESOURCE_ATTRIBUTES="+attrs)
	}
	env = append(env, "BD_OTEL_METRICS_URL="+metricsURL)
	if logsURL := os.Getenv(EnvLogsURL); logsURL != "" {
		env = append(env, "BD_OTEL_LOGS_URL="+logsURL)
	}
	if runID := os.Getenv("MS_RUN"); runID != "" {
		env = append(env, "MS_RUN="+runID)
	}
	return env
}
