package events

import (
	"testing"
)

func TestSlingPayload(t *testing.T) {
	p := SlingPayload("gt-123", "mineshaft")
	if p["bead"] != "gt-123" {
		t.Errorf("bead = %v, want gt-123", p["bead"])
	}
	if p["target"] != "mineshaft" {
		t.Errorf("target = %v, want mineshaft", p["target"])
	}
}

func TestHookPayload(t *testing.T) {
	p := HookPayload("gt-456")
	if p["bead"] != "gt-456" {
		t.Errorf("bead = %v, want gt-456", p["bead"])
	}
	if len(p) != 1 {
		t.Errorf("expected 1 key, got %d", len(p))
	}
}

func TestUnhookPayload(t *testing.T) {
	p := UnhookPayload("gt-789")
	if p["bead"] != "gt-789" {
		t.Errorf("bead = %v, want gt-789", p["bead"])
	}
}

func TestHandoffPayload_WithSubject(t *testing.T) {
	p := HandoffPayload("working on auth", true)
	if p["to_session"] != true {
		t.Errorf("to_session = %v, want true", p["to_session"])
	}
	if p["subject"] != "working on auth" {
		t.Errorf("subject = %v, want 'working on auth'", p["subject"])
	}
}

func TestHandoffPayload_NoSubject(t *testing.T) {
	p := HandoffPayload("", false)
	if _, ok := p["subject"]; ok {
		t.Error("expected no subject key when empty")
	}
	if p["to_session"] != false {
		t.Errorf("to_session = %v, want false", p["to_session"])
	}
}

func TestDonePayload(t *testing.T) {
	p := DonePayload("gt-100", "miner/alpha")
	if p["bead"] != "gt-100" {
		t.Errorf("bead = %v, want gt-100", p["bead"])
	}
	if p["branch"] != "miner/alpha" {
		t.Errorf("branch = %v, want miner/alpha", p["branch"])
	}
}

func TestMailPayload(t *testing.T) {
	p := MailPayload("overseer/", "status update")
	if p["to"] != "overseer/" {
		t.Errorf("to = %v, want overseer/", p["to"])
	}
	if p["subject"] != "status update" {
		t.Errorf("subject = %v, want 'status update'", p["subject"])
	}
}

func TestSpawnPayload(t *testing.T) {
	p := SpawnPayload("mineshaft", "alpha")
	if p["rig"] != "mineshaft" {
		t.Errorf("rig = %v, want mineshaft", p["rig"])
	}
	if p["miner"] != "alpha" {
		t.Errorf("miner = %v, want alpha", p["miner"])
	}
}

func TestBootPayload(t *testing.T) {
	agents := []string{"witness", "refinery"}
	p := BootPayload("mineshaft", agents)
	if p["rig"] != "mineshaft" {
		t.Errorf("rig = %v, want mineshaft", p["rig"])
	}
	gotAgents, ok := p["agents"].([]string)
	if !ok {
		t.Fatal("agents is not []string")
	}
	if len(gotAgents) != 2 {
		t.Errorf("agents has %d items, want 2", len(gotAgents))
	}
}

func TestMergePayload_WithReason(t *testing.T) {
	p := MergePayload("mr-1", "alpha", "miner/alpha", "conflict")
	if p["mr"] != "mr-1" {
		t.Errorf("mr = %v, want mr-1", p["mr"])
	}
	if p["reason"] != "conflict" {
		t.Errorf("reason = %v, want conflict", p["reason"])
	}
}

func TestMergePayload_NoReason(t *testing.T) {
	p := MergePayload("mr-2", "beta", "miner/beta", "")
	if _, ok := p["reason"]; ok {
		t.Error("expected no reason key when empty")
	}
}

func TestPatrolPayload_WithMessage(t *testing.T) {
	p := PatrolPayload("mineshaft", 3, "all healthy")
	if p["rig"] != "mineshaft" {
		t.Errorf("rig = %v, want mineshaft", p["rig"])
	}
	if p["miner_count"] != 3 {
		t.Errorf("miner_count = %v, want 3", p["miner_count"])
	}
	if p["message"] != "all healthy" {
		t.Errorf("message = %v, want 'all healthy'", p["message"])
	}
}

func TestPatrolPayload_NoMessage(t *testing.T) {
	p := PatrolPayload("mineshaft", 0, "")
	if _, ok := p["message"]; ok {
		t.Error("expected no message key when empty")
	}
}

func TestMinerCheckPayload_WithIssue(t *testing.T) {
	p := MinerCheckPayload("mineshaft", "alpha", "working", "gt-123")
	if p["issue"] != "gt-123" {
		t.Errorf("issue = %v, want gt-123", p["issue"])
	}
}

func TestMinerCheckPayload_NoIssue(t *testing.T) {
	p := MinerCheckPayload("mineshaft", "alpha", "working", "")
	if _, ok := p["issue"]; ok {
		t.Error("expected no issue key when empty")
	}
}

func TestNudgePayload(t *testing.T) {
	p := NudgePayload("mineshaft", "alpha", "stuck")
	if p["rig"] != "mineshaft" {
		t.Errorf("rig = %v, want mineshaft", p["rig"])
	}
	if p["target"] != "alpha" {
		t.Errorf("target = %v, want alpha", p["target"])
	}
	if p["reason"] != "stuck" {
		t.Errorf("reason = %v, want stuck", p["reason"])
	}
}

func TestEscalationPayload(t *testing.T) {
	p := EscalationPayload("mineshaft", "alpha", "overseer", "unresponsive")
	if p["to"] != "overseer" {
		t.Errorf("to = %v, want overseer", p["to"])
	}
	if p["reason"] != "unresponsive" {
		t.Errorf("reason = %v, want unresponsive", p["reason"])
	}
}

func TestKillPayload(t *testing.T) {
	p := KillPayload("mineshaft", "alpha", "zombie")
	if p["rig"] != "mineshaft" {
		t.Errorf("rig = %v, want mineshaft", p["rig"])
	}
	if p["target"] != "alpha" {
		t.Errorf("target = %v, want alpha", p["target"])
	}
	if p["reason"] != "zombie" {
		t.Errorf("reason = %v, want zombie", p["reason"])
	}
}

func TestHaltPayload(t *testing.T) {
	services := []string{"witness", "refinery", "supervisor"}
	p := HaltPayload(services)
	gotServices, ok := p["services"].([]string)
	if !ok {
		t.Fatal("services is not []string")
	}
	if len(gotServices) != 3 {
		t.Errorf("services has %d items, want 3", len(gotServices))
	}
}

func TestSessionDeathPayload(t *testing.T) {
	p := SessionDeathPayload("gt-mineshaft-alpha", "mineshaft/miners/alpha", "zombie cleanup", "daemon")
	if p["session"] != "gt-mineshaft-alpha" {
		t.Errorf("session = %v, want gt-mineshaft-alpha", p["session"])
	}
	if p["agent"] != "mineshaft/miners/alpha" {
		t.Errorf("agent = %v", p["agent"])
	}
	if p["reason"] != "zombie cleanup" {
		t.Errorf("reason = %v", p["reason"])
	}
	if p["caller"] != "daemon" {
		t.Errorf("caller = %v", p["caller"])
	}
}

func TestMassDeathPayload_WithCause(t *testing.T) {
	sessions := []string{"s1", "s2"}
	p := MassDeathPayload(2, "5s", sessions, "rate limit")
	if p["count"] != 2 {
		t.Errorf("count = %v, want 2", p["count"])
	}
	if p["window"] != "5s" {
		t.Errorf("window = %v, want 5s", p["window"])
	}
	if p["possible_cause"] != "rate limit" {
		t.Errorf("possible_cause = %v, want 'rate limit'", p["possible_cause"])
	}
}

func TestMassDeathPayload_NoCause(t *testing.T) {
	p := MassDeathPayload(1, "3s", []string{"s1"}, "")
	if _, ok := p["possible_cause"]; ok {
		t.Error("expected no possible_cause key when empty")
	}
}

func TestSessionPayload_Full(t *testing.T) {
	p := SessionPayload("uuid-123", "mineshaft/crew/tester", "fixing bugs", "/some/dir")
	if p["session_id"] != "uuid-123" {
		t.Errorf("session_id = %v", p["session_id"])
	}
	if p["role"] != "mineshaft/crew/tester" {
		t.Errorf("role = %v", p["role"])
	}
	if p["topic"] != "fixing bugs" {
		t.Errorf("topic = %v", p["topic"])
	}
	if p["cwd"] != "/some/dir" {
		t.Errorf("cwd = %v", p["cwd"])
	}
}

func TestSessionPayload_Minimal(t *testing.T) {
	p := SessionPayload("uuid-456", "supervisor", "", "")
	if _, ok := p["topic"]; ok {
		t.Error("expected no topic key when empty")
	}
	if _, ok := p["cwd"]; ok {
		t.Error("expected no cwd key when empty")
	}
}
