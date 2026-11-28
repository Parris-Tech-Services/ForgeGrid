package agentbridge

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// maxChannelMessages bounds how many messages a single channel keeps on disk.
// Channels are append-only logs, not archives; the oldest entries are dropped
// once the cap is reached so an unauthenticated flood cannot fill the disk.
const maxChannelMessages = 1000

// DefaultChannel is the shared household board. It always exists.
const DefaultChannel = "household"

// channelState is the on-disk shape of channels.json.
type channelState struct {
	NextSeq  uint64                      `json:"next_seq"`
	Channels map[string][]ChannelMessage `json:"channels"`
}

type Store struct {
	mu       sync.RWMutex
	dataDir  string
	agents   map[string]AgentRegistration
	messages map[string]AgentMessage
	channels map[string][]ChannelMessage
	nextSeq  uint64

	// reloadMu serialises agents.json re-reads; agentsMod/agentsSize remember
	// the last version seen so an unchanged file is not parsed on every request.
	reloadMu   sync.Mutex
	agentsMod  time.Time
	agentsSize int64

	// saveMu serialises channels.json writes. PostChannelMessage appends under
	// mu but writes the file after releasing it, so two concurrent posts would
	// otherwise race on the same temporary file.
	saveMu sync.Mutex
}

func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	dataDir := filepath.Join(home, ".local", "share", "forgegrid", "agentbridge")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create agentbridge data dir: %w", err)
	}

	s := &Store{
		dataDir:  dataDir,
		agents:   make(map[string]AgentRegistration),
		messages: make(map[string]AgentMessage),
		channels: make(map[string][]ChannelMessage),
	}

	if err := s.load(); err != nil {
		return nil, err
	}

	if _, ok := s.channels[DefaultChannel]; !ok {
		s.channels[DefaultChannel] = []ChannelMessage{}
	}

	return s, nil
}

func (s *Store) load() error {
	agentsPath := filepath.Join(s.dataDir, "agents.json")
	if b, err := os.ReadFile(agentsPath); err == nil {
		if err := json.Unmarshal(b, &s.agents); err != nil {
			return fmt.Errorf("corrupt agents.json: %w", err)
		}
	}

	msgsPath := filepath.Join(s.dataDir, "messages.json")
	if b, err := os.ReadFile(msgsPath); err == nil {
		if err := json.Unmarshal(b, &s.messages); err != nil {
			return fmt.Errorf("corrupt messages.json: %w", err)
		}
	}

	channelsPath := filepath.Join(s.dataDir, "channels.json")
	if b, err := os.ReadFile(channelsPath); err == nil {
		var st channelState
		if err := json.Unmarshal(b, &st); err != nil {
			return fmt.Errorf("corrupt channels.json: %w", err)
		}
		if st.Channels != nil {
			s.channels = st.Channels
		}
		s.nextSeq = st.NextSeq
	}
	return nil
}

func (s *Store) save() error {
	// Save agents
	agB, err := json.MarshalIndent(s.agents, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(s.dataDir, "agents.json"), agB); err != nil {
		return err
	}

	// Save messages
	msgB, err := json.MarshalIndent(s.messages, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(s.dataDir, "messages.json"), msgB); err != nil {
		return err
	}
	return nil
}

func atomicWrite(path string, data []byte) error {
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func (s *Store) RegisterAgent(name, tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.agents[name] = AgentRegistration{
		Name:      name,
		TokenHash: tokenHash,
		CreatedAt: time.Now(),
	}
	return s.save()
}

func (s *Store) GetAgent(name string) (AgentRegistration, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.agents[name]
	return a, ok
}

func (s *Store) AddMessage(msg AgentMessage) (AgentMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if msg.IdempotencyKey != "" {
		for _, existing := range s.messages {
			if existing.Sender == msg.Sender && existing.IdempotencyKey == msg.IdempotencyKey {
				return existing, nil // Return existing message
			}
		}
	}

	if _, ok := s.messages[msg.ID]; ok {
		return msg, nil // Idempotent fallback
	}
	s.messages[msg.ID] = msg
	return msg, s.save()
}

func (s *Store) GetMessage(id string) (AgentMessage, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	msg, ok := s.messages[id]
	return msg, ok
}

func (s *Store) TransitionMessage(id, recipient, action string, result json.RawMessage) (AgentMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg, ok := s.messages[id]
	if !ok {
		return AgentMessage{}, fmt.Errorf("not found")
	}

	if msg.Recipient != recipient {
		return AgentMessage{}, fmt.Errorf("forbidden")
	}

	now := time.Now()

	switch action {
	case "acknowledge":
		if msg.Status == StatusPending {
			msg.Status = StatusAcknowledged
			msg.AcknowledgedAt = &now
		}
	case "complete":
		if msg.Status != StatusCompleted && msg.Status != StatusFailed {
			msg.Status = StatusCompleted
			msg.CompletedAt = &now
			msg.Result = result
		}
	case "fail":
		if msg.Status != StatusCompleted && msg.Status != StatusFailed {
			msg.Status = StatusFailed
			msg.CompletedAt = &now
			msg.Result = result
		}
	default:
		return AgentMessage{}, fmt.Errorf("invalid action")
	}

	s.messages[id] = msg
	if err := s.save(); err != nil {
		return AgentMessage{}, err
	}

	return msg, nil
}

func (s *Store) GetInbox(recipient string) []AgentMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var inbox []AgentMessage
	for _, m := range s.messages {
		if m.Recipient == recipient && time.Now().Before(m.ExpiresAt) {
			if m.Status == StatusPending || m.Status == StatusAcknowledged {
				inbox = append(inbox, m)
			}
		}
	}
	return inbox
}

func GenerateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ReloadAgentsIfChanged re-reads agents.json when a different version is on
// disk. This lets `register` take effect on a relay that is already running:
// previously a new agent was invisible until the relay restarted, and every
// call it made returned 401. A missing file is not an error (fresh install).
func (s *Store) ReloadAgentsIfChanged() {
	path := filepath.Join(s.dataDir, "agents.json")

	fi, err := os.Stat(path)
	if err != nil {
		return
	}

	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()

	if fi.ModTime().Equal(s.agentsMod) && fi.Size() == s.agentsSize {
		return
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var agents map[string]AgentRegistration
	if err := json.Unmarshal(b, &agents); err != nil {
		// Leave the last good copy in place rather than dropping every agent.
		return
	}

	s.mu.Lock()
	s.agents = agents
	s.mu.Unlock()

	s.agentsMod = fi.ModTime()
	s.agentsSize = fi.Size()
}

func (s *Store) saveChannels() error {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	s.mu.RLock()
	st := channelState{NextSeq: s.nextSeq, Channels: s.channels}
	s.mu.RUnlock()

	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(s.dataDir, "channels.json"), b)
}

// PostChannelMessage appends one message to a channel and returns it with its
// assigned sequence number.
func (s *Store) PostChannelMessage(channel, sender, body string) (ChannelMessage, error) {
	id, err := GenerateID()
	if err != nil {
		return ChannelMessage{}, err
	}

	s.mu.Lock()
	s.nextSeq++
	msg := ChannelMessage{
		ID:        id,
		Seq:       s.nextSeq,
		Channel:   channel,
		Sender:    sender,
		Body:      body,
		CreatedAt: time.Now().UTC(),
	}
	log := append(s.channels[channel], msg)
	if len(log) > maxChannelMessages {
		log = log[len(log)-maxChannelMessages:]
	}
	s.channels[channel] = log
	s.mu.Unlock()

	if err := s.saveChannels(); err != nil {
		return ChannelMessage{}, err
	}
	return msg, nil
}

// GetChannelMessages returns channel messages with Seq greater than since,
// oldest first, at most limit entries (limit is clamped to 1..500).
func (s *Store) GetChannelMessages(channel string, since uint64, limit int) []ChannelMessage {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]ChannelMessage, 0, limit)
	for _, m := range s.channels[channel] {
		if m.Seq <= since {
			continue
		}
		out = append(out, m)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// ListChannels returns channel summaries sorted by name.
func (s *Store) ListChannels() []ChannelInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.channels))
	for name := range s.channels {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]ChannelInfo, 0, len(names))
	for _, name := range names {
		log := s.channels[name]
		var last uint64
		if len(log) > 0 {
			last = log[len(log)-1].Seq
		}
		out = append(out, ChannelInfo{Name: name, LastSeq: last, Count: len(log)})
	}
	return out
}

// ChannelExists reports whether a channel is known to the relay.
func (s *Store) ChannelExists(channel string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.channels[channel]
	return ok
}
