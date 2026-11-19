# Bounded AI-assisted security testing: ForgeGrid integration contract
**11 November 2026. Public architecture note; no privileged deployment or external security scan.**

## Research basis

Recent AI-security findings show strong discovery capabilities, but also a need for independent reproduction and review:
- [OpenAI: A thousand fewer paths to root (November 2026)](https://openai.com/index/a-thousand-fewer-paths-to-root/): reproducible Linux kernel vulnerability research and proposed patches at research-scale costs, not evidence of installed patches.
- [Anthropic: Project Glasswing initial update (May 2026)](https://www.anthropic.com/research/glasswing-initial-update): AI-generated findings create verification/disclosure bottlenecks.
- [Google: PageBreak (September 2026)](https://blog.google/security/agentic-hacks-real-proofs-inside-googles-pagebreak-project/): dynamic proof-oriented validation helps manage false positives.
- [UC Berkeley: CyberGym-E2E (June 2026)](https://rdi.berkeley.edu/blog/cybergym-e2e/): historical benchmark separately measures reproduction, patched behaviour and functional regressions.
- [Microsoft: Intune Vulnerability Remediation Agent](https://learn.microsoft.com/en-us/intune/copilot/agents/manage-vulnerability-remediation-agent): analysis/recommendation is separate from privileged device update execution.

These are first-party reports/product documentation plus a benchmark study; they do not prove the same fix success rate on this repository or provide an unrestricted vulnerability-scanning model.

## Role in AutoPatch

ForgeGrid is an **existing paired, TLS-protected build/test worker infrastructure**, **not** an OS patch-control plane. Do not add a privileged shell, SSH/WinRM launcher, arbitrary remote command, persistent admin credentials or automatic firewall/firmware access. Keep the restrictions in [SECURITY.md](../SECURITY.md), including authenticated pairing and workspace boundaries.

Recommended interface for an explicitly enrolled disposable test worker only:

1. **Input**: pinned repository commit and signed/hashed fixture reference, purpose, timeout, expected stdout/stderr filtering, no credentials or unrestricted URLs.
2. **Allowed job**: pre-defined, narrowly allowlisted build/test/lint or security regression profile already approved for that worker. No dynamic command supplied by LLM text, no production device or network targeting.
3. **Output**: test result, provenance hash, fixture commit, build digest, elapsed time, redacted log, unambiguous PASS/FAIL/UNKNOWN and reason. Treat agent logs as untrusted.
4. **Verification**: controller compares original vulnerable fixture test against patched fixture, requires an independent reviewer and scanner, and publishes private evidence to the existing owner dashboard.
5. **Fail closed**: offline worker, malformed evidence, failed test, missing credentials, uncertain authorisation or budget exceeded => UNKNOWN/BLOCKED, never SECURE or AUTO-MERGE.
6. **Isolation**: ephemeral workspace, strict quotas, no mounted home directory or secrets, default-deny egress except approved registries when required; no privileged Docker socket.

## Existing practical guardrails

ForgeGrid's existing [CI](../.github/workflows/ci.yml) runs Linux and Windows Go builds/tests/vet plus an independent race detector. Run these on **every** candidate change. A previously observed integration-test temporary-directory cleanup race motivated PR #31; preserve clean shutdown and test evidence before merging its fix. Go dependency update PR #30 and any overlapping proposals require fresh CI/merge-policy verification before adoption. This document neither resolves their merge status nor declares them complete.

Use Go module and GitHub Actions Dependabot PRs (subject to repository settings), not a second ad hoc package updater. When proposed versions break tests, stop for independent review rather than disguising failures as a green status.

## Threat model

| Boundary | Risk | Defensive test |
|---|---|---|
| Pairing credential / TLS fingerprint | Worker identity spoofing and remote abuse | Reject bad pairing codes and untrusted certificate fingerprints |
| Coordinator/job API | Unauthorised scheduling / status mutation | Authentication, rate limiting, job owner checks and explicit profiles |
| Workspace and file paths | Path traversal / filesystem escape | Confined workspace fixtures, rejection tests for absolute/symlink paths |
| Job output and logs | Secrets leaking through worker logs or agent prompts | Redact, bound sizes, neutralise untrusted content |
| Background worker lifetime | Writes after shutdown; stale job status | Context cancellation, clean test teardown and race detector |
| Dependency and build runner | Supply-chain compromise | Locked Go module provenance, readonly CI permissions and verification |

## Measurable next acceptance

In a sandbox, one known/synthetic vulnerability fixture must fail before a candidate patch and pass afterwards; regression tests must remain green on Linux and Windows, with a separate reviewer, fixed commit hash and independent rescan. The AutoPatch coordinator may classify the proof **PATCH_TESTED** but must not claim **VERIFIED_FIXED** until the authorised deployed target is checked. Strictly **no** automatic OS patching through ForgeGrid. Model calls disabled until approved budgets/model access exist.

**Status: documentation of a constrained potential integration only. No remote patching, live scanning or deployment activated.**
