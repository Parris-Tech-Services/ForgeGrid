package chatstore

import (
	"path/filepath"
	"testing"
)

func TestPersistentLifecycle(t *testing.T) {
	p := filepath.Join(t.TempDir(), "llm-chat-history.json")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Create("first", "qwen3.5:4b")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(c.ID, Message{Role: "user", Content: "<script>"}, Message{Role: "assistant", Content: "safe"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Rename(c.ID, "renamed"); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Get(c.ID)
	if !ok || got.Title != "renamed" || len(got.Messages) != 2 || got.Messages[1].Sequence != 2 {
		t.Fatalf("%+v %v", got, ok)
	}
	if err := s2.Delete(c.ID); err != nil {
		t.Fatal(err)
	}
	if len(s2.List("")) != 0 {
		t.Fatal("deleted conversation listed")
	}
}
