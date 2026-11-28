package main

// Minimal macOS/portable entry point for the AgentBridge relay and client.
// ForgeGrid's own binary cannot build on darwin because internal/execution only
// implements setupCmd/manageProcess for linux and windows. AgentBridge needs
// neither, so this exposes the same `agent-bridge <cmd>` surface without the
// job-executor package. Invoked identically to the main binary so that
// agentbridge.getClient's os.Args[3:] flag parsing keeps working.

import (
	"fmt"
	"os"

	"forgegrid/internal/agentbridge"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "agent-bridge" {
		agentbridge.RunCLI(os.Args[2:])
		return
	}
	fmt.Fprintln(os.Stderr, "usage: agentbridge agent-bridge <serve|rotate-tls|register|configure-client|reset-client|send|inbox|ack|complete|fail> [args...]")
	os.Exit(1)
}
