package telemetry

import (
	"os"
	"strings"
	"testing"
)

func TestBuildGTResourceAttrs_Empty(t *testing.T) {
	t.Setenv("MS_ROLE", "")
	t.Setenv("MS_RIG", "")
	t.Setenv("BD_ACTOR", "")
	t.Setenv("MS_MINER", "")
	t.Setenv("MS_CREW", "")
	t.Setenv("MS_SESSION", "")
	t.Setenv("MS_RUN", "")
	t.Setenv("MS_WORK_RIG", "")
	t.Setenv("MS_WORK_BEAD", "")
	t.Setenv("MS_WORK_MOL", "")

	result := buildGTResourceAttrs()
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestBuildGTResourceAttrs_Session(t *testing.T) {
	t.Setenv("MS_ROLE", "supervisor")
	t.Setenv("MS_RIG", "")
	t.Setenv("BD_ACTOR", "supervisor")
	t.Setenv("MS_MINER", "")
	t.Setenv("MS_CREW", "")
	t.Setenv("MS_SESSION", "hq-supervisor")

	result := buildGTResourceAttrs()
	if !strings.Contains(result, "ms.session=hq-supervisor") {
		t.Errorf("expected ms.session in result, got %q", result)
	}
}

func TestBuildGTResourceAttrs_AllVars(t *testing.T) {
	t.Setenv("MS_ROLE", "mol/witness")
	t.Setenv("MS_RIG", "mol")
	t.Setenv("BD_ACTOR", "mol/witness")
	t.Setenv("MS_MINER", "furiosa")
	t.Setenv("MS_CREW", "")
	t.Setenv("MS_SESSION", "")

	result := buildGTResourceAttrs()
	for _, want := range []string{"ms.role=mol/witness", "ms.rig=mol", "ms.actor=mol/witness", "ms.agent=furiosa"} {
		if !strings.Contains(result, want) {
			t.Errorf("expected %q in result, got %q", want, result)
		}
	}
}

func TestBuildGTResourceAttrs_MinerTakesPriorityOverCrew(t *testing.T) {
	t.Setenv("MS_MINER", "furiosa")
	t.Setenv("MS_CREW", "overseer")
	t.Setenv("MS_ROLE", "")
	t.Setenv("MS_RIG", "")
	t.Setenv("BD_ACTOR", "")

	result := buildGTResourceAttrs()
	if !strings.Contains(result, "ms.agent=furiosa") {
		t.Errorf("expected ms.agent=furiosa (MS_MINER), got %q", result)
	}
	if strings.Contains(result, "ms.agent=overseer") {
		t.Errorf("MS_CREW should not override MS_MINER, got %q", result)
	}
}

func TestBuildGTResourceAttrs_CrewFallback(t *testing.T) {
	t.Setenv("MS_MINER", "")
	t.Setenv("MS_CREW", "overseer")
	t.Setenv("MS_ROLE", "")
	t.Setenv("MS_RIG", "")
	t.Setenv("BD_ACTOR", "")

	result := buildGTResourceAttrs()
	if !strings.Contains(result, "ms.agent=overseer") {
		t.Errorf("expected ms.agent=overseer from MS_CREW, got %q", result)
	}
}

func TestBuildGTResourceAttrs_WorkContext(t *testing.T) {
	t.Setenv("MS_ROLE", "")
	t.Setenv("MS_RIG", "")
	t.Setenv("BD_ACTOR", "")
	t.Setenv("MS_MINER", "")
	t.Setenv("MS_CREW", "")
	t.Setenv("MS_WORK_RIG", "mineshaft")
	t.Setenv("MS_WORK_BEAD", "sg-05iq")
	t.Setenv("MS_WORK_MOL", "mol-miner-work")

	result := buildGTResourceAttrs()
	for _, want := range []string{"ms.work_rig=mineshaft", "ms.work_bead=sg-05iq", "ms.work_mol=mol-miner-work"} {
		if !strings.Contains(result, want) {
			t.Errorf("expected %q in result, got %q", want, result)
		}
	}
}

func TestBuildGTResourceAttrs_WorkContextPartial(t *testing.T) {
	t.Setenv("MS_ROLE", "")
	t.Setenv("MS_RIG", "")
	t.Setenv("BD_ACTOR", "")
	t.Setenv("MS_MINER", "")
	t.Setenv("MS_CREW", "")
	t.Setenv("MS_WORK_RIG", "")
	t.Setenv("MS_WORK_BEAD", "sg-05iq")
	t.Setenv("MS_WORK_MOL", "")

	result := buildGTResourceAttrs()
	if !strings.Contains(result, "ms.work_bead=sg-05iq") {
		t.Errorf("expected ms.work_bead in result, got %q", result)
	}
	if strings.Contains(result, "ms.work_rig") {
		t.Errorf("ms.work_rig should not appear when empty, got %q", result)
	}
	if strings.Contains(result, "ms.work_mol") {
		t.Errorf("ms.work_mol should not appear when empty, got %q", result)
	}
}

func TestBuildGTResourceAttrs_Comma(t *testing.T) {
	t.Setenv("MS_ROLE", "a")
	t.Setenv("MS_RIG", "b")
	t.Setenv("BD_ACTOR", "")
	t.Setenv("MS_MINER", "")
	t.Setenv("MS_CREW", "")

	result := buildGTResourceAttrs()
	if !strings.Contains(result, ",") {
		t.Errorf("expected comma-separated result, got %q", result)
	}
}

func TestOTELEnvForSubprocess_Disabled(t *testing.T) {
	t.Setenv(EnvMetricsURL, "")
	env := OTELEnvForSubprocess()
	if env != nil {
		t.Errorf("expected nil when telemetry disabled, got %v", env)
	}
}

func TestOTELEnvForSubprocess_BothURLs(t *testing.T) {
	t.Setenv(EnvMetricsURL, "http://localhost:8428/opentelemetry/api/v1/push")
	t.Setenv(EnvLogsURL, "http://localhost:9428/insert/opentelemetry/v1/logs")
	t.Setenv("MS_ROLE", "")
	t.Setenv("MS_RIG", "")
	t.Setenv("BD_ACTOR", "")
	t.Setenv("MS_MINER", "")
	t.Setenv("MS_CREW", "")

	env := OTELEnvForSubprocess()
	if len(env) == 0 {
		t.Fatal("expected non-empty env")
	}

	hasMetrics, hasLogs := false, false
	for _, e := range env {
		if strings.HasPrefix(e, "BD_OTEL_METRICS_URL=") {
			hasMetrics = true
		}
		if strings.HasPrefix(e, "BD_OTEL_LOGS_URL=") {
			hasLogs = true
		}
	}
	if !hasMetrics {
		t.Error("expected BD_OTEL_METRICS_URL in subprocess env")
	}
	if !hasLogs {
		t.Error("expected BD_OTEL_LOGS_URL in subprocess env")
	}
}

func TestOTELEnvForSubprocess_NoLogsURL(t *testing.T) {
	t.Setenv(EnvMetricsURL, "http://localhost:8428/opentelemetry/api/v1/push")
	t.Setenv(EnvLogsURL, "")
	t.Setenv("MS_ROLE", "")
	t.Setenv("MS_RIG", "")
	t.Setenv("BD_ACTOR", "")
	t.Setenv("MS_MINER", "")
	t.Setenv("MS_CREW", "")

	env := OTELEnvForSubprocess()
	for _, e := range env {
		if strings.HasPrefix(e, "BD_OTEL_LOGS_URL=") {
			t.Errorf("BD_OTEL_LOGS_URL should not appear when MS_OTEL_LOGS_URL is empty, got %q", e)
		}
	}
}

func TestOTELEnvForSubprocess_WithResourceAttrs(t *testing.T) {
	t.Setenv(EnvMetricsURL, "http://localhost:8428/opentelemetry/api/v1/push")
	t.Setenv(EnvLogsURL, "")
	t.Setenv("MS_ROLE", "mol/witness")
	t.Setenv("MS_RIG", "mol")
	t.Setenv("BD_ACTOR", "")
	t.Setenv("MS_MINER", "")
	t.Setenv("MS_CREW", "")

	env := OTELEnvForSubprocess()
	hasAttrs := false
	for _, e := range env {
		if strings.HasPrefix(e, "OTEL_RESOURCE_ATTRIBUTES=") {
			hasAttrs = true
			if !strings.Contains(e, "ms.role=mol/witness") {
				t.Errorf("expected ms.role in OTEL_RESOURCE_ATTRIBUTES, got %q", e)
			}
		}
	}
	if !hasAttrs {
		t.Error("expected OTEL_RESOURCE_ATTRIBUTES in subprocess env when MS vars present")
	}
}

func TestSetProcessOTELAttrs_Disabled(t *testing.T) {
	t.Setenv(EnvMetricsURL, "")
	os.Unsetenv("BD_OTEL_METRICS_URL")
	os.Unsetenv("BD_OTEL_LOGS_URL")

	SetProcessOTELAttrs()

	if v := os.Getenv("BD_OTEL_METRICS_URL"); v != "" {
		t.Errorf("BD_OTEL_METRICS_URL should not be set when telemetry disabled, got %q", v)
	}
}

func TestSetProcessOTELAttrs_Enabled(t *testing.T) {
	metricsURL := "http://localhost:8428/opentelemetry/api/v1/push"
	logsURL := "http://localhost:9428/insert/opentelemetry/v1/logs"
	t.Setenv(EnvMetricsURL, metricsURL)
	t.Setenv(EnvLogsURL, logsURL)
	t.Setenv("MS_ROLE", "")
	t.Setenv("MS_RIG", "")
	t.Setenv("BD_ACTOR", "")
	t.Setenv("MS_MINER", "")
	t.Setenv("MS_CREW", "")

	SetProcessOTELAttrs()

	if got := os.Getenv("BD_OTEL_METRICS_URL"); got != metricsURL {
		t.Errorf("BD_OTEL_METRICS_URL = %q, want %q", got, metricsURL)
	}
	if got := os.Getenv("BD_OTEL_LOGS_URL"); got != logsURL {
		t.Errorf("BD_OTEL_LOGS_URL = %q, want %q", got, logsURL)
	}
}

func TestSetProcessOTELAttrs_SetsResourceAttrs(t *testing.T) {
	t.Setenv(EnvMetricsURL, "http://localhost:8428/opentelemetry/api/v1/push")
	t.Setenv(EnvLogsURL, "")
	t.Setenv("MS_ROLE", "mol/witness")
	t.Setenv("MS_RIG", "mol")
	t.Setenv("BD_ACTOR", "")
	t.Setenv("MS_MINER", "")
	t.Setenv("MS_CREW", "")
	os.Unsetenv("OTEL_RESOURCE_ATTRIBUTES")

	SetProcessOTELAttrs()

	got := os.Getenv("OTEL_RESOURCE_ATTRIBUTES")
	if got == "" {
		t.Error("expected OTEL_RESOURCE_ATTRIBUTES to be set")
	}
	if !strings.Contains(got, "ms.role=mol/witness") {
		t.Errorf("expected ms.role in OTEL_RESOURCE_ATTRIBUTES, got %q", got)
	}
}
