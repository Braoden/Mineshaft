// Package protocol provides inter-agent protocol message handling.
//
// This package defines protocol message types for Witness-Refinery communication
// and provides handlers for processing these messages.
//
// Protocol Message Types:
//   - MERGE_READY: Witness → Refinery (branch ready for merge)
//   - MERGED: Refinery → Witness (merge succeeded, cleanup ok)
//   - MERGE_FAILED: Refinery → Witness (merge failed, needs rework) [LEGACY]
//   - FIX_NEEDED: Refinery → Miner (merge failed, fix and resubmit)
//   - REWORK_REQUEST: Refinery → Witness (rebase needed)
package protocol

import (
	"strings"
	"time"
)

// MessageType identifies the protocol message type.
type MessageType string

const (
	// TypeMergeReady is sent from Witness to Refinery when a miner's work
	// is verified and ready for merge queue processing.
	// Subject format: "MERGE_READY <miner-name>"
	TypeMergeReady MessageType = "MERGE_READY"

	// TypeMerged is sent from Refinery to Witness when a branch has been
	// successfully merged to the target branch.
	// Subject format: "MERGED <miner-name>"
	TypeMerged MessageType = "MERGED"

	// TypeMergeFailed is sent from Refinery to Witness when a merge attempt
	// failed (tests, build, or other non-conflict error).
	// LEGACY: New code should use TypeFixNeeded (sent directly to miner).
	// Subject format: "MERGE_FAILED <miner-name>"
	TypeMergeFailed MessageType = "MERGE_FAILED"

	// TypeFixNeeded is sent from Refinery directly to the Miner when a
	// merge attempt failed (tests, build, lint, etc.). The miner reads
	// failure details, fixes the code, and resubmits the MR.
	// This replaces the old MERGE_FAILED → Witness → Miner flow.
	// Subject format: "FIX_NEEDED <miner-name>"
	TypeFixNeeded MessageType = "FIX_NEEDED"

	// TypeReworkRequest is sent from Refinery to Witness when a miner's
	// branch needs rebasing due to conflicts with the target branch.
	// Subject format: "REWORK_REQUEST <miner-name>"
	TypeReworkRequest MessageType = "REWORK_REQUEST"

	// TypeMinecartNeedsFeeding is sent from Refinery to Supervisor after a
	// minecart-eligible merge completes. This triggers immediate minecart
	// feeding instead of waiting for the next supervisor patrol cycle.
	// Subject format: "MINECART_NEEDS_FEEDING <minecart-id>"
	TypeMinecartNeedsFeeding MessageType = "MINECART_NEEDS_FEEDING"
)

// ParseMessageType extracts the protocol message type from a mail subject.
// Returns empty string if subject doesn't match a known protocol type.
func ParseMessageType(subject string) MessageType {
	subject = strings.TrimSpace(subject)

	// Check each known prefix
	prefixes := []MessageType{
		TypeMergeReady,
		TypeMerged,
		TypeMergeFailed,
		TypeFixNeeded,
		TypeReworkRequest,
		TypeMinecartNeedsFeeding,
	}

	for _, prefix := range prefixes {
		p := string(prefix)
		if subject == p || strings.HasPrefix(subject, p+" ") {
			return prefix
		}
	}

	return ""
}

// MergeReadyPayload contains the data for a MERGE_READY message.
// Sent by Witness after verifying miner work is complete.
type MergeReadyPayload struct {
	// Branch is the miner's work branch (e.g., "miner/Toast/ms-abc").
	Branch string `json:"branch"`

	// Issue is the beads issue ID the miner completed.
	Issue string `json:"issue"`

	// Miner is the worker name.
	Miner string `json:"miner"`

	// Rig is the rig name containing the miner.
	Rig string `json:"rig"`

	// Verified contains verification notes.
	Verified string `json:"verified,omitempty"`

	// Timestamp is when the message was created.
	Timestamp time.Time `json:"timestamp"`
}

// MergedPayload contains the data for a MERGED message.
// Sent by Refinery after successful merge to target branch.
type MergedPayload struct {
	// Branch is the source branch that was merged.
	Branch string `json:"branch"`

	// Issue is the beads issue ID.
	Issue string `json:"issue"`

	// Miner is the worker name.
	Miner string `json:"miner"`

	// Rig is the rig name.
	Rig string `json:"rig"`

	// MergedAt is when the merge completed.
	MergedAt time.Time `json:"merged_at"`

	// MergeCommit is the SHA of the merge commit.
	MergeCommit string `json:"merge_commit,omitempty"`

	// TargetBranch is the branch merged into (e.g., "main").
	TargetBranch string `json:"target_branch"`
}

// MergeFailedPayload contains the data for a MERGE_FAILED message.
// Sent by Refinery when merge fails due to tests, build, or other errors.
type MergeFailedPayload struct {
	// Branch is the source branch that failed to merge.
	Branch string `json:"branch"`

	// Issue is the beads issue ID.
	Issue string `json:"issue"`

	// Miner is the worker name.
	Miner string `json:"miner"`

	// Rig is the rig name.
	Rig string `json:"rig"`

	// FailedAt is when the failure occurred.
	FailedAt time.Time `json:"failed_at"`

	// FailureType categorizes the failure (tests, build, push, etc.).
	FailureType string `json:"failure_type"`

	// Error is the error message.
	Error string `json:"error"`

	// TargetBranch is the branch we tried to merge into.
	TargetBranch string `json:"target_branch"`
}

// FixNeededPayload contains the data for a FIX_NEEDED message.
// Sent by Refinery directly to the Miner when merge fails due to tests,
// build, lint, or other quality checks. The miner fixes and resubmits.
type FixNeededPayload struct {
	// Branch is the source branch that failed to merge.
	Branch string `json:"branch"`

	// Issue is the beads issue ID.
	Issue string `json:"issue"`

	// Miner is the worker name.
	Miner string `json:"miner"`

	// Rig is the rig name.
	Rig string `json:"rig"`

	// FailedAt is when the failure occurred.
	FailedAt time.Time `json:"failed_at"`

	// FailureType categorizes the failure (tests, build, lint, typecheck, etc.).
	FailureType string `json:"failure_type"`

	// Error is the error message/output from the failed check.
	Error string `json:"error"`

	// TargetBranch is the branch we tried to merge into.
	TargetBranch string `json:"target_branch"`

	// MRBeadID is the merge-request bead ID (preserved for resubmission).
	MRBeadID string `json:"mr_bead_id,omitempty"`

	// AttemptNumber tracks how many fix attempts have been made.
	AttemptNumber int `json:"attempt_number"`
}

// ReworkRequestPayload contains the data for a REWORK_REQUEST message.
// Sent by Refinery when a miner's branch has conflicts requiring rebase.
type ReworkRequestPayload struct {
	// Branch is the source branch that needs rebasing.
	Branch string `json:"branch"`

	// Issue is the beads issue ID.
	Issue string `json:"issue"`

	// Miner is the worker name.
	Miner string `json:"miner"`

	// Rig is the rig name.
	Rig string `json:"rig"`

	// RequestedAt is when the rework was requested.
	RequestedAt time.Time `json:"requested_at"`

	// TargetBranch is the branch to rebase onto.
	TargetBranch string `json:"target_branch"`

	// ConflictFiles lists files with conflicts (if known).
	ConflictFiles []string `json:"conflict_files,omitempty"`

	// Instructions provides specific rebase instructions.
	Instructions string `json:"instructions,omitempty"`
}

// MinerDonePayload contains the data from a MINER_DONE notification.
// This is not a formal protocol message (it's a mail convention), but the
// payload is structured for programmatic parsing by witness handlers.
type MinerDonePayload struct {
	// Miner is the worker name.
	Miner string `json:"miner"`

	// ExitType is the exit status (COMPLETED, ESCALATED, DEFERRED, PHASE_COMPLETE).
	ExitType string `json:"exit_type"`

	// Issue is the beads issue ID the miner worked on.
	Issue string `json:"issue,omitempty"`

	// Branch is the miner's work branch.
	Branch string `json:"branch"`

	// MR is the merge-request bead ID (empty for owned+direct minecarts).
	MR string `json:"mr,omitempty"`

	// MinecartID is the tracking minecart ID (if any).
	MinecartID string `json:"minecart_id,omitempty"`

	// MinecartOwned indicates the minecart has caller-managed lifecycle.
	MinecartOwned bool `json:"minecart_owned,omitempty"`

	// MergeStrategy is the minecart's merge strategy (direct, mr, local).
	MergeStrategy string `json:"merge_strategy,omitempty"`

	// Errors contains any non-fatal errors encountered during ms done.
	Errors string `json:"errors,omitempty"`
}

// SkipMergeFlow returns true if this miner's work should bypass the
// standard witness/refinery merge pipeline (owned minecart + direct merge).
func (p *MinerDonePayload) SkipMergeFlow() bool {
	return p.MinecartOwned && p.MergeStrategy == "direct"
}

// MinecartNeedsFeedingPayload contains the data for a MINECART_NEEDS_FEEDING message.
// Sent by Refinery to Supervisor after a minecart-eligible merge completes.
type MinecartNeedsFeedingPayload struct {
	// MinecartID is the minecart that may have newly-ready issues.
	MinecartID string `json:"minecart_id"`

	// SourceIssue is the issue that was just merged and closed.
	SourceIssue string `json:"source_issue"`

	// Rig is the rig where the merge happened.
	Rig string `json:"rig"`

	// MergedAt is when the merge completed.
	MergedAt time.Time `json:"merged_at"`
}

// IsProtocolMessage returns true if the subject matches a known protocol type.
func IsProtocolMessage(subject string) bool {
	return ParseMessageType(subject) != ""
}

// ExtractMiner extracts the miner name from a protocol message subject.
// Subject format: "TYPE <miner-name>"
func ExtractMiner(subject string) string {
	subject = strings.TrimSpace(subject)
	parts := strings.SplitN(subject, " ", 2)
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
