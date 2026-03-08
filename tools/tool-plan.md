# Agentic Tool Development Guide

Agentic tools are standalone executables that use an internal LLM + tools (ReAct loop) to handle a subtask, then return a structured result to the SolAI coordinator. This document captures lessons learned and best practices.

## Structure

Each tool is a directory under `tools/` containing:
- `manifest.json` — tool metadata and capability requirements
- A compiled binary or script (referenced by `executable` in the manifest)
- Source files (Go recommended): `main.go`, `llm.go`, `models.go`

### manifest.json
```json
{
  "name": "tool-name",
  "description": "What this tool does (shown to the coordinator LLM)",
  "version": "1.0.0",
  "executable": "./tool-name",
  "required_capabilities": ["network-manager"]
}
```

## File-based IPC

The coordinator communicates with tools via JSON files in a temp directory. The path is passed via the `SOLAI_IPC_DIR` environment variable (inside the sandbox it is always `/run/solai`).

**`$SOLAI_IPC_DIR/input.json`** (coordinator → tool):
```json
{
  "type": "input",
  "prompt": "User's high-level goal",
  "tasks": ["specific task 1", "task 2"],
  "capabilities": { "wallet_address": "ABC...XYZ" },
  "payload": "",
  "error_details": ""
}
```

`prompt` is always present. `tasks` may be empty on re-invocation. `payload` carries data from a prior capability response (e.g. a signed transaction). `error_details` is set when a capability request previously failed.

**`$SOLAI_IPC_DIR/output.json`** (tool → coordinator):
```json
{"type": "success", "payload": <json-value>}
{"type": "error",   "payload": "<error string>"}
{"type": "request", "payload": {"capability":"<name>","action":"<action>","input":"<data>","description":"<short note>"}}
```

Write exactly one JSON object to `output.json` and exit. Never write anything to stdout (use stderr for logs).

## System Prompt Design

### Required preamble

Every agentic tool using `OneShotAgent` (ReAct) must include this block at the top of the system prompt (passed via `agents.WithPromptPrefix`):

```
IMPORTANT: You MUST follow the ReAct format for EVERY response. Always begin
with "Thought:" and end with either "Action:"/"Action Input:" (to call a tool)
or "Final Answer:" (when done). Never output free-form text outside this
format. You MUST call a tool before giving a Final Answer — never answer from
memory or training data.

OUTPUT RULES: Your Final Answer must contain ONLY the result data — no
meta-commentary, no statements like "I will compile", "Here is the
information", "Based on the results", or any other preamble. Output the
data directly.
Tool inputs must be plain text — never wrap Action Input in markdown code
fences (no ```json or ``` blocks).
```

**Why:** Modern LLMs (Gemini, Claude, GPT-4) often ignore the ReAct format built into langchaingo's template and respond conversationally. The preamble overrides that tendency. Without it you will see errors like `unable to parse agent output: Of course, here is...`.

### Tool descriptions

Tool descriptions are the primary way the internal LLM decides which tool to call and how to format inputs. Be explicit:

- State the exact input format (comma-separated, plain string, JSON fields).
- Give a concrete example.
- State what is returned.

```go
func (t *myTool) Description() string {
    return `Short purpose statement.
Input: comma-separated token symbols or mint addresses (e.g. "SOL,USDC,JUP").
Returns JSON array with price_usd and change_24h_pct per token.`
}
```

## Input Parsing in Tool Calls

### Always strip markdown fences

Even with prompt instructions, LLMs sometimes wrap `Action Input` in markdown code fences. Strip them defensively at the top of every `Call` method:

```go
func stripMarkdownFence(s string) string {
    s = strings.TrimSpace(s)
    for _, fence := range []string{"```json", "```"} {
        if strings.HasPrefix(s, fence) {
            s = strings.TrimPrefix(s, fence)
            s = strings.TrimSuffix(strings.TrimSpace(s), "```")
            s = strings.TrimSpace(s)
            break
        }
    }
    return s
}
```

Apply it as the first line: `input = stripMarkdownFence(input)`.

### Handle JSON object inputs for list-type tools

When a tool expects a comma-separated list, the LLM may pass a JSON object like `{"mints":["So111...","JUP..."]}` or a JSON array. Parse defensively:

```go
func parseTokenList(input string) []string {
    // Try JSON array
    var arr []string
    if json.Unmarshal([]byte(input), &arr) == nil {
        return arr
    }
    // Try JSON object with common key names
    var obj map[string]json.RawMessage
    if json.Unmarshal([]byte(input), &obj) == nil {
        for _, k := range []string{"mints", "tokens", "ids", "symbols", "addresses"} {
            if raw, ok := obj[k]; ok {
                var list []string
                if json.Unmarshal(raw, &list) == nil {
                    return list
                }
            }
        }
    }
    // Fall back to comma-separated
    return strings.Split(input, ",")
}
```

### Handle JSON object inputs for string-type tools

When a tool expects a plain string query, the LLM may pass `{"query":"..."}` or `{"q":"..."}`:

```go
input = strings.TrimSpace(stripMarkdownFence(input))
var obj map[string]string
if json.Unmarshal([]byte(input), &obj) == nil {
    for _, k := range []string{"query", "q", "input", "search"} {
        if v, ok := obj[k]; ok {
            input = v
            break
        }
    }
}
```

## ReAct Parse Error Recovery

langchaingo's `OneShotAgent` returns an error like `"unable to parse agent output: <raw LLM text>"` when the model skips the ReAct format. Rather than surfacing this as a hard failure, extract the LLM text and return it as the result:

```go
result, err := chains.Run(ctx, executor, prompt)
if err != nil {
    const parsePrefix = "unable to parse agent output: "
    if i := strings.Index(err.Error(), parsePrefix); i >= 0 {
        result = err.Error()[i+len(parsePrefix):]
    } else {
        writeError(fmt.Sprintf("agent run failed: %v", err))
        return
    }
}
```

**Note:** In the fallback case the model answered from training data (no tool calls were made). For tools that require live data (prices, swaps) this is undesirable but still better than returning an error to the coordinator. Consider logging a warning to stderr.

## LLM Initialisation

Read provider/model/key from env vars injected by the coordinator:

```go
provider := os.Getenv("SOLAI_LLM_PROVIDER") // "google" | "openai" | "anthropic"
model    := os.Getenv("SOLAI_LLM_MODEL")
apiKey   := os.Getenv("SOLAI_LLM_API_KEY")
```

## Capability Requests

If a tool needs the coordinator to perform a privileged action (e.g. signing a transaction), it writes a `"request"` output to `output.json` and exits immediately — no blocking.

```go
capReq, _ := json.Marshal(CapabilityRequest{
    Capability:  "wallet",
    Action:      "sign",
    Input:       unsignedTxBase64,
    Description: "Sign and submit the swap transaction",
})
writeOutput(ToolOutput{Type: "request", Payload: json.RawMessage(capReq)})
os.Exit(0)
```

The coordinator reads the request, calls the named capability, then **re-invokes the tool** with a new `input.json` where:
- `payload` holds the capability result (e.g. the signed transaction)
- `prompt` is derived from the request's `description` field

On failure the coordinator sets `error_details` instead of `payload`.

The tool detects re-invocation by checking `input.Payload != ""` and handles the second phase accordingly. See `tools/jupiter-swap/main.go` for the full two-phase pattern.

Pre-injected capabilities (like `wallet_address`) are available immediately in `input.Capabilities` — no request needed for read-only values.

## Checklist for a New Tool

- [ ] `manifest.json` with correct `required_capabilities`
- [ ] Read input from `$SOLAI_IPC_DIR/input.json` as `ToolInput{Prompt, Tasks, Capabilities, Payload, ErrorDetails}`
- [ ] Write exactly one JSON object to `$SOLAI_IPC_DIR/output.json` (`success`, `error`, or `request`) and exit
- [ ] If using capability requests: check `input.Payload != ""` for re-invocation phase
- [ ] ReAct format + output rules preamble in system prompt
- [ ] `stripMarkdownFence` applied in every tool `Call`
- [ ] Defensive JSON parsing for list/string inputs
- [ ] Parse error recovery around `chains.Run`
- [ ] LLM constructed from `SOLAI_LLM_*` env vars
