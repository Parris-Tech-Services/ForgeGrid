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
	Meta     map[string]ChannelMeta      `json:"meta,omitempty"`
}

type Store struct {
	mu       sync.RWMutex
	dataDir  string
	agents   map[string]AgentRegistration
	messages map[string]AgentMessage
	channels map[string][]ChannelMessage
	chanMeta map[string]ChannelMeta
	nextSeq  uint64

	// reloadMu serialises agents.json re-reads; agentsMod/agentsSize remember
	// the last version seen so an unchanged file is not parsed on every request.
	reloadMu   sync.Mutex
	agentsMod  time.Time
	agentsSize int64

	// filesMu serialises JSON state file reads and writes (see writeFile).
	filesMu sync.Mutex

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
		chanMeta: make(map[string]ChannelMeta),
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
		if st.Meta != nil {
			s.chanMeta = st.Meta
		}
		s.nextSeq = st.NextSeq
	}
	return nil
}

// saveAgentsLocked writes agents.json. Callers hold s.mu.
func (s *Store) saveAgentsLocked() error {
	agB, err := json.MarshalIndent(s.agents, "", "  ")
	if err != nil {
		return err
	}
	return s.writeFile("agents.json", agB)
}

// saveMessagesLocked writes messages.json. Callers hold s.mu.
func (s *Store) saveMessagesLocked() error {
	msgB, err := json.MarshalIndent(s.messages, "", "  ")
	if err != nil {
		return err
	}
	return s.writeFile("messages.json", msgB)
}

// writeFile writes through the shared file mutex with a short retry, because a
// concurrent reader (or another process running `register`) can transiently
// hold the destination open.
func (s *Store) writeFile(name string, data []byte) error {
	s.filesMu.Lock()
	defer s.filesMu.Unlock()

	path := filepath.Join(s.dataDir, name)
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if err = atomicWrite(path, data); err == nil {
			return nil
		}
		time.Sleep(time.Duration(20*(attempt+1)) * time.Millisecond)
	}
	return err
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
	return s.saveAgentsLocked()
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
	return msg, s.saveMessagesLocked()
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
	if err := s.saveMessagesLocked(); err != nil {
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

	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()

	// Hold the file mutex only for the stat and read; never while taking s.mu,
	// so a writer holding s.mu cannot deadlock against a reload.
	s.filesMu.Lock()
	fi, err := os.Stat(path)
	if err != nil {
		s.filesMu.Unlock()
		return
	}
	if fi.ModTime().Equal(s.agentsMod) && fi.Size() == s.agentsSize {
		s.filesMu.Unlock()
		return
	}
	b, err := os.ReadFile(path)
	s.filesMu.Unlock()
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

// saveChannelsLocked persists channel state. Callers hold s.mu; it acquires the
// save mutex only, so it must not be called while holding saveMu.
func (s *Store) saveChannelsLocked() error {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	st := s.channelStateLocked()
	return s.writeFile("channels.json", st)
}

// channelStateLocked snapshots channel state. Callers hold at least s.mu.RLock.
func (s *Store) channelStateLocked() []byte {
	st := channelState{NextSeq: s.nextSeq, Channels: s.channels, Meta: s.chanMeta}
	b, _ := json.MarshalIndent(st, "", "  ")
	return b
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
	s.claimChannelOwnerLocked(channel, sender)
	err = s.saveChannelsLocked()
	s.mu.Unlock()
	if err != nil {
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
		meta := s.chanMeta[name]
		out = append(out, ChannelInfo{
			Name:     name,
			LastSeq:  last,
			Count:    len(log),
			Owner:    meta.Owner,
			Members:  meta.Members,
			ReadOnly: meta.ReadOnly,
		})
	}
	return out
}

// ChannelMetaOf returns the policy for a channel. An unknown or unconfigured
// channel has the zero policy, which allows every authenticated agent.
func (s *Store) ChannelMetaOf(channel string) ChannelMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.chanMeta[channel]
}

// SetChannelMeta stores a channel policy. A channel with no owner is claimed by
// whoever configures it first.
func (s *Store) SetChannelMeta(channel string, meta ChannelMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.chanMeta[channel]; !ok {
		s.chanMeta[channel] = meta
	} else {
		s.chanMeta[channel] = meta
	}
	if _, ok := s.channels[channel]; !ok {
		s.channels[channel] = []ChannelMessage{}
	}
	return s.saveChannelsLocked()
}

// ClaimChannelOwner records the first poster as owner of a new channel.
// Callers hold s.mu; this is a no-op when an owner is already recorded.
func (s *Store) claimChannelOwnerLocked(channel, owner string) {
	meta, ok := s.chanMeta[channel]
	if ok && meta.Owner != "" {
		return
	}
	meta.Owner = owner
	s.chanMeta[channel] = meta
}

// ChannelExists reports whether a channel is known to the relay.
func (s *Store) ChannelExists(channel string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.channels[channel]
	return ok
}
