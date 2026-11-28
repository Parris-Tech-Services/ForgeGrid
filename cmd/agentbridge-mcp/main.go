package main

// MCP stdio server for AgentBridge. Configure it in any MCP client:
//
//   command: agentbridge-mcp
//   env:
//     AGENT_NAME: <this machine's agent name>
//     AGENT_TOKEN: <its token>
//     RELAY_URL: https://<relay>:9090
//     RELAY_FP: <pinned fingerprint>
//
// The token stays in the client's environment; it is never passed on a command
// line, so it does not appear in process listings.

import (
	"fmt"
	"os"

	"forgegrid/internal/agentbridge"
)

func main() {
	required := []string{"AGENT_NAME", "AGENT_TOKEN", "RELAY_URL", "RELAY_FP"}
	var missing []string
	for _, key := range required {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "agentbridge-mcp: missing required environment: %v\n", missing)
		os.Exit(2)
	}

	client, err := agentbridge.NewClient(
		os.Getenv("RELAY_URL"),
		os.Getenv("AGENT_NAME"),
		os.Getenv("AGENT_TOKEN"),
		os.Getenv("RELAY_FP"),
		false,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbridge-mcp: %v\n", err)
		os.Exit(1)
	}

	if err := agentbridge.ServeMCP(os.Stdin, os.Stdout, client); err != nil {
		fmt.Fprintf(os.Stderr, "agentbridge-mcp: %v\n", err)
		os.Exit(1)
	}
}
