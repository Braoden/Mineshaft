package mail

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAddressToIdentity(t *testing.T) {
	tests := []struct {
		address  string
		expected string
	}{
		// Town-level agents keep trailing slash
		{"overseer", "overseer/"},
		{"overseer/", "overseer/"},
		{"supervisor", "supervisor/"},
		{"supervisor/", "supervisor/"},

		// Rig-scoped town-level roles resolve to canonical form (ms-te23)
		{"mineshaft/overseer", "overseer/"},
		{"mineshaft/supervisor", "supervisor/"},
		{"laser/overseer", "overseer/"},
		{"laser/supervisor", "supervisor/"},

		// Rig-level agents: crew/ and miners/ normalized to canonical form
		{"mineshaft/miners/Toast", "mineshaft/Toast"},
		{"mineshaft/crew/max", "mineshaft/max"},
		{"mineshaft/Toast", "mineshaft/Toast"}, // Already canonical
		{"mineshaft/max", "mineshaft/max"},     // Already canonical
		{"mineshaft/refinery", "mineshaft/refinery"},
		{"mineshaft/witness", "mineshaft/witness"},

		// Rig broadcast (trailing slash removed)
		{"mineshaft/", "mineshaft"},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			got := AddressToIdentity(tt.address)
			if got != tt.expected {
				t.Errorf("AddressToIdentity(%q) = %q, want %q", tt.address, got, tt.expected)
			}
		})
	}
}

func TestIdentityToAddress(t *testing.T) {
	tests := []struct {
		identity string
		expected string
	}{
		// Town-level agents
		{"overseer", "overseer/"},
		{"overseer/", "overseer/"},
		{"supervisor", "supervisor/"},
		{"supervisor/", "supervisor/"},

		// Rig-level agents: crew/ and miners/ normalized
		{"mineshaft/miners/Toast", "mineshaft/Toast"},
		{"mineshaft/crew/max", "mineshaft/max"},
		{"mineshaft/Toast", "mineshaft/Toast"}, // Already canonical
		{"mineshaft/refinery", "mineshaft/refinery"},
		{"mineshaft/witness", "mineshaft/witness"},

		// Rig name only (no transformation)
		{"mineshaft", "mineshaft"},
	}

	for _, tt := range tests {
		t.Run(tt.identity, func(t *testing.T) {
			got := identityToAddress(tt.identity)
			if got != tt.expected {
				t.Errorf("identityToAddress(%q) = %q, want %q", tt.identity, got, tt.expected)
			}
		})
	}
}

func TestPriorityToBeads(t *testing.T) {
	tests := []struct {
		priority Priority
		expected int
	}{
		{PriorityUrgent, 0},
		{PriorityHigh, 1},
		{PriorityNormal, 2},
		{PriorityLow, 3},
		{Priority("unknown"), 2}, // Default to normal
	}

	for _, tt := range tests {
		t.Run(string(tt.priority), func(t *testing.T) {
			got := PriorityToBeads(tt.priority)
			if got != tt.expected {
				t.Errorf("PriorityToBeads(%q) = %d, want %d", tt.priority, got, tt.expected)
			}
		})
	}
}

func TestPriorityFromInt(t *testing.T) {
	tests := []struct {
		p        int
		expected Priority
	}{
		{0, PriorityUrgent},
		{1, PriorityHigh},
		{2, PriorityNormal},
		{3, PriorityLow},
		{4, PriorityLow},     // Out of range maps to low
		{-1, PriorityNormal}, // Negative maps to normal
	}

	for _, tt := range tests {
		got := PriorityFromInt(tt.p)
		if got != tt.expected {
			t.Errorf("PriorityFromInt(%d) = %q, want %q", tt.p, got, tt.expected)
		}
	}
}

func TestParsePriority(t *testing.T) {
	tests := []struct {
		s        string
		expected Priority
	}{
		{"urgent", PriorityUrgent},
		{"high", PriorityHigh},
		{"normal", PriorityNormal},
		{"low", PriorityLow},
		{"unknown", PriorityNormal}, // Default
		{"", PriorityNormal},        // Empty
		{"URGENT", PriorityNormal},  // Case-sensitive, defaults to normal
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := ParsePriority(tt.s)
			if got != tt.expected {
				t.Errorf("ParsePriority(%q) = %q, want %q", tt.s, got, tt.expected)
			}
		})
	}
}

func TestParseMessageType(t *testing.T) {
	tests := []struct {
		s        string
		expected MessageType
	}{
		{"task", TypeTask},
		{"escalation", TypeEscalation},
		{"scavenge", TypeScavenge},
		{"notification", TypeNotification},
		{"reply", TypeReply},
		{"unknown", TypeNotification}, // Default
		{"", TypeNotification},        // Empty
		{"TASK", TypeNotification},    // Case-sensitive, defaults to notification
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := ParseMessageType(tt.s)
			if got != tt.expected {
				t.Errorf("ParseMessageType(%q) = %q, want %q", tt.s, got, tt.expected)
			}
		})
	}
}

func TestNewMessage(t *testing.T) {
	msg := NewMessage("overseer/", "mineshaft/Toast", "Test Subject", "Test Body")

	if msg.From != "overseer/" {
		t.Errorf("From = %q, want 'overseer/'", msg.From)
	}
	if msg.To != "mineshaft/Toast" {
		t.Errorf("To = %q, want 'mineshaft/Toast'", msg.To)
	}
	if msg.Subject != "Test Subject" {
		t.Errorf("Subject = %q, want 'Test Subject'", msg.Subject)
	}
	if msg.Body != "Test Body" {
		t.Errorf("Body = %q, want 'Test Body'", msg.Body)
	}
	if msg.ID == "" {
		t.Error("ID should be generated")
	}
	if msg.ThreadID == "" {
		t.Error("ThreadID should be generated")
	}
	if msg.Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}
	if msg.Priority != PriorityNormal {
		t.Errorf("Priority = %q, want PriorityNormal", msg.Priority)
	}
	if msg.Type != TypeNotification {
		t.Errorf("Type = %q, want TypeNotification", msg.Type)
	}
}

func TestNewReplyMessage(t *testing.T) {
	original := &Message{
		ID:       "orig-001",
		ThreadID: "thread-001",
		From:     "mineshaft/Toast",
		To:       "overseer/",
		Subject:  "Original Subject",
	}

	reply := NewReplyMessage("overseer/", "mineshaft/Toast", "Re: Original Subject", "Reply body", original)

	if reply.ThreadID != "thread-001" {
		t.Errorf("ThreadID = %q, want 'thread-001'", reply.ThreadID)
	}
	if reply.ReplyTo != "orig-001" {
		t.Errorf("ReplyTo = %q, want 'orig-001'", reply.ReplyTo)
	}
	if reply.From != "overseer/" {
		t.Errorf("From = %q, want 'overseer/'", reply.From)
	}
	if reply.To != "mineshaft/Toast" {
		t.Errorf("To = %q, want 'mineshaft/Toast'", reply.To)
	}
	if reply.Subject != "Re: Original Subject" {
		t.Errorf("Subject = %q, want 'Re: Original Subject'", reply.Subject)
	}
}

func TestBeadsMessageToMessage(t *testing.T) {
	now := time.Now()
	bm := BeadsMessage{
		ID:          "hq-test",
		Title:       "Test Subject",
		Description: "Test Body",
		Status:      "open",
		Assignee:    "mineshaft/Toast",
		Labels:      []string{"from:overseer/", "thread:t-001"},
		CreatedAt:   now,
		Priority:    1,
	}

	msg := bm.ToMessage()

	if msg.ID != "hq-test" {
		t.Errorf("ID = %q, want 'hq-test'", msg.ID)
	}
	if msg.Subject != "Test Subject" {
		t.Errorf("Subject = %q, want 'Test Subject'", msg.Subject)
	}
	if msg.Body != "Test Body" {
		t.Errorf("Body = %q, want 'Test Body'", msg.Body)
	}
	if msg.From != "overseer/" {
		t.Errorf("From = %q, want 'overseer/'", msg.From)
	}
	if msg.ThreadID != "t-001" {
		t.Errorf("ThreadID = %q, want 't-001'", msg.ThreadID)
	}
	if msg.To != "mineshaft/Toast" {
		t.Errorf("To = %q, want 'mineshaft/Toast'", msg.To)
	}
	if msg.Priority != PriorityHigh {
		t.Errorf("Priority = %q, want PriorityHigh", msg.Priority)
	}
}

func TestBeadsMessageToMessageWithReplyTo(t *testing.T) {
	bm := BeadsMessage{
		ID:          "hq-reply",
		Title:       "Reply Subject",
		Description: "Reply Body",
		Status:      "open",
		Assignee:    "mineshaft/Toast",
		Labels:      []string{"from:overseer/", "thread:t-002", "reply-to:orig-001", "msg-type:reply"},
		CreatedAt:   time.Now(),
		Priority:    2,
	}

	msg := bm.ToMessage()

	if msg.ReplyTo != "orig-001" {
		t.Errorf("ReplyTo = %q, want 'orig-001'", msg.ReplyTo)
	}
	if msg.Type != TypeReply {
		t.Errorf("Type = %q, want TypeReply", msg.Type)
	}
}

func TestBeadsMessageToMessageWithEscalationTypeAndLabels(t *testing.T) {
	bm := BeadsMessage{
		ID:          "hq-esc",
		Title:       "Escalation subject",
		Description: "Escalation body",
		Status:      "open",
		Assignee:    "overseer",
		Labels: []string{
			"from:supervisor/",
			"thread:t-esc",
			"msg-type:escalation",
			"ms:escalation",
			"severity:critical",
			"escalation:hq-abc123",
		},
		CreatedAt: time.Now(),
		Priority:  0,
	}

	msg := bm.ToMessage()

	if msg.Type != TypeEscalation {
		t.Errorf("Type = %q, want TypeEscalation", msg.Type)
	}
	if !bm.HasLabel("ms:escalation") {
		t.Error("expected ms:escalation label to be preserved")
	}
	if !bm.HasLabel("severity:critical") {
		t.Error("expected severity:critical label to be preserved")
	}
	if !bm.HasLabel("escalation:hq-abc123") {
		t.Error("expected escalation linkage label to be preserved")
	}
}

func TestBeadsMessageToMessagePriorities(t *testing.T) {
	tests := []struct {
		priority int
		expected Priority
	}{
		{0, PriorityUrgent},
		{1, PriorityHigh},
		{2, PriorityNormal},
		{3, PriorityLow},
		{4, PriorityNormal},  // Out of range defaults to normal
		{99, PriorityNormal}, // Out of range defaults to normal
	}

	for _, tt := range tests {
		bm := BeadsMessage{
			ID:       "hq-test",
			Priority: tt.priority,
		}
		msg := bm.ToMessage()
		if msg.Priority != tt.expected {
			t.Errorf("Priority %d -> %q, want %q", tt.priority, msg.Priority, tt.expected)
		}
	}
}

func TestBeadsMessageToMessageTypes(t *testing.T) {
	tests := []struct {
		msgType  string
		expected MessageType
	}{
		{"task", TypeTask},
		{"escalation", TypeEscalation},
		{"scavenge", TypeScavenge},
		{"reply", TypeReply},
		{"notification", TypeNotification},
		{"", TypeNotification}, // Default
	}

	for _, tt := range tests {
		bm := BeadsMessage{
			ID:     "hq-test",
			Labels: []string{"msg-type:" + tt.msgType},
		}
		msg := bm.ToMessage()
		if msg.Type != tt.expected {
			t.Errorf("msg-type:%s -> %q, want %q", tt.msgType, msg.Type, tt.expected)
		}
	}
}

func TestBeadsMessageToMessageEmptyLabels(t *testing.T) {
	bm := BeadsMessage{
		ID:          "hq-empty",
		Title:       "Empty Labels",
		Description: "Test with empty labels",
		Assignee:    "mineshaft/Toast",
		Labels:      []string{}, // No labels
		Priority:    2,
	}

	msg := bm.ToMessage()

	if msg.From != "" {
		t.Errorf("From should be empty, got %q", msg.From)
	}
	if msg.ThreadID != "" {
		t.Errorf("ThreadID should be empty, got %q", msg.ThreadID)
	}
}

func TestNewQueueMessage(t *testing.T) {
	msg := NewQueueMessage("overseer/", "work-requests", "New Task", "Please process this")

	if msg.From != "overseer/" {
		t.Errorf("From = %q, want 'overseer/'", msg.From)
	}
	if msg.Queue != "work-requests" {
		t.Errorf("Queue = %q, want 'work-requests'", msg.Queue)
	}
	if msg.To != "" {
		t.Errorf("To should be empty for queue messages, got %q", msg.To)
	}
	if msg.Channel != "" {
		t.Errorf("Channel should be empty for queue messages, got %q", msg.Channel)
	}
	if msg.Type != TypeTask {
		t.Errorf("Type = %q, want TypeTask", msg.Type)
	}
	if msg.ID == "" {
		t.Error("ID should be generated")
	}
	if msg.ThreadID == "" {
		t.Error("ThreadID should be generated")
	}
}

func TestNewChannelMessage(t *testing.T) {
	msg := NewChannelMessage("supervisor/", "alerts", "System Alert", "System is healthy")

	if msg.From != "supervisor/" {
		t.Errorf("From = %q, want 'supervisor/'", msg.From)
	}
	if msg.Channel != "alerts" {
		t.Errorf("Channel = %q, want 'alerts'", msg.Channel)
	}
	if msg.To != "" {
		t.Errorf("To should be empty for channel messages, got %q", msg.To)
	}
	if msg.Queue != "" {
		t.Errorf("Queue should be empty for channel messages, got %q", msg.Queue)
	}
	if msg.Type != TypeNotification {
		t.Errorf("Type = %q, want TypeNotification", msg.Type)
	}
}

func TestMessageIsQueueMessage(t *testing.T) {
	directMsg := NewMessage("overseer/", "mineshaft/Toast", "Test", "Body")
	queueMsg := NewQueueMessage("overseer/", "work-requests", "Task", "Body")
	channelMsg := NewChannelMessage("supervisor/", "alerts", "Alert", "Body")

	if directMsg.IsQueueMessage() {
		t.Error("Direct message should not be a queue message")
	}
	if !queueMsg.IsQueueMessage() {
		t.Error("Queue message should be a queue message")
	}
	if channelMsg.IsQueueMessage() {
		t.Error("Channel message should not be a queue message")
	}
}

func TestMessageIsChannelMessage(t *testing.T) {
	directMsg := NewMessage("overseer/", "mineshaft/Toast", "Test", "Body")
	queueMsg := NewQueueMessage("overseer/", "work-requests", "Task", "Body")
	channelMsg := NewChannelMessage("supervisor/", "alerts", "Alert", "Body")

	if directMsg.IsChannelMessage() {
		t.Error("Direct message should not be a channel message")
	}
	if queueMsg.IsChannelMessage() {
		t.Error("Queue message should not be a channel message")
	}
	if !channelMsg.IsChannelMessage() {
		t.Error("Channel message should be a channel message")
	}
}

func TestMessageIsDirectMessage(t *testing.T) {
	directMsg := NewMessage("overseer/", "mineshaft/Toast", "Test", "Body")
	queueMsg := NewQueueMessage("overseer/", "work-requests", "Task", "Body")
	channelMsg := NewChannelMessage("supervisor/", "alerts", "Alert", "Body")

	if !directMsg.IsDirectMessage() {
		t.Error("Direct message should be a direct message")
	}
	if queueMsg.IsDirectMessage() {
		t.Error("Queue message should not be a direct message")
	}
	if channelMsg.IsDirectMessage() {
		t.Error("Channel message should not be a direct message")
	}
}

func TestMessageValidate(t *testing.T) {
	tests := []struct {
		name    string
		msg     *Message
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid direct message",
			msg:     NewMessage("overseer/", "mineshaft/Toast", "Test", "Body"),
			wantErr: false,
		},
		{
			name:    "valid queue message",
			msg:     NewQueueMessage("overseer/", "work-requests", "Task", "Body"),
			wantErr: false,
		},
		{
			name:    "valid channel message",
			msg:     NewChannelMessage("supervisor/", "alerts", "Alert", "Body"),
			wantErr: false,
		},
		{
			name: "missing ID",
			msg: &Message{
				From:    "overseer/",
				To:      "mineshaft/Toast",
				Subject: "Test",
			},
			wantErr: true,
			errMsg:  "must have an ID",
		},
		{
			name: "missing From",
			msg: &Message{
				ID:      "msg-001",
				To:      "mineshaft/Toast",
				Subject: "Test",
			},
			wantErr: true,
			errMsg:  "must have a From address",
		},
		{
			name: "missing Subject",
			msg: &Message{
				ID:   "msg-001",
				From: "overseer/",
				To:   "mineshaft/Toast",
			},
			wantErr: true,
			errMsg:  "must have a Subject",
		},
		{
			name: "no routing target",
			msg: &Message{
				ID:      "msg-001",
				From:    "overseer/",
				Subject: "Test",
			},
			wantErr: true,
			errMsg:  "must have exactly one of",
		},
		{
			name: "both to and queue",
			msg: &Message{
				ID:      "msg-001",
				From:    "overseer/",
				To:      "mineshaft/Toast",
				Queue:   "work-requests",
				Subject: "Test",
			},
			wantErr: true,
			errMsg:  "mutually exclusive",
		},
		{
			name: "both to and channel",
			msg: &Message{
				ID:      "msg-001",
				From:    "overseer/",
				To:      "mineshaft/Toast",
				Channel: "alerts",
				Subject: "Test",
			},
			wantErr: true,
			errMsg:  "mutually exclusive",
		},
		{
			name: "both queue and channel",
			msg: &Message{
				ID:      "msg-001",
				From:    "overseer/",
				Queue:   "work-requests",
				Channel: "alerts",
				Subject: "Test",
			},
			wantErr: true,
			errMsg:  "mutually exclusive",
		},
		{
			name: "claimed_by on non-queue message",
			msg: &Message{
				ID:        "msg-001",
				From:      "overseer/",
				To:        "mineshaft/Toast",
				Subject:   "Test",
				ClaimedBy: "mineshaft/nux",
			},
			wantErr: true,
			errMsg:  "claimed_by is only valid for queue messages",
		},
		{
			name: "claimed_by on queue message is valid",
			msg: &Message{
				ID:        "msg-001",
				From:      "overseer/",
				Queue:     "work-requests",
				Subject:   "Test",
				ClaimedBy: "mineshaft/nux",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.Validate()
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if tt.errMsg != "" && !containsString(err.Error(), tt.errMsg) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestBeadsMessageParseQueueChannelLabels(t *testing.T) {
	claimedTime := time.Date(2026, 1, 14, 12, 0, 0, 0, time.UTC)
	claimedAtStr := claimedTime.Format(time.RFC3339)

	bm := BeadsMessage{
		ID:          "hq-queue",
		Title:       "Queue Message",
		Description: "Test queue message",
		Status:      "open",
		Labels: []string{
			"from:overseer/",
			"queue:work-requests",
			"claimed-by:mineshaft/nux",
			"claimed-at:" + claimedAtStr,
		},
		Priority: 2,
	}

	msg := bm.ToMessage()

	if msg.Queue != "work-requests" {
		t.Errorf("Queue = %q, want 'work-requests'", msg.Queue)
	}
	if msg.ClaimedBy != "mineshaft/nux" {
		t.Errorf("ClaimedBy = %q, want 'mineshaft/nux'", msg.ClaimedBy)
	}
	if msg.ClaimedAt == nil {
		t.Error("ClaimedAt should not be nil")
	} else if !msg.ClaimedAt.Equal(claimedTime) {
		t.Errorf("ClaimedAt = %v, want %v", msg.ClaimedAt, claimedTime)
	}
}

func TestBeadsMessageParseChannelLabel(t *testing.T) {
	bm := BeadsMessage{
		ID:          "hq-channel",
		Title:       "Channel Message",
		Description: "Test channel message",
		Status:      "open",
		Labels:      []string{"from:supervisor/", "channel:alerts"},
		Priority:    2,
	}

	msg := bm.ToMessage()

	if msg.Channel != "alerts" {
		t.Errorf("Channel = %q, want 'alerts'", msg.Channel)
	}
	if msg.Queue != "" {
		t.Errorf("Queue should be empty, got %q", msg.Queue)
	}
}

func TestBeadsMessageIsQueueMessage(t *testing.T) {
	queueMsg := BeadsMessage{
		ID:     "hq-queue",
		Labels: []string{"queue:work-requests"},
	}
	directMsg := BeadsMessage{
		ID:       "hq-direct",
		Assignee: "mineshaft/Toast",
	}
	channelMsg := BeadsMessage{
		ID:     "hq-channel",
		Labels: []string{"channel:alerts"},
	}

	if !queueMsg.IsQueueMessage() {
		t.Error("Queue message should be identified as queue message")
	}
	if directMsg.IsQueueMessage() {
		t.Error("Direct message should not be identified as queue message")
	}
	if channelMsg.IsQueueMessage() {
		t.Error("Channel message should not be identified as queue message")
	}
}

func TestBeadsMessageIsChannelMessage(t *testing.T) {
	queueMsg := BeadsMessage{
		ID:     "hq-queue",
		Labels: []string{"queue:work-requests"},
	}
	directMsg := BeadsMessage{
		ID:       "hq-direct",
		Assignee: "mineshaft/Toast",
	}
	channelMsg := BeadsMessage{
		ID:     "hq-channel",
		Labels: []string{"channel:alerts"},
	}

	if queueMsg.IsChannelMessage() {
		t.Error("Queue message should not be identified as channel message")
	}
	if directMsg.IsChannelMessage() {
		t.Error("Direct message should not be identified as channel message")
	}
	if !channelMsg.IsChannelMessage() {
		t.Error("Channel message should be identified as channel message")
	}
}

func TestBeadsMessageIsDirectMessage(t *testing.T) {
	queueMsg := BeadsMessage{
		ID:     "hq-queue",
		Labels: []string{"queue:work-requests"},
	}
	directMsg := BeadsMessage{
		ID:       "hq-direct",
		Assignee: "mineshaft/Toast",
	}
	channelMsg := BeadsMessage{
		ID:     "hq-channel",
		Labels: []string{"channel:alerts"},
	}

	if queueMsg.IsDirectMessage() {
		t.Error("Queue message should not be identified as direct message")
	}
	if !directMsg.IsDirectMessage() {
		t.Error("Direct message should be identified as direct message")
	}
	if channelMsg.IsDirectMessage() {
		t.Error("Channel message should not be identified as direct message")
	}
}

func TestMessageIsClaimed(t *testing.T) {
	unclaimed := NewQueueMessage("overseer/", "work-requests", "Task", "Body")
	if unclaimed.IsClaimed() {
		t.Error("Unclaimed message should not be claimed")
	}

	claimed := NewQueueMessage("overseer/", "work-requests", "Task", "Body")
	claimed.ClaimedBy = "mineshaft/nux"
	now := time.Now()
	claimed.ClaimedAt = &now

	if !claimed.IsClaimed() {
		t.Error("Claimed message should be claimed")
	}
}

func TestParseLabelsIdempotent(t *testing.T) {
	bm := BeadsMessage{
		ID:    "hq-test",
		Title: "Test",
		Labels: []string{
			"from:overseer/",
			"thread:t-001",
			"reply-to:orig-001",
			"msg-type:task",
			"cc:mineshaft/Toast",
			"cc:mineshaft/nux",
			"queue:work-requests",
			"channel:alerts",
			"claimed-by:mineshaft/nux",
			"delivery:pending",
			"delivery-acked-by:mineshaft/nux",
			"delivery-acked-at:2026-02-17T12:00:00Z",
			"delivery:acked",
		},
	}

	// Call ParseLabels multiple times
	bm.ParseLabels()
	bm.ParseLabels()
	bm.ParseLabels()

	// CC list should not accumulate duplicates
	if len(bm.cc) != 2 {
		t.Errorf("cc should have 2 entries after multiple ParseLabels calls, got %d: %v", len(bm.cc), bm.cc)
	}

	// Other fields should remain correct
	if bm.sender != "overseer/" {
		t.Errorf("sender = %q, want 'overseer/'", bm.sender)
	}
	if bm.threadID != "t-001" {
		t.Errorf("threadID = %q, want 't-001'", bm.threadID)
	}
	if bm.replyTo != "orig-001" {
		t.Errorf("replyTo = %q, want 'orig-001'", bm.replyTo)
	}
	if bm.msgType != "task" {
		t.Errorf("msgType = %q, want 'task'", bm.msgType)
	}
	if bm.queue != "work-requests" {
		t.Errorf("queue = %q, want 'work-requests'", bm.queue)
	}
	if bm.channel != "alerts" {
		t.Errorf("channel = %q, want 'alerts'", bm.channel)
	}
	if bm.claimedBy != "mineshaft/nux" {
		t.Errorf("claimedBy = %q, want 'mineshaft/nux'", bm.claimedBy)
	}
	if bm.deliveryState != DeliveryStateAcked {
		t.Errorf("deliveryState = %q, want %q", bm.deliveryState, DeliveryStateAcked)
	}
	if bm.deliveryAckedBy != "mineshaft/nux" {
		t.Errorf("deliveryAckedBy = %q, want %q", bm.deliveryAckedBy, "mineshaft/nux")
	}
}

func TestParseLabelsIdempotentViaPublicMethods(t *testing.T) {
	bm := BeadsMessage{
		ID:       "hq-test",
		Title:    "Test",
		Assignee: "mineshaft/Toast",
		Labels: []string{
			"from:overseer/",
			"cc:mineshaft/nux",
			"cc:mineshaft/slit",
		},
	}

	// Simulate the bug: calling IsDirectMessage then ToMessage
	// Both call ParseLabels internally
	_ = bm.IsDirectMessage()
	_ = bm.IsQueueMessage()
	_ = bm.IsChannelMessage()
	msg := bm.ToMessage()

	if len(msg.CC) != 2 {
		t.Errorf("CC should have 2 entries after multiple method calls, got %d: %v", len(msg.CC), msg.CC)
	}
}

func TestToMessage_DeliveryStatePendingOnPartialAck(t *testing.T) {
	bm := BeadsMessage{
		ID:       "hq-test",
		Title:    "Test",
		Assignee: "mineshaft/Toast",
		Labels: []string{
			"from:overseer/",
			"delivery:pending",
			"delivery-acked-by:mineshaft/Toast",
		},
	}

	msg := bm.ToMessage()
	if msg.DeliveryState != DeliveryStatePending {
		t.Fatalf("DeliveryState = %q, want %q", msg.DeliveryState, DeliveryStatePending)
	}
	if msg.DeliveryAckedBy != "" || msg.DeliveryAckedAt != nil {
		t.Fatalf("partial ack should not expose ack metadata, got by=%q at=%v", msg.DeliveryAckedBy, msg.DeliveryAckedAt)
	}
}

func TestSuppressNotifyNotSerialized(t *testing.T) {
	msg := NewMessage("overseer/", "mineshaft/Toast", "Test", "Body")
	msg.SuppressNotify = true

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// SuppressNotify should not appear in JSON output (json:"-" tag)
	if containsString(string(data), "SuppressNotify") || containsString(string(data), "suppress") {
		t.Errorf("SuppressNotify should not be serialized, but found in JSON: %s", data)
	}

	// Roundtrip: unmarshal should leave SuppressNotify as false (zero value)
	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded.SuppressNotify {
		t.Error("SuppressNotify should be false after roundtrip (not deserialized)")
	}
}

func TestNewMessageValidatesForCrossRigAddresses(t *testing.T) {
	// Regression test: cross-rig addresses like "beads/crew/emma" must have
	// auto-generated ID and pass validation (ms-rud3p).
	crossRigAddresses := []string{
		"beads/crew/emma",
		"mineshaft/miners/Toast",
		"otherrig/witness",
		"overseer/",
	}

	for _, addr := range crossRigAddresses {
		t.Run(addr, func(t *testing.T) {
			msg := NewMessage("mineshaft/dag", addr, "Test subject", "Test body")

			if msg.ID == "" {
				t.Error("NewMessage must generate a non-empty ID")
			}
			if msg.ThreadID == "" {
				t.Error("NewMessage must generate a non-empty ThreadID")
			}

			if err := msg.Validate(); err != nil {
				t.Errorf("NewMessage for %q should produce a valid message, got: %v", addr, err)
			}
		})
	}
}

func TestNewMessageFanOutCopiesGetUniqueIDs(t *testing.T) {
	// When fanning out to multiple recipients, copies with cleared IDs
	// should get unique IDs from sendToSingle (ms-rud3p).
	msg := NewMessage("mineshaft/dag", "beads/crew/emma", "Test", "Body")
	originalID := msg.ID

	if originalID == "" {
		t.Fatal("original message must have an ID")
	}

	// Simulate fan-out: create a copy and clear its ID
	msgCopy := *msg
	msgCopy.To = "otherrig/crew/bob"
	msgCopy.ID = ""

	if msgCopy.ID == originalID {
		t.Error("fan-out copy ID should be cleared, not match original")
	}

	// The cleared copy should fail validation (sendToSingle regenerates it)
	if err := msgCopy.Validate(); err == nil {
		t.Error("copy with empty ID should fail validation before sendToSingle regenerates it")
	}
}
