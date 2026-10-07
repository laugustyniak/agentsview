package ingest

import (
	"slices"
	"strings"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/signals"
)

// ToolResultContentUnknown classifies individual retained results before agent
// labels and separators are added for display. Selection matches the summary's
// latest nonempty result per agent, including the anonymous result.
func ToolResultContentUnknown(call db.ToolCall) bool {
	switch len(call.ResultEvents) {
	case 0:
		return signals.IsUnknownResultContent(call.ResultContent)
	case 1:
		return signals.IsUnknownResultContent(call.ResultEvents[0].Content)
	}
	seen := make(map[string]bool)
	found := false
	for _, event := range slices.Backward(call.ResultEvents) {
		if strings.TrimSpace(event.Content) == "" {
			continue
		}
		agent := strings.TrimSpace(event.AgentID)
		if seen[agent] {
			continue
		}
		seen[agent] = true
		if !signals.IsUnknownResultContent(event.Content) {
			return false
		}
		found = true
	}
	return found
}

// ClassifyToolResult uses the sequence classifier while projected content is available.
func ClassifyToolResult(name, category, content, status string) string {
	result := signals.ExtractToolSequences([]signals.ToolCallRow{{ToolName: name, Category: category, ResultContent: content, ResultContentLength: len(content), EventStatus: status, ResultContentUnknown: signals.IsUnknownResultContent(content)}}, false)
	return string(result.Calls[0].Outcome)
}
