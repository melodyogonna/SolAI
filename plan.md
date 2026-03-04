# Overview

This is an attempt to build an Autonomous AI agent that interacts with Solana, the project is built on Go and leverages [langchaingo](https://tmc.github.io/langchaingo/docs/) | https://pkg.go.dev/github.com/tmc/langchaingo@v0.1.14 project.

## Design

The basic idea is a fully autonomous AI agent that runs in a sandbox and coordinates a suite of "Agentic Tools". The agent itself would do nothing but plan how tasks would be accomplished based on available agentic tools; if the task it is supposed to work can not be accomplished by available agentic tools then it would show a message to this effect and not attempt to work on the task.

### Environment

Like I said, the agent would run inside a sandbox, I've not decided on which, I'm still building the agent ... but here is a basic idea of what I envision. The main agent would run inside a main sandbox, with system capabilities mounted on top as enabled by the user. System capabilities are system functionalities which the agent would be allowed to use outside of the sandbox, example of system capabilities are: File Management, System manager (which would manage the sandbox), Wallet manager (which would manager Solana wallets and sign transactions and requests), etc.
I hope to provide installation mediums which allow capabilities to be enabled and configured during setup.

In addition to capability the AI agent would have available a suite of agentic tools, available ones are entirely configured by the user and is the main way which the agent would gain capacity. Agentic tools are simple one-shot agents which wrap around a specific functionality or tool. They can be packaged by external contributors and installed by users of SolAI as plugins. Agentic tools would run in their own sandbox and communication between them and the main AI would be implicitly handled by some form of core system capability ... the main AI agent would just see a tool which can be called.


### Agentic tools

Like I've explained, I want agentic tools to be simple agents that wrap around specific functionalities. They need to be agentic because I want them to accept prompts, then use their tool call to accomplish the task specified in the prompt. The main AI which cordinates these agentic tools would formulate these prompts and then call the agentic tools with them.

#### Interface of agentic tools

The agentic tools would have 3 communication interfaces: input, output, and signal. input and output would be file directories mounted into the sandbox (and maybe exposed as env vars) where the agent can read the prompts for input, and finally write the output to a file in the output directory. When a prompt is written into a tools input, it does not immediately start working on it unless it receives a signal in the signal channel. In fact, it should not even know a prompt has been made, agentic tools are expected to be listening on the signal channel for signals of what to do next, this would contain signal name and payload (which is where the exact input filename would be.). After executing the agentic tool should write the result to their output directory, using the same as the input name. It should not notify anybody or anything that it has finished, we would have a system monitor looking at the output buffers(folder) of agentic tools for when a response is out.

#### Communication format

My plan is for the input and output to be communicated as json files, but maybe this is a bad idea and I'll need to find something better. Input messages would have the format:

```json
{
  "overview": "This is a description of the tasks I want you to accomplish.",
  "tasks": ["Do this first", "then this", "finally this"]
}

```

Output messages will have the following format:

```json
{
  "type": "success",
  "ouput": <Some json object repesenting output, could be a string, number, or a json object>
}
```

There'll be 3 basic types of outputs: success, error, and request.

#### Agentic tool requests

When agentic tools needs to use system capabilities not provided to them during startup to accomplish a task they need to formulate request. This is basically an output placed in their output buffer with a type of "request", this request will be passed to the main AI to analyse, if the request is valid, a permission will be granted, which the AI will sign. IF the request is invalid (or malicious) it'll be rejected and the agentic tool session terminate.

### Communication logistics

The idea I have is that there'll be a core system capability responsible for passing messages between the main agentic and the agentic tools. The agentic tools will be exposed as normal tools, LLM tool calls will be translated and passed to the correct agentic tool, if the tool is not already running then it'll be instantiated, its input and output setup, then a signal sent to execute. Outputs from these tool calls will be passed back to main agent as result of running a tool call.


## System capabilities

Another core idea is the idea of system capabilities. We'll have 3 classes of system capabilities: 
1. Core - These are system capabilities which the agent must be started with, for example - a communication manager system capability. I expect that for the most part these will run in the background and remain hidden from both main AI and agentic tools, they'll cordinate messages and manage running sandboxes etc
2. Internal - These are system capabilities that can be enabled or disabled but its existence should only be known by the main AI agent and never the agentic tools, it is only used by the core to accomplish tasks (e.g if we had a Solana keypair capability)
3. Regular - These are system capabilities that are available to the main AI, but the agentic tools can also request to use.

### System capability ideas to start:

Core:

- Communication manager: Managers communications between agentic tools and main AI.
- System manager: Responsible for setting up environments and sandboxes, mounting necessary permissions and capabilities.

Internal:

- Wallet manager: Internal tool for main AI to sign transactions and requests.
- WebUI - A system capability (planned for long term) to expose setting up the agents and tools from a web interface.

Regular:

- File Manager: Responsible for writing to disk outside of the sandbox
- Network manager: Responsible for managing networks.
