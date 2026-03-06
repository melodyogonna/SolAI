# SolAI

An autonomous AI agent for the Solana blockchain. SolAI runs a continuous reasoning loop powered by a user-configured LLM (Google, OpenAI, or Anthropic), coordinating a suite of external tools to accomplish user-defined goals — monitoring token prices, reading on-chain data, executing transactions, or anything else a tool can be built to do.

---

## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/melodyogonna/solai/main/install.sh | bash
```

This downloads the latest `solai` binary for your architecture (`x86_64` or `aarch64`) and places it in `~/.local/bin`. If that directory is not on your `PATH` the script will print the exact command to add it.

## Quick start

```bash
# Select which model the coordinator uses
solai config set model.provider google
solai config set model.name gemini-2.5-pro

# Set API credentials (add as many providers as you have keys for —
# they are made available to agentic tools that need an LLM)
solai config set provider.google <your-google-api-key>

solai config set user-goals "Monitor SOL price and report every cycle"

solai install token-price   # install a tool
solai start                 # launch the agent inside a sandbox
```

---

## How it works

SolAI operates in a **ReAct loop** (Reason → Act → Observe). Each cycle:

1. The agent is given a system prompt (its role and rules) and the user goals (what to accomplish).
2. It plans which tools to call and in what order.
3. It calls those tools, observes results, and adapts.
4. It produces a structured summary of what was accomplished, what failed, and what to try next.
5. It immediately begins the next cycle. Each cycle has a configurable timeout (`cycle-timeout`) to prevent runaway LLM calls.

Each cycle creates a **fresh agent instance** — there is no state carried between cycles at the LLM level. Persistent state lives in tools and on-chain.

---

## CLI

```
solai install <owner/repo[@tag]>   Install a tool from a GitHub release
solai config set <key> <value>     Set a configuration value
solai config get <key>             Get a configuration value
solai config list                  List all values (sensitive fields redacted)
solai start [--no-sandbox]         Start the autonomous agent
```

### Configuration keys

**Coordinator model** (required — pick one provider and model):

| Key | Description |
|---|---|
| `model.provider` | LLM provider for the coordinator: `google`, `openai`, or `anthropic` |
| `model.name` | Model name (e.g. `gemini-2.5-pro`, `gpt-4o`, `claude-opus-4-6`) |

**Provider credentials** (set any you have — the coordinator uses its own, tools use whichever matches their `llm_options`):

| Key | Description |
|---|---|
| `provider.google` | Google AI API key |
| `provider.openai` | OpenAI API key |
| `provider.anthropic` | Anthropic API key |

**Agent settings:**

| Key | Description |
|---|---|
| `user-goals` | Goals the agent should pursue autonomously |
| `cycle-timeout` | Maximum time allowed for a single cycle before it is cancelled and the next begins (default: `5m`) |
| `wallet-seed` | BIP39 mnemonic — a new wallet is generated if unset |
| `sandbox.share-net` | Allow agent sandbox network access (default: `true`) |

**Solana settings:**

| Key | Description |
|---|---|
| `solana.rpc-url` | Solana RPC endpoint (default: `https://api.mainnet-beta.solana.com`) |
| `solana.commitment` | Commitment level: `finalized`, `confirmed`, or `processed` (default: `confirmed`) |

Configuration is stored in `~/.solai/config.json` and written atomically.

---

## Architecture

```
solai start
  └─ config.Load()                  ~/.solai/config.json
  └─ sandbox.Extract()              embedded bwrap binary → /tmp/bwrap-*
  └─ bwrap --unshare-all            agent sandbox
       --ro-bind ~/.solai/config.json /etc/solai/config.json
       --ro-bind ~/.solai/tools       /tools
       --ro-bind <solai-binary>       /solai
       -- /solai __agent-run

__agent-run (inside sandbox)
  └─ agent.LoadConfigFrom()         reads /etc/solai/config.json
  └─ googleai.New()                 initialises Gemini LLM
  └─ capability.SetUp()             wallet, network-manager
  └─ agent.Run()                    autonomous cycle loop
       └─ SystemManager.Setup()     loads tools from /tools, extracts bwrap for tool sandboxing
       └─ SystemManager.Start(ctx)  background cleanup jobs
       └─ ReAct cycle loop
            └─ buildCyclePrompt()   injects capability context (wallet address, etc.)
            └─ runCycle()           OneShotAgent → chains.Run
                 └─ AgenticTool.Call()  spawns tool subprocess via stdin/stdout JSON
                      └─ bwrap (nested)  each tool runs in its own sandbox
```

### Packages

| Package | Responsibility |
|---|---|
| `cli/` | Cobra commands: `install`, `config`, `start`, `__agent-run` |
| `config/` | `~/.solai/config.json` — load, save, set, get |
| `registry/` | Tool installation from GitHub releases |
| `agent/` | Autonomous cycle loop, prompt assembly, config loading |
| `capability/` | Capability system: wallet, LLM provider registry, SystemManager |
| `tool/` | Tool discovery, manifest parsing, subprocess IPC |
| `sandbox/` | Embedded bwrap binary, extraction |
| `prompts/` | Embedded system prompt (`system.md`) |
| `wallet/` | BIP39 seed derivation, ed25519 keypair, Base58 encoding |

### Sandboxing

Tools and the agent itself are isolated using [bubblewrap](https://github.com/containers/bubblewrap) (bwrap), a lightweight unprivileged sandbox.

**Agent sandbox** (`solai start`): the `solai` binary re-invokes itself inside bwrap with `--unshare-all`. Only `~/.solai/config.json`, `~/.solai/tools/`, and the binary itself are visible inside. Networking is enabled by default so the agent can reach the Gemini API.

**Tool sandbox** (nested): each tool subprocess gets its own bwrap instance. The tool directory is mounted read-only at `/app`. Network access is only granted to tools that declare `"required_capabilities": ["network-manager"]` in their manifest.

### Capabilities

Capabilities are system-level features injected at startup, separate from agentic tools.

| Class | Visibility | Example |
|---|---|---|
| `Core` | Invisible — background infrastructure | `SystemManager` |
| `Internal` | Registered as callable LLM tools; also injected into the cycle prompt | `WalletCapability`, `SolanaCapability` |
| `Regular` | Grants sandbox permissions to tools that request them | `NetworkManagerCapability` |

**Built-in capabilities:**

| Capability | Description |
|---|---|
| `wallet` | Returns the agent's Solana wallet public address |
| `solana` | Direct Solana RPC: `get_balance`, `transfer_sol`, `get_recent_blockhash`, `send_transaction`, `get_account_info` |

---

## Tools

### Installing tools

```bash
solai install token-price                       # latest release (core/curated tool)
solai install token-price@v1.0.0               # specific version
solai install owner/repo                        # third-party tool (latest release)
solai install owner/repo@v1.0.0                # third-party tool (specific tag)
```

Tools are downloaded from GitHub releases and stored in `~/.solai/tools/`. The release must include:

- `manifest.json` — tool manifest with `"executable": "./bin/<name>"`
- `<name>-linux-amd64` / `<name>-linux-arm64` — statically linked binary
- `checksums.txt` (optional) — SHA256 checksums for verification

### Writing a tool

Tools are standalone executables (any language) that communicate via stdin/stdout JSON.

**IPC protocol:**

```json
// Written to tool stdin
{ "overview": "One sentence describing the objective.", "tasks": ["Step 1", "Step 2"] }

// Read from tool stdout
{ "type": "success", "output": "..." }
{ "type": "error",   "output": "something went wrong" }
```

Tool errors are returned as strings in `output` so the LLM can observe them in the ReAct loop and adapt.

**manifest.json:**

```json
{
  "name": "token-price",
  "description": "Fetches current USD prices for Solana tokens from Jupiter.",
  "version": "1.0.0",
  "executable": "./bin/token-price",
  "required_capabilities": ["network-manager"]
}
```

For tools that need their own LLM, add `llm_options`:

```json
{
  "name": "my-tool",
  "description": "...",
  "version": "1.0.0",
  "executable": "./bin/my-tool",
  "llm_options": {
    "primary": "gemini-2.5-pro",
    "supported": [
      { "model": "gemini-2.5-pro", "provider": "google" },
      { "model": "gpt-4o",         "provider": "openai" }
    ]
  }
}
```

The agent injects `SOLAI_LLM_PROVIDER`, `SOLAI_LLM_MODEL`, and `SOLAI_LLM_API_KEY` into the tool's environment automatically.

### Adding a tool locally (without a GitHub release)

1. Create `~/.solai/tools/my-tool/manifest.json` (see format above, `executable` must be `"./bin/my-tool"`).
2. Place the binary at `~/.solai/tools/my-tool/bin/my-tool` (chmod 0755).
3. The tool is discovered automatically on the next `solai start`.
