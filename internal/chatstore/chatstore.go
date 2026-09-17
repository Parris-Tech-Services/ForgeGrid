// Package chatstore persists advisory local-LLM conversations for the
// coordinator. It deliberately stores no credentials or authorization data.
package chatstore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Sequence       int       `json:"sequence"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"created_at"`
	LatencyMs      int64     `json:"latency_ms,omitempty"`
	Model          string    `json:"model,omitempty"`
	SourceHost     string    `json:"source_host,omitempty"`
	Sources        []string  `json:"sources,omitempty"`
}

type Conversation struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Title     string    `json:"title"`
	Model     string    `json:"model"`
	Deleted   bool      `json:"deleted,omitempty"`
	Messages  []Message `json:"messages,omitempty"`
}

type diskState struct {
	Conversations map[string]Conversation `json:"conversations"`
}

type Store struct {
	mu            sync.RWMutex
	path          string
	conversations map[string]Conversation
}

const (
	maxMessageBytes      = 64 * 1024
	maxConversationBytes = 4 * 1024 * 1024
	maxHistoryBytes      = 25 * 1024 * 1024
)

func Open(path string) (*Store, error) {
	s := &Store{path: path, conversations: make(map[string]Conversation)}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var d diskState
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	if d.Conversations != nil {
		s.conversations = d.Conversations
	}
	return s, nil
}

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(diskState{Conversations: s.conversations}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	f, err := os.OpenFile(tmp, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if _, err = os.Stat(s.path); err == nil {
		_ = os.Remove(s.path + ".bak")
		if err = os.Rename(s.path, s.path+".bak"); err != nil {
			return err
		}
	}
	if err = os.Rename(tmp, s.path); err != nil {
		return err
	}
	if dir, openErr := os.Open(filepath.Dir(s.path)); openErr == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(b)
}

func (s *Store) Create(title, model string) (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	c := Conversation{ID: newID(), CreatedAt: now, UpdatedAt: now, Title: title, Model: model}
	s.conversations[c.ID] = c
	return c, s.saveLocked()
}

func (s *Store) List(query string) []Conversation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	query = strings.ToLower(strings.TrimSpace(query))
	out := make([]Conversation, 0)
	for _, c := range s.conversations {
		if c.Deleted {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(c.Title), query) && !conversationContains(c, query) {
			continue
		}
		c.Messages = nil
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

func conversationContains(c Conversation, query string) bool {
	for _, m := range c.Messages {
		if strings.Contains(strings.ToLower(m.Content), query) {
			return true
		}
	}
	return false
}

func (s *Store) Get(id string) (Conversation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.conversations[id]
	if !ok || c.Deleted {
		return Conversation{}, false
	}
	c.Messages = append([]Message(nil), c.Messages...)
	return c, true
}

func (s *Store) Rename(id, title string) (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.conversations[id]
	if !ok || c.Deleted {
		return Conversation{}, os.ErrNotExist
	}
	c.Title = strings.TrimSpace(title)
	if c.Title == "" {
		return Conversation{}, errors.New("title is empty")
	}
	c.UpdatedAt = time.Now().UTC()
	s.conversations[id] = c
	return c, s.saveLocked()
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.conversations[id]
	if !ok || c.Deleted {
		return os.ErrNotExist
	}
	delete(s.conversations, id)
	return s.saveLocked()
}

func (s *Store) Append(id string, messages ...Message) (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.conversations[id]
	if !ok || c.Deleted {
		return Conversation{}, os.ErrNotExist
	}
	for _, m := range messages {
		if len(m.Content) > maxMessageBytes {
			return Conversation{}, errors.New("message exceeds 64 KiB")
		}
	}
	if len(c.Messages) > 0 && conversationJSONSize(c, messages) > maxConversationBytes {
		return Conversation{}, errors.New("conversation exceeds 4 MiB")
	}
	// Keep the in-memory state unchanged when a later whole-history limit
	// rejects the append. Callers may retry after trimming the request.
	original := c
	for _, m := range messages {
		m.ID = newID()
		m.ConversationID = id
		m.Sequence = len(c.Messages) + 1
		if m.CreatedAt.IsZero() {
			m.CreatedAt = time.Now().UTC()
		}
		c.Messages = append(c.Messages, m)
		if c.Title == "New conversation" && m.Role == "user" {
			c.Title = strings.TrimSpace(m.Content)
			if len(c.Title) > 60 {
				c.Title = c.Title[:60] + "…"
			}
		}
	}
	c.UpdatedAt = time.Now().UTC()
	s.conversations[id] = c
	if len(mustJSON(diskState{Conversations: s.conversations})) > maxHistoryBytes {
		s.conversations[id] = original
		return Conversation{}, errors.New("history exceeds 25 MiB")
	}
	if err := s.saveLocked(); err != nil {
		s.conversations[id] = original
		return Conversation{}, err
	}
	return c, nil
}

func mustJSON(v interface{}) []byte { b, _ := json.Marshal(v); return b }
func conversationJSONSize(c Conversation, extra []Message) int {
	c.Messages = append(append([]Message(nil), c.Messages...), extra...)
	return len(mustJSON(c))
}
