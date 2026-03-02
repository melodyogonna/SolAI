// Package prompts embeds the agent system prompt so it is available
// inside the bwrap sandbox without any file bind-mount.
package prompts

import _ "embed"

//go:embed system.md
var SystemPrompt string
