package agentbridge

import (
	"encoding/json"
	"time"
)

type MessageType string

const (
	TypeInstruction     MessageType = "instruction"
	TypeAcknowledgement MessageType = "acknowledgement"
	TypeProgress        MessageType = "progress"
	TypeResult          MessageType = "result"
	TypeError           MessageType = "error"
	TypeQuestion        MessageType = "question"
	TypeAnswer          MessageType = "answer"
	TypeShutdownNotice  MessageType = "shutdown_notice"
)

type MessageStatus string

const (
	StatusPending      MessageStatus = "pending"
	StatusAcknowledged MessageStatus = "acknowledged"
	StatusCompleted    MessageStatus = "completed"
	StatusFailed       MessageStatus = "failed"
	StatusExpired      MessageStatus = "expired"
)

type AgentMessage struct {
	ID             string          `json:"id"`
	Sender         string          `json:"sender"`
	Recipient      string          `json:"recipient"`
	TaskID         string          `json:"task_id"`
	Type           MessageType     `json:"type"`
	Body           string          `json:"body"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	ExpiresAt      time.Time       `json:"expires_at"`
	Status         MessageStatus   `json:"status"`
	AcknowledgedAt *time.Time      `json:"acknowledged_at,omitempty"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
	Result         json.RawMessage `json:"result,omitempty"`
}

type AgentRegistration struct {
	Name      string    `json:"name"`
	TokenHash string    `json:"token_hash"`
	CreatedAt time.Time `json:"created_at"`
}

// ChannelMessage is a message posted to a named channel (a shared board or
// group conversation). Unlike AgentMessage it has no recipient, no expiry and
// no acknowledgement state: a channel is append-only and every member reads the
// same ordered log. Seq is a relay-wide monotonic cursor so a client can ask for
// "everything after what I have already read".
type ChannelMessage struct {
	ID        string    `json:"id"`
	Seq       uint64    `json:"seq"`
	Channel   string    `json:"channel"`
	Sender    string    `json:"sender"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// ChannelInfo summarises a channel for discovery.
type ChannelInfo struct {
	Name     string   `json:"name"`
	LastSeq  uint64   `json:"last_seq"`
	Count    int      `json:"count"`
	Owner    string   `json:"owner,omitempty"`
	Members  []string `json:"members,omitempty"`
	ReadOnly bool     `json:"read_only,omitempty"`
}

// ChannelMeta is a channel's access policy.
//
//   - Members empty means every authenticated agent may read and post.
//   - Members non-empty means only those agents (and the owner) may.
//   - ReadOnly means only the owner may post; members may still read.
//
// The owner is whoever first created the channel, or the first agent to
// configure a channel that was created implicitly (the always-present board).
type ChannelMeta struct {
	Owner    string   `json:"owner,omitempty"`
	Members  []string `json:"members,omitempty"`
	ReadOnly bool     `json:"read_only,omitempty"`
}

// CanRead reports whether an agent may read the channel.
func (m ChannelMeta) CanRead(agent string) bool {
	if len(m.Members) == 0 {
		return true
	}
	if agent == m.Owner {
		return true
	}
	for _, member := range m.Members {
		if member == agent {
			return true
		}
	}
	return false
}

// CanPost reports whether an agent may post to the channel.
func (m ChannelMeta) CanPost(agent string) bool {
	if !m.CanRead(agent) {
		return false
	}
	if m.ReadOnly && agent != m.Owner {
		return false
	}
	return true
}
