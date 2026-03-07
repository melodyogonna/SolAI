# SolAI — Architecture Plan

## Overview

An autonomous AI agent that interacts with the Solana blockchain. Built in Go using [langchaingo](https://tmc.github.io/langchaingo/docs/) with a user-configured LLM as the reasoning engine (Google, OpenAI, or Anthropic). The agent runs in a continuous loop, coordinating a suite of user-installed agentic tools to accomplish user-defined goals. If a goal cannot be accomplished with available tools, the agent reports this rather than hallucinating actions.

---

## Core Design Principles

- **Hierarchical multi-agent system.** SolAI is a coordinator agent that delegates to subagents. Each agentic tool is itself an LLM agent — it receives a prompt from the main agent, reasons about it using its own LLM, calls the specific capability it wraps (an API, an RPC endpoint, a smart contract), and returns a structured result.
- **The coordinator plans; subagents execute.** The main agent never calls external APIs or executes transactions directly. It decomposes goals into prompts and dispatches them to the appropriate subagent tools.
- **Two-layer memory across cycles.** A new coordinator agent instance is created each cycle, but state is carried forward via two mechanisms: (1) a shared `ConversationWindowBuffer` passed to each executor — `chains.Call` automatically saves and loads the last 3 cycles' conversation; (2) an explicit `memory` capability tool the LLM calls to persist structured state (current plan, observations, pending tasks). Together they allow multi-cycle strategies without unbounded context growth.
- **Sandboxed by default.** Both the coordinator and each subagent tool run in isolated bubblewrap (bwrap) sandboxes. Capabilities explicitly widen what is allowed.
- **Tools as plugins.** Agentic tools are packaged and distributed as GitHub releases, installable via `solai install`. No code changes to the agent are required to add a tool.

---

## Startup Flow

```
solai start
  └─ Load ~/.solai/config.json
  └─ Validate: api_key and user_goals must be set
  └─ Extract embedded bwrap binary → /tmp/bwrap-*
  └─ Launch agent sandbox:
       bwrap --unshare-all [--share-net]
         --ro-bind ~/.solai/config.json  /etc/solai/config.json
         --ro-bind ~/.solai/tools        /tools
         --ro-bind <solai-binary>        /solai
         --proc /proc --dev /dev --tmpfs /tmp
         --die-with-parent
         -- /solai __agent-run

__agent-run (inside sandbox)
  └─ Read config from /etc/solai/config.json
  └─ Init LLM from cfg.Model (provider + name, key from cfg.Providers)
  └─ Register capabilities: wallet, network-manager
  └─ agent.Run(ctx, cfg, capManager)
```

---

## Coordinator Cycle Loop

Cycles run back-to-back with no sleep between them. Each cycle has a `cycle_timeout` (default 5m) after which it is cancelled and a new one starts immediately.

```
SystemManager.Setup()
  └─ Extract bwrap for subagent sandboxing (into /tmp inside agent sandbox)
  └─ Discover tools from /tools/ — load manifests, validate capabilities
  └─ Log configured LLM providers

buildCyclePrompt()   [rebuilt every cycle]
  └─ ## Available Tools — capabilities with non-empty Description + agentic tools
  └─ ## Agent Memory — injected from MemoryCapability.BuildMemorySection() if non-empty
  └─ ## Your Goals — user goals from config

runCycle()  [OneShotAgent, max 10 iterations, exponential retry up to 3×]
  └─ ReAct: Reason → pick subagent → formulate prompt → Act → Observe result → repeat
  └─ Each "Act" spawns a subagent process:
       writes ToolInput (type=input, overview, tasks, available_capabilities) to stdin
       runs bidirectional request/response loop:
         ← tool may write {"type":"request",...} for capability actions
         → coordinator dispatches to Regular capability, writes {"type":"response",...}
       reads final {"type":"success"|"error",...} from stdout
  └─ On transient error (e.g. LLM rate limit): retry with 2s→30s exponential backoff
  └─ On ErrNotFinished: log warning, no retry, continue to next cycle
  └─ On cycle_timeout: log warning, continue to next cycle
```

---

## Agentic Tools

### What they are

Agentic tools are **LLM subagents** — each one wraps a specific capability (a Solana RPC call, a DEX API, a price feed, etc.) and exposes it to the coordinator as a single callable tool. When the coordinator invokes a tool, the tool binary:

1. Receives a natural-language prompt from the coordinator (overview + task list)
2. Spins up its own internal LLM agent
3. Uses that agent to reason about the prompt and call its wrapped capability
4. Returns a structured result to the coordinator

This means the coordinator never needs to know the details of how a capability works — it just describes what it wants accomplished and the subagent figures out how.

Agentic tools are standalone executables (any language) distributed as GitHub releases. Each has a subdirectory under `~/.solai/tools/`:

```
~/.solai/tools/<name>/
  manifest.json        — metadata, executable path, capabilities required
  bin/<name>           — statically linked binary (chmod 0755)
```

### IPC protocol

The coordinator communicates with each subagent via **bidirectional** stdin/stdout JSON. All messages carry a `type` field so each side can distinguish them without additional framing. The tool process lifetime is one invocation — it receives a prompt, runs its internal agent loop, optionally requests coordinator capabilities, writes its final result, and exits.

**Initial input** (coordinator → subagent, written to stdin at startup):
```json
{
  "type": "input",
  "overview": "One sentence describing the objective.",
  "tasks": ["Step 1", "Step 2", "Step 3"],
  "available_capabilities": "## Capability Requests\n..."
}
```

`available_capabilities` is a generated Markdown block documenting every `Regular` capability that has a non-empty `ToolRequestDescription`. It describes the request/response protocol and the available actions for each capability. Empty string when no such capabilities are registered.

**Final output** (subagent → coordinator, written to stdout):
```json
{ "type": "success", "output": <any JSON value> }
{ "type": "error",   "output": "human-readable error string" }
```

Errors are surfaced as strings so the coordinator observes them as Observations in its ReAct loop and can adapt (retry with different input, try a different tool, or report the failure).

**Capability request** (subagent → coordinator, mid-execution):
```json
{ "type": "request", "capability": "<name>", "action": "<action>", "input": "<value>" }
```

After writing a request the tool blocks reading stdin. The coordinator dispatches the request to the named `Regular` capability and writes back:

```json
{ "type": "response", "output": "<value>" }
{ "type": "response", "error":  "human-readable error string" }
```

Only `Regular` capabilities are accessible this way — `Core` and `Internal` capabilities are silently rejected with an error response. This lets tools perform privileged operations (e.g. signing a transaction with the wallet) without receiving secrets directly.

### Tool sandbox (nested bwrap)

Each tool subprocess runs in its own bwrap instance inside the agent sandbox (nested unprivileged namespaces, supported on Linux 5.x+):

```
bwrap --unshare-all
  --ro-bind /tools/<name>  /app
  --tmpfs /tmp --proc /proc --dev /dev
  --die-with-parent
  [--share-net]   ← only if "network-manager" in required_capabilities
  -- /app/<name>
```

### Tool manifest format

```json
{
  "name": "<tool-name>",
  "description": "What this tool does.",
  "version": "1.0.0",
  "executable": "./bin/<tool-name>",
  "required_capabilities": ["network-manager"],
  "env": [
    { "name": "SOME_API_KEY", "sensitive": true,  "required": true  },
    { "name": "SOME_BASE_URL", "sensitive": false, "required": false }
  ],
  "llm_options": {
    "primary": "gemini-2.5-pro",
    "supported": [
      { "model": "gemini-2.5-pro", "provider": "google" },
      { "model": "gpt-4o",         "provider": "openai" }
    ]
  }
}
```

`llm_options` is required for any tool that uses an internal LLM agent (which is the expected pattern). The coordinator resolves credentials from `config.providers` and injects `SOLAI_LLM_PROVIDER`, `SOLAI_LLM_MODEL`, and `SOLAI_LLM_API_KEY` into the subagent's environment. Tools that are purely deterministic (no reasoning required) may omit `llm_options`.

`env` declares runtime variables the tool needs. Values are set via `solai config set tool-env.<tool>.<VAR> <value>` and injected into the tool's environment at runtime. `sensitive: true` causes the value to be redacted in `config list` output. `required: true` causes the agent to refuse to load the tool if the value is missing.

### Tool installation

```
solai install <name[@tag]>          — curated registry (index.json hosted on GitHub)
solai install owner/repo[@tag]      — direct GitHub release install
solai uninstall <name>              — remove installed tool

Short-name path:
  └─ Fetch registry/index.json from GitHub
  └─ Resolve name → versioned asset URLs
  └─ Download manifest.json, binary, verify SHA256 against checksums.txt (if present)
  └─ Write to ~/.solai/tools/{name}/

GitHub path:
  └─ GET github.com/repos/{owner}/{repo}/releases/{latest|tag}
  └─ Download manifest.json → validate name, description, executable fields
  └─ Download {name}-linux-{amd64|arm64}
  └─ Verify SHA256 against checksums.txt (if present)
  └─ Write to ~/.solai/tools/{name}/manifest.json
  └─ Write to ~/.solai/tools/{name}/bin/{name} (chmod 0755)

Both paths: abort early if the tool is already installed.
Tool name from remote manifest is validated against [a-z0-9][a-z0-9-]* before any file operations.
```

---

## Capabilities

Capabilities are system-level features registered at startup. They run inside the agent process and serve one or more of three roles: injecting context into the coordinator prompt, acting as callable tools for the coordinator LLM, or being requestable by agentic tools at runtime.

| Class | Accessible to | Purpose |
|---|---|---|
| `Core` | Nobody | Background infrastructure; never exposed to any LLM |
| `Internal` | Coordinator LLM only | Exposed as callable tools to the coordinator; description injected into the cycle prompt |
| `Regular` | Coordinator LLM **and** agentic tools | Same as Internal, plus tools can request actions via the capability request protocol at runtime |

The distinction between `Internal` and `Regular` is intentional: some capabilities (e.g. Solana RPC) hold private keys and should only be callable by the trusted coordinator LLM. Others (e.g. wallet signing) are safe to expose to tools under the coordinator's supervision — the coordinator LLM reviews each tool invocation before it runs.

A `Regular` capability may also have no runtime actions (e.g. `network-manager`) — in that case `ToolRequestDescription()` returns empty and no request docs are generated, but the capability still appears in the coordinator prompt and grants sandbox permissions.

Capabilities with an empty `Description()` (e.g. `network-manager`) are excluded from both the coordinator's tool list and the `## Available Tools` prompt section. This prevents infrastructure-only capabilities from confusing the LLM with misleading action suggestions.

### Implemented capabilities

| Name | Class | Effect |
|---|---|---|
| `system-manager` | Core | Owns tool loading, LLM provider logging, cleanup jobs, sandbox extraction |
| `wallet` | Regular | Derives ed25519 keypair from BIP39 seed; provides `address` and `sign` actions requestable by tools; coordinator can also call it directly |
| `network-manager` | Regular | Grants `--share-net` to tool sandboxes that declare it; no runtime request actions |
| `solana` | Internal | Solana RPC access (balance, transfer, blockhash, send_transaction, account_info); coordinator LLM only — tools use `wallet` to sign, then pass the transaction to the coordinator |
| `memory` | Internal | Two-layer cross-cycle state: (a) `ConversationWindowBuffer` (last 3 cycles, automatic via langchaingo); (b) structured plan/observation/task store callable by coordinator LLM |

### Planned capabilities

| Name | Class | Effect |
|---|---|---|
| `file-manager` | Regular | Bind-mounts user-configured paths into tool sandboxes |
| `web-ui` | Internal | Web interface for configuring agent and tools |

---

## Configuration

Stored in `~/.solai/config.json`. Managed via `solai config set/get/list`.

```json
{
  "model": {
    "provider": "google",
    "name": "gemini-2.5-pro"
  },
  "providers": {
    "google": "",
    "openai": "",
    "anthropic": ""
  },
  "wallet_seed": "",
  "cycle_timeout": "5m",
  "user_goals": "",
  "sandbox": {
    "share_net": true,
    "extra_binds": []
  },
  "tool_env": {
    "<tool-name>": {
      "VAR_NAME": "value"
    }
  }
}
```

`model` selects the coordinator's LLM. At least one provider credential must be set, and it must match `model.provider`.

`providers` holds credentials for all configured providers. The coordinator uses `providers[model.provider]`. Agentic tools draw from this map independently — a tool whose `llm_options.primary` is `openai` will use `providers.openai` even if the coordinator is configured for Google. This lets the coordinator and tools use different models.

`cycle_timeout` caps the duration of a single coordinator cycle (default 5m). There is no sleep between cycles.

`tool_env` holds per-tool runtime variables declared in the tool's `env` manifest field. Set via `solai config set tool-env.<tool>.<VAR> <value>`. Sensitive values are redacted in `config list`.

`sandbox.extra_binds` accepts `[{"path": "/host/path", "read_only": true}]` entries that are bind-mounted into the agent sandbox (not tool sandboxes — use `file-manager` for that).

---

## Shared modules

| Module | Purpose |
|---|---|
| `ratelimit` | `RateLimitStrategy` and `RetryStrategy` interfaces + implementations. Used by the agent (exponential retry on LLM calls) and by tools (fixed-window rate limiting for external APIs). |

### Implementations

| Type | Struct | Parameters |
|---|---|---|
| Rate limit | `FixedWindowLimiter` | `limit int`, `window time.Duration` — at most N calls per window |
| Retry | `ExponentialRetry` | `maxAttempts`, `initialDelay`, `maxDelay`, `multiplier` — backs off exponentially, respects context cancellation |

---

## What is not yet implemented

| Feature | Notes |
|---|---|
| File manager capability | `FSBinds` field exists in `SandboxPolicy`; no capability wires it up yet |
| Web UI | Planned long-term; listed as an Internal capability |
| Signal-based IPC | Original design used file directories + signal channel; simplified to direct stdin/stdout. May revisit for long-running tools |
| LLM env vars in sandboxed agent | Provider keys from `config.providers` are available to tools but not currently forwarded as env vars into the outer agent sandbox itself |
