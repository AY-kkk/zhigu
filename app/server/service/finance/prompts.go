package finance

import (
	_ "embed"
	"strings"
)

//go:embed prompts/synthesizer.md
var synthesizerPrompt string

func SynthesizerPrompt() string {
	return ResearchScopePrefix + "\n" + strings.TrimSpace(synthesizerPrompt) + "\n"
}
