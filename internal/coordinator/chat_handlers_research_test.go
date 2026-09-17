package coordinator

import "testing"

func TestShouldResearchPrompt(t *testing.T) {
	for _, prompt := range []string{
		"What's the weather today?",
		"What is the latest news?",
		"current price of bitcoin",
		"Who won tonight's game?",
	} {
		if !shouldResearchPrompt(prompt) {
			t.Errorf("expected research prompt: %q", prompt)
		}
	}
	for _, prompt := range []string{
		"Explain how mutexes work",
		"Help me write a Go function",
		"What is recursion?",
	} {
		if shouldResearchPrompt(prompt) {
			t.Errorf("unexpected research prompt: %q", prompt)
		}
	}
}
