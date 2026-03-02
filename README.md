# SolAI

An autonomous AI agent for the Solana blockchain. SolAI runs a continuous reasoning loop powered by Gemini 2.5 Pro, coordinating a suite of external tools to accomplish user-defined goals — monitoring token prices, reading on-chain data, executing transactions, or anything else a tool can be built to do.

---

## How it works

SolAI operates in a **ReAct loop** (Reason → Act → Observe). Each cycle:

1. The agent is given a system prompt (its role and rules) and a user goals prompt (what to accomplish).
2. It plans which tools to call and in what order.
3. It calls those tools, observes results, and adapts.
4. It produces a structured summary of what was accomplished, what failed, and what to try next.
5. It sleeps for a configurable interval, then repeats.

Each cycle creates a **fresh agent instance** — there is no state carried between cycles at the LLM level. Persistent state lives in tools and on-chain.

---

## Architecture

```
main.go
  └─ loads config (agent/config.go)
  └─ initialises Gemini LLM
  └─ registers capabilities (wallet)
  └─ runs agent loop (agent/agent.go)
       └─ SystemManager.Setup()     ← loads tools, logs LLM providers
       └─ SystemManager.Start(ctx)  ← runs background cleanup jobs
       └─ ReAct cycle loop
            └─ buildCyclePrompt()   ← injects capability context (wallet address, etc.)
            └─ runCycle()           ← OneShotAgent → chains.Run
                 └─ AgenticTool.Call()  ← spawns tool subprocess via stdin/stdout JSON
```

### Packages

| Package | Responsibility |
|---|---|
| `agent/` | Config loading, autonomous cycle loop, prompt assembly |
| `capability/` | Capability system: wallet, LLM provider registry, SystemManager |
| `tool/` | Tool discovery, manifest parsing, subprocess IPC |
| `wallet/` | BIP39 seed derivation, ed25519 keypair, Base58 encoding |

### Capabilities

Capabilities are system-level features injected into the agent at startup. They are **not** LLM tools — they run server-side and inform the prompt.

| Class | Visibility | Example |
|---|---|---|
| `Core` | Invisible — background infrastructure | `SystemManager` |
| `Internal` | Known to the main LLM, hidden from tools | `WalletCapability` (exposes public key) |
| `Regular` | Available to both LLM and tools | _(reserved)_ |

### Agentic Tools

Tools are standalone executables (binaries or scripts) that live in the `TOOLS_DIR` directory. Each tool has its own subdirectory containing:

- `manifest.json` — name, description, version, executable path, and optional LLM config
- An executable binary or script

**IPC protocol:** The agent writes a JSON `ToolInput` to the tool's stdin and reads a JSON `ToolOutput` from stdout. Tools that need their own LLM receive `SOLAI_LLM_PROVIDER`, `SOLAI_LLM_MODEL`, and `SOLAI_LLM_API_KEY` as environment variables.

```json
// ToolInput (written to tool stdin)
{
  "overview": "One sentence describing the objective.",
  "tasks": ["Step 1", "Step 2"]
}

// ToolOutput (read from tool stdout)
{
  "type": "result",   // or "error"
  "content": "..."
}
```

Tool errors are returned as strings in the `content` field so the LLM can observe them in the ReAct loop and adapt.

#### Example tool manifest

```json
{
  "name": "token-price",
  "description": "Fetches current USD prices for Solana tokens from Jupiter.",
  "version": "1.0.0",
  "executable": "./token-price"
}
```

For tools that need an LLM, add `llm_options`:

```json
{
  "name": "my-tool",
  "description": "...",
  "version": "1.0.0",
  "executable": "./my-tool",
  "llm_options": {
    "primary": "gemini-2.5-pro",
    "supported": [
      { "model": "gemini-2.5-pro", "provider": "google" },
      { "model": "gpt-4o",         "provider": "openai" }
    ]
  }
}
```

### SystemManager

`SystemManager` is the Core capability that owns the agent's operational environment:

- Loads tools from `TOOLS_DIR` on startup via an injected `ToolLoaderFunc` (the indirection prevents a circular import between `capability` and `tool`).
- Logs which LLM providers are configured at startup.
- Runs registered `CleanupJob`s in background goroutines, each on its own ticker interval. Job errors are logged as warnings — they never crash the agent.

---

## Configuration

All configuration is via environment variables. Copy `.env.example` to `.env` (loaded automatically via `godotenv`).

| Variable | Required | Description |
|---|---|---|
| `API_KEY` | yes | Gemini API key for the main agent LLM |
| `SYSTEM_PROMPT` | yes | Path to the system prompt file |
| `USER_PROMPT` | yes | Path to the user goals file |
| `TOOLS_DIR` | yes | Path to the directory containing tool subdirectories |
| `WALLET_SEED` | no | BIP39 mnemonic. A new wallet is generated if omitted |
| `CYCLE_INTERVAL` | no | How long to sleep between cycles (default: `5m`) |
| `LLM_PROVIDER_GOOGLE` | no | Google AI API key, injected into tools that need it |
| `LLM_PROVIDER_OPENAI` | no | OpenAI API key, injected into tools that need it |
| `LLM_PROVIDER_ANTHROPIC` | no | Anthropic API key, injected into tools that need it |

---

## Running

```bash
cd solai-agent
go build -o solai .
TOOLS_DIR=../tools ./solai
```

---

## Adding a tool

1. Create a subdirectory under `TOOLS_DIR`, e.g. `tools/my-tool/`.
2. Write a `manifest.json` (see format above).
3. Build or place your executable in that directory.
4. The tool is discovered automatically on the next agent startup — no code changes required.

Tools can be written in any language. The only contract is the stdin/stdout JSON protocol.
