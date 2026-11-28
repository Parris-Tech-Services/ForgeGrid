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
	Name    string `json:"name"`
	LastSeq uint64 `json:"last_seq"`
	Count   int    `json:"count"`
}
