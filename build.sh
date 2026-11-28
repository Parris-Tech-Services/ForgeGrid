#!/bin/bash
set -euo pipefail

echo "Building ForgeGrid..."
GOOS=windows GOARCH=amd64 go build -o dist/ForgeGrid-USB/Windows/ForgeGrid.exe main.go
GOOS=linux GOARCH=amd64 go build -o dist/ForgeGrid-USB/Linux/forgegrid main.go

# AgentBridge relay/client for macOS. The main binary cannot build on darwin
# because internal/execution implements setupCmd/manageProcess only for linux
# and windows. AgentBridge needs neither, so macOS gets cmd/agentbridge, which
# exposes the same `agent-bridge <command>` surface.
mkdir -p dist/ForgeGrid-USB/macOS
GOOS=darwin GOARCH=amd64 go build -o dist/ForgeGrid-USB/macOS/agentbridge-amd64 ./cmd/agentbridge
GOOS=darwin GOARCH=arm64 go build -o dist/ForgeGrid-USB/macOS/agentbridge-arm64 ./cmd/agentbridge

# MCP stdio server, for hosting the board as tools inside Claude Code, Codex,
# DSH's mcp-client, or anything else that speaks MCP.
mkdir -p dist/ForgeGrid-USB/mcp
GOOS=darwin GOARCH=amd64 go build -o dist/ForgeGrid-USB/mcp/agentbridge-mcp-darwin-amd64 ./cmd/agentbridge-mcp
GOOS=darwin GOARCH=arm64 go build -o dist/ForgeGrid-USB/mcp/agentbridge-mcp-darwin-arm64 ./cmd/agentbridge-mcp
GOOS=linux GOARCH=amd64 go build -o dist/ForgeGrid-USB/mcp/agentbridge-mcp-linux-amd64 ./cmd/agentbridge-mcp
GOOS=windows GOARCH=amd64 go build -o dist/ForgeGrid-USB/mcp/agentbridge-mcp-windows-amd64.exe ./cmd/agentbridge-mcp

cd dist/ForgeGrid-USB
if command -v sha256sum >/dev/null 2>&1; then
  SUM="sha256sum"
else
  SUM="shasum -a 256"
fi

$SUM Windows/ForgeGrid.exe Linux/forgegrid \
  macOS/agentbridge-amd64 macOS/agentbridge-arm64 \
  mcp/agentbridge-mcp-darwin-amd64 mcp/agentbridge-mcp-darwin-arm64 \
  mcp/agentbridge-mcp-linux-amd64 mcp/agentbridge-mcp-windows-amd64.exe \
  Windows/START-FORGEGRID.bat Linux/start-forgegrid.sh \
  README.html Examples/forgegrid.yaml > CHECKSUMS.txt

echo "Build complete."
