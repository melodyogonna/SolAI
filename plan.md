# SolAI — Architecture Plan

## Overview

An autonomous AI agent that interacts with the Solana blockchain. Built in Go using [langchaingo](https://tmc.github.io/langchaingo/docs/) with a user-configured LLM as the reasoning engine (Google, OpenAI, or Anthropic). The agent runs in a continuous loop, coordinating a suite of user-installed agentic tools to accomplish user-defined goals. If a goal cannot be accomplished with available tools, the agent reports this rather than hallucinating actions.

---

## Core Design Principles

- **Hierarchical multi-agent system.** SolAI is a coordinator agent that delegates to subagents. Each agentic tool is itself an LLM agent — it receives a prompt from the main agent, reasons about it using its own LLM, calls the specific capability it wraps (an API, an RPC endpoint, a smart contract), and returns a structured result.
- **The coordinator plans; subagents execute.** The main agent never calls external APIs or executes transactions directly. It decomposes goals into prompts and dispatches them to the appropriate subagent tools.
- **Fresh state per cycle.** A new coordinator agent instance is created each cycle. No LLM state bleeds between cycles. Persistent state lives in tools and on-chain.
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

Every `cycle_interval` (default 5m):

```
SystemManager.Setup()
  └─ Extract bwrap for subagent sandboxing (into /tmp inside agent sandbox)
  └─ Discover tools from /tools/ — load manifests, validate capabilities
  └─ Log configured LLM providers

buildCyclePrompt()
  └─ Append capability context (e.g. wallet public key) to user goals

runCycle()  [OneShotAgent, max 10 iterations]
  └─ ReAct: Reason → pick subagent → formulate prompt → Act → Observe result → repeat
  └─ Each "Act" spawns a subagent process: writes ToolInput to stdin, reads ToolOutput from stdout
  └─ On ErrNotFinished: log warning, continue to next cycle
  └─ On timeout (2× cycle_interval): log warning, continue

Sleep cycle_interval (or wake early on SIGINT/SIGTERM)
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

```
Main coordinator (user-configured LLM)
  ├─ "Get me the current SOL price" → token-price subagent
  │     └─ internal LLM → calls Jupiter price API → returns { price_usd: 142.50 }
  ├─ "What is the balance at address XYZ?" → solana-balance subagent
  │     └─ internal LLM → calls Solana RPC getBalance → returns { balance: 4.2 }
  └─ "Swap 1 SOL for USDC" → swap-executor subagent
        └─ internal LLM → builds Jupiter swap tx → signs → submits → returns tx sig
```

Agentic tools are standalone executables (any language) distributed as GitHub releases. Each has a subdirectory under `~/.solai/tools/`:

```
~/.solai/tools/<name>/
  manifest.json        — metadata, executable path, capabilities required
  bin/<name>           — statically linked binary (chmod 0755)
```

### IPC protocol

The coordinator communicates with each subagent via stdin/stdout JSON. The tool process lifetime is one invocation — it receives a prompt, runs its internal agent loop to completion, writes its result, and exits.

**Input** (coordinator → subagent, written to stdin):
```json
{
  "overview": "One sentence describing the objective.",
  "tasks": ["Step 1", "Step 2", "Step 3"]
}
```

The `overview` and `tasks` are a structured prompt. The subagent's internal LLM interprets them and determines how to use its wrapped capability to satisfy the request.

**Output** (subagent → coordinator, read from stdout):
```json
{ "type": "success", "output": <any JSON value> }
{ "type": "error",   "output": "human-readable error string" }
```

Errors are surfaced as strings so the coordinator observes them as Observations in its ReAct loop and can adapt (retry with different input, try a different tool, or report the failure).

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
  "name": "token-price",
  "description": "Fetches USD prices for Solana tokens from Jupiter.",
  "version": "1.0.0",
  "executable": "./bin/token-price",
  "required_capabilities": ["network-manager"],
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

### Tool installation

```
solai install owner/repo[@tag]
  └─ GET github.com/repos/{owner}/{repo}/releases/{latest|tag}
  └─ Download manifest.json → parse tool name
  └─ Download {name}-linux-{amd64|arm64}
  └─ Verify SHA256 against checksums.txt (if present)
  └─ Write to ~/.solai/tools/{name}/manifest.json
  └─ Write to ~/.solai/tools/{name}/bin/{name} (chmod 0755)
```

---

## Capabilities

Capabilities are system-level features registered at startup. They are not LLM tools — they run inside the agent process and either inject context into the prompt or grant sandbox permissions to tools.

| Class | Visible to | Purpose |
|---|---|---|
| `Core` | Nobody | Background infrastructure (SystemManager) |
| `Internal` | Main LLM only | Inform the agent about itself (e.g. wallet public key in prompt) |
| `Regular` | Main LLM + tools | Grant sandbox permissions to tools that request them |

### Implemented capabilities

| Name | Class | Effect |
|---|---|---|
| `system-manager` | Core | Owns tool loading, LLM provider logging, cleanup jobs, sandbox extraction |
| `wallet` | Internal | Derives ed25519 keypair from BIP39 seed; exposes public key in cycle prompt |
| `network-manager` | Regular | Grants `--share-net` to tool sandboxes that declare it |

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
  "cycle_interval": "5m",
  "user_goals": "",
  "sandbox": {
    "share_net": true,
    "extra_binds": []
  }
}
```

`model` selects the coordinator's LLM. At least one provider credential must be set, and it must match `model.provider`.

`providers` holds credentials for all configured providers. The coordinator uses `providers[model.provider]`. Agentic tools draw from this map independently — a tool whose `llm_options.primary` is `openai` will use `providers.openai` even if the coordinator is configured for Google. This lets the coordinator and tools use different models.

`sandbox.extra_binds` accepts `[{"path": "/host/path", "read_only": true}]` entries that are bind-mounted into the agent sandbox (not tool sandboxes — use `file-manager` for that).

---

## What is not yet implemented

| Feature | Notes |
|---|---|
| Permission request system | Tools can output `"type": "request"` — not yet handled by the agent; planned for a future capability |
| File manager capability | `FSBinds` field exists in `SandboxPolicy`; no capability wires it up yet |
| Web UI | Planned long-term; listed as an Internal capability |
| Signal-based IPC | Original design used file directories + signal channel; simplified to direct stdin/stdout. May revisit for long-running tools |
| LLM env vars in sandboxed agent | Provider keys from `config.providers` are available to tools but not currently forwarded as env vars into the outer agent sandbox itself |
