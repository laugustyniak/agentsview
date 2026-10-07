package ingest_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/ingest"
)

func TestToolObservationsUseCallCoordinatesAndCompletion(t *testing.T) {
	messages := []db.Message{
		{Ordinal: 2, Role: "assistant", ToolCalls: []db.ToolCall{{ToolUseID: "duplicate", ToolName: "Grep", Category: "Grep", InputJSON: `{"pattern":"alpha"}`, ResultContent: "No matches found"}}},
		{Ordinal: 4, Role: "assistant", ToolCalls: []db.ToolCall{{ToolUseID: "duplicate", ToolName: "Grep", Category: "Grep", InputJSON: `{ "pattern": "alpha" }`, ResultContent: "No matches found"}, {ToolName: "Read", Category: "Read", ResultContent: "content"}}},
		{Ordinal: 5, Role: "assistant", ToolCalls: []db.ToolCall{{ToolName: "Read", Category: "Read", ResultContent: "content"}}},
		{Ordinal: 6, Role: "assistant", ToolCalls: []db.ToolCall{{ToolName: "Grep", Category: "Grep", ResultContent: "No matches found"}}},
	}
	for _, tc := range []struct {
		status *string
		ending string
	}{{nil, "open"}, {new("clean"), "abandoned"}, {new("awaiting_user"), "abandoned"}, {new("interrupted"), "open"}} {
		update := ingest.ComputeSignalsFromMessages(db.Session{TerminationStatus: tc.status}, messages)
		require.Len(t, update.ToolObservations, 5)
		assert.Equal(t, db.ToolObservation{MessageOrdinal: 2, Outcome: "empty", Repeat: "none", SequenceEnding: new("recovered")}, update.ToolObservations[0])
		assert.Equal(t, db.ToolObservation{MessageOrdinal: 4, Outcome: "empty", Repeat: "near_identical"}, update.ToolObservations[1])
		assert.Equal(t, db.ToolObservation{MessageOrdinal: 4, CallIndex: 1, Outcome: "content", Repeat: "none"}, update.ToolObservations[2])
		assert.Equal(t, db.ToolObservation{MessageOrdinal: 6, Outcome: "empty", Repeat: "none", SequenceEnding: new(tc.ending)}, update.ToolObservations[4])
	}
}
