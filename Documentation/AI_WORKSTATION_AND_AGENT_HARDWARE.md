# AI Workstation & Agent Hardware Guide

_Last updated: 15 September 2026_

This document consolidates the workstation, mini-PC, local-AI, browser-automation and multi-agent hardware discussions that sit around ForgeGrid and the DadLAN/Avance development fleet.

## 1. Current procurement case: Shannon's AI automation workstation

### Intended workload

The workstation is intended primarily for:

- ChatGPT desktop and similar cloud-AI clients
- AI agents that interact with local applications and browser sessions
- Chrome automation, potentially with about 52 tabs open at once
- Heavy multitasking across browser processes, office applications and automation tools
- Development and testing of agent workflows
- Possible future participation as a ForgeGrid worker/coordinator

This is **not the same workload as running a frontier model locally**. For ChatGPT, Claude, Gemini and most hosted agents, the expensive model inference happens in the provider's cloud. The local PC still needs substantial RAM and CPU capacity for browser renderers, automation processes, local tools, downloads, indexing, test workloads and any local helper models.

### Baseline recommendation

For this use case, **64 GB RAM is the sensible starting point**. It provides comfortable headroom for dozens of Chrome tabs, the ChatGPT desktop app, multiple local tools and several automation processes without pushing Windows into constant paging.

A strong modern 8–16 core CPU is useful, but the absolute fastest mobile/workstation CPU is less important than:

1. enough RAM;
2. sustained cooling;
3. fast, reliable NVMe storage;
4. serviceability;
5. quiet all-day operation;
6. sensible networking; and
7. upgrade room.

A discrete GPU is unnecessary for cloud-first ChatGPT/browser automation. Add one only if there is a real local-model, CUDA, rendering, gaming or GPU-compute requirement.

---

## 2. Minisforum MS-A2 assessment

Reference: [Minisforum Australia MS-A2 product page](https://au.minisforum.com/products/minisforum-ms-a2)

The MS-A2 is a genuinely powerful compact system and is technically capable of Shannon's workload.

### Relevant specifications

- AMD Ryzen 9 9955HX
- Zen 5
- 16 cores / 32 threads
- Up to 5.4 GHz boost
- Radeon 610M integrated graphics
- Up to 96 GB DDR5 SO-DIMM memory
- Multiple NVMe/U.2 storage options
- PCIe expansion
- Dual 2.5 GbE plus dual 10 Gb SFP+
- Compact 196 × 189 × 48 mm chassis

### Important power clarification

The statement that the CPU is simply a "55 W CPU" is incomplete.

AMD lists the Ryzen 9 9955HX with a **55 W default TDP** and **55–75 W configurable TDP**. Minisforum advertises the MS-A2 implementation at **up to 100 W turbo TDP**. That means the cooling system has to deal with substantially more than 55 W during boost-heavy work.

Sources:

- [AMD Ryzen 9 9955HX specifications](https://www.amd.com/en/products/processors/laptop/ryzen/9000-series/amd-ryzen-9-9955hx.html)
- [Minisforum MS-A2 specifications](https://au.minisforum.com/products/minisforum-ms-a2)

### Cooling and sustained-load behaviour

Independent Notebookcheck testing found the MS-A2 to be very fast, but also found that the cooling system reaches its limits during prolonged heavy CPU load. The CPU settles back toward roughly the 70–75 W region under long stress runs, with high temperatures and audible fan operation.

Reference: [Notebookcheck MS-A2 review](https://www.notebookcheck.net/Minisforum-MS-A2-review-Compact-AMD-mini-PC-with-workstation-ambitions-and-GPU-upgrade-option.1062179.0.html)

This does **not** mean the system is unsafe. Modern CPUs are designed to manage temperature and power automatically. The longer-term concerns for an all-day business workstation are more practical:

- small high-speed fan wear;
- dust sensitivity;
- more fan noise;
- less thermal mass;
- hotter internal SSD/RAM environment;
- sustained-performance throttling;
- less standardised replacement hardware than a normal desktop.

### Where the MS-A2 is excellent

Choose it when compact size is genuinely valuable and you want unusually strong networking and expansion in a very small box.

It is particularly attractive for:

- compact lab/server roles;
- portable high-core-count compute;
- SFP+/10 Gb networking;
- homelab use;
- space-constrained offices;
- a powerful ForgeGrid node where physical size matters.

### Where it is over-specified for Shannon

For browser/ChatGPT automation, Shannon is unlikely to make meaningful use of:

- dual 10 Gb SFP+;
- U.2 enterprise-style storage;
- much of the specialised networking;
- PCIe expansion in such a constrained thermal envelope.

Those features are impressive, but they are not the main things that make her workflow fast.

---

## 3. Preferred long-life office build

If Brendan can build a conventional desktop for roughly the same overall budget, that is the preferred path for a machine expected to run hard for years.

### Recommended specification

| Component | Recommendation | Reason |
|---|---|---|
| CPU | Modern Ryzen 9 9900X-class / strong 12–16 core desktop CPU | Excellent multitasking without relying on a tiny cooling system |
| RAM | 64 GB DDR5, 2 DIMMs | Best starting point for 50+ tabs and parallel agents |
| RAM ceiling | Prefer board support for 96/128 GB+ | Easy future expansion |
| Storage | 2 TB quality NVMe SSD | Room for profiles, downloads, caches, dev tools and local models |
| Motherboard | B850/X870-class or equivalent reliable business board | Expansion and long-term parts availability |
| Cooling | Large quality tower cooler | Low noise and sustained boost without thermal stress |
| Case | Ventilated micro-ATX or compact ATX | Still reasonably small, but much easier to cool and clean |
| PSU | Quality ~650 W unit | Efficient, replaceable and leaves GPU headroom |
| GPU | Integrated graphics initially | Hosted AI/browser automation does not need a discrete GPU |
| OS | Windows 11 Pro | Best fit for desktop automation and business management |
| Network | 2.5 GbE is already more than adequate for most office use | Spend elsewhere unless there is a real 10 Gb requirement |

### Why the desktop wins for longevity

A modestly larger system provides:

- slower/larger fans;
- lower acoustic load;
- easier dust cleaning;
- replaceable standard fans;
- replaceable power supply;
- standard RAM and storage access;
- easier future GPU addition;
- better sustained CPU performance;
- easier fault diagnosis;
- more local repair options.

### Procurement brief for Brendan

> Can you build a workstation around the same budget as the Minisforum MS-A2, with roughly equivalent real-world CPU performance and 64 GB RAM, but prioritising quiet sustained operation, cooling, reliability, standard replaceable parts and future expansion rather than making it tiny? It will mainly run ChatGPT/AI agents, browser automation with up to roughly 52 Chrome tabs, office apps and development tools. A discrete GPU is not required initially unless there is a clear local-AI reason for one. Prefer 2 TB NVMe storage and an easy path to 96/128 GB RAM later.

---

## 4. Memory planning for browser and agent workloads

Chrome's memory use varies dramatically by site, extensions, media, background activity and whether pages are discarded. Therefore the number of tabs alone is not a precise RAM calculator.

Practical tiers:

| Workload | RAM target |
|---|---:|
| Normal office + cloud AI | 32 GB |
| 40–60 tabs + multiple agents + dev tools | **64 GB** |
| Heavy browser automation + VMs/containers + local models | 96 GB |
| Large local-model/VM lab | 128 GB+ |

For Shannon, 64 GB is the sweet spot today. Buy the platform so it can grow beyond that without replacing the whole machine.

---

## 5. Cloud AI vs local AI: what hardware changes

### Cloud-first agents

Examples: ChatGPT, Claude, Gemini, hosted coding agents.

The PC mainly handles:

- browser/app processes;
- local automation;
- file indexing;
- IDEs;
- downloads/uploads;
- local MCP/tool servers;
- screenshots and browser control;
- testing and build processes.

Priorities: **RAM, CPU responsiveness, SSD, reliability, network stability**.

### Local models

When a model runs locally through Ollama/llama.cpp/etc., the requirements change.

The existing fleet discussions established that small quantised models can be useful on CPU/RAM-only machines, but large frontier-class models are not realistic on ordinary office PCs. A practical pattern is:

- small local models for private/offline helper tasks;
- cloud APIs for flagship reasoning/coding models;
- ForgeGrid for distributing deterministic jobs and agent work across machines;
- one stronger GPU box only when a real local-model workload justifies it.

A prior reference point was Qwen3.5 4B Q4_K_M on the HP ProDesk/i5-10500T-class coordinator: roughly a few GB of model memory and useful CPU-only speed for lightweight local assistance, but nowhere near hosted frontier performance.

---

## 6. Related DadLAN / ForgeGrid hardware history

ForgeGrid grew out of the idea of reusing a mixed fleet of older laptops/desktops rather than requiring every task to run on one giant workstation.

### Known fleet roles

| Machine/class | Role |
|---|---|
| AVANCE-WS7 / HP ProDesk 400 G6 Mini / i5-10500T / ~24 GB | Coordinator/controller, orchestration, routing |
| JParrisDesktop / i5-9400 / 16 GB / GTX 1660 6 GB | Strongest existing GPU/local-model box |
| Ryzen 5 5600U / 16 GB laptop | Higher-priority intelligent worker |
| ThinkPad L480-class machines | General dev/test workers; RAM upgrades worthwhile when cheap |
| N6000/N4500 and older low-end systems | Test/execution/monitoring workers |
| Very old 3–4 GB systems | Only lightweight execution/monitoring; replace rather than heavily upgrade |

The broader lesson is still useful for new purchases: **match the machine to the job**. A browser/agent workstation needs different optimisation from a local-model GPU server, a ForgeGrid worker or a game/test node.

---

## 7. Mini-PC buying lessons from earlier research

Earlier mini-PC sourcing research showed a broad ladder:

- Intel N95/N97/N100: cheap lightweight workers/browser endpoints;
- Ryzen 5 5500U/5700U/5800U: inexpensive general-purpose agent/dev nodes;
- Ryzen 7 7840HS/8845HS-class: strong compact workstations;
- Ryzen 9 HX / MS-A2-class: extreme CPU density, but cooling/noise become more important;
- dedicated GPU systems: only worth paying for when a GPU workload exists.

This is why the MS-A2 is not a bad choice—it is simply optimised for **maximum capability per litre**, while Shannon's priority is better described as **maximum dependable productivity per dollar over years**.

---

## 8. Agent and browser automation design principles

The workstation should support tools that can operate Chrome and local apps, but the hardware purchase should not be used as a substitute for good automation architecture.

Recommended principles:

- keep automation identities separated from personal browser profiles;
- use least-privilege accounts and tokens;
- avoid storing plaintext secrets in repos or scripts;
- make destructive actions explicit and reviewable;
- log agent actions sufficiently for troubleshooting;
- keep browser automation restartable after crashes;
- prefer idempotent tasks where possible;
- use ForgeGrid for job leasing/status rather than assuming a worker always completes;
- isolate concurrent code agents with branches/worktrees;
- require real-machine verification for hardware/Windows deployment changes;
- treat green CI as necessary but not sufficient evidence that fleet deployment is healthy.

These fit the existing ForgeGrid security model: authenticated workers, protected execution, leasing/state transitions, auditability and isolated work.

---

## 9. Decision summary

### For Shannon

**Preferred:** Brendan-built micro-ATX/compact desktop, 64 GB RAM, 2 TB NVMe, strong modern 12–16 core Ryzen, large air cooler, integrated graphics initially, standard replaceable components.

**Very good compact fallback:** Minisforum MS-A2 with 64 GB RAM, particularly if space or portability is important and the price advantage is substantial.

**Do not buy a discrete GPU by default** for ChatGPT/Chrome automation. Add one only when a local-model or GPU workload has been identified.

### Decision rule

If a well-built desktop is within a reasonable margin of the MS-A2 price, choose the desktop for longevity and serviceability. If the desktop is substantially more expensive or compactness is important, the MS-A2 remains a capable option—prefer a balanced power profile rather than chasing maximum sustained 100 W CPU operation.

---

## 10. Future roadmap

- [ ] Record Shannon's final quote and purchased configuration
- [ ] Run a 30–60 minute CPU/RAM/browser soak test after delivery
- [ ] Test 60-tab Chrome session plus ChatGPT desktop plus automation processes
- [ ] Log peak RAM use, CPU package temperature, SSD temperature and fan/noise behaviour
- [ ] Add the machine as a ForgeGrid worker if useful
- [ ] Decide whether any real workload justifies a discrete GPU
- [ ] Standardise a Windows agent/browser automation profile
- [ ] Document recovery/re-image procedure before the workstation becomes business-critical
- [ ] Reassess RAM after three months of real workload data
