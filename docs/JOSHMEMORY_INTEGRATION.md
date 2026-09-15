# ForgeGrid ↔ JoshMemory integration

Last updated: 15 September 2026

## Boundary

ForgeGrid is the fleet execution/orchestration plane. JoshMemory is the shared development-continuity/provenance layer. GitHub and the live checkout remain the authority for code state.

The current JoshMemory shared-state default is an always-available private GitHub backing store (`joshualparris/JoshDashboard4`, `joshmemory-cloud/v1/`), so ForgeGrid and AVANCE-WS7 are **not** uptime dependencies for development handoffs.

## Expected workflow

Before an agent/worker continues a software task:

1. identify the canonical Git repository and local checkout;
2. load the latest JoshMemory handoff/project context;
3. inspect live branch, HEAD, dirty state and required runtime evidence;
4. treat live evidence as authoritative when it conflicts with memory;
5. execute only the still-valid work;
6. record meaningful checkpoint/blocker/next-action context back to JoshMemory.

ForgeGrid should return execution evidence; JoshMemory may retain a reference/summary of that evidence but should not fabricate verification.

## Fleet roles

- **ForgeGrid:** normal distributed execution and worker evidence.
- **AVANCE-WS7 / DadLAN control plane:** optional coordinator/policy host.
- **Action1:** bootstrap/recovery/Windows admin side channel.
- **JoshMemory:** shared continuity/history/provenance.
- **GitHub:** durable code truth.
- **AgentCheck / AgentWitness / LLMAccountability:** independent evidence/accountability sources as applicable.

## Do not

- Do not use a shared SMB/NFS SQLite file as fleet memory.
- Do not make AVANCE-WS7 the only copy of handoff state.
- Do not allow an old handoff to overrule live Git or machine state.
- Do not put credentials into handoff payloads.
- Do not mark a task verified solely because an executing agent says it is complete.

## Roadmap

The next fleet-specific improvement is a shared lease/current-work record so multiple agents can see whether a task is already actively owned before resuming it.

Canonical implementation/history: https://github.com/joshualparris/JoshMemory
