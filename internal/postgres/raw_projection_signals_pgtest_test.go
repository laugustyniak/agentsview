//go:build pgtest

package postgres

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/ingest"
	"go.kenn.io/agentsview/internal/parser"
)

func TestRawProjectionTranscriptsOnlyToolCoverage(t *testing.T) {
	f := newHostedFixture(t, "tenant-tool-coverage")
	options := RawProjectionOptions{Tenant: f.tenant, Content: ingest.ContentOptions{ArchiveContent: config.ArchiveContentTranscripts}}
	sink, err := NewRawProjectionStore(f.runtime, options)
	require.NoError(t, err)
	outcome := projectionOutcome("Check the fixture")
	outcome.Outcome.Results[0].Result.Messages = append(outcome.Outcome.Results[0].Result.Messages, parser.ParsedMessage{
		Ordinal: 2, Role: parser.RoleAssistant, ToolCalls: []parser.ParsedToolCall{{ToolUseID: "call-2", ToolName: "Bash", Category: "Bash", InputJSON: `{"command":"true"}`}},
	})
	candidate, err := ingest.PrepareCandidate(t.Context(), outcome.Outcome.Results[0].Result, options.Content)
	require.NoError(t, err)
	prepared, err := ingest.Finalize(t.Context(), candidate, options.Content)
	require.NoError(t, err)
	require.NotNil(t, prepared.Signals.ToolObservations)
	assert.Empty(t, prepared.Signals.ToolObservations)
	revision, err := rawContentRevision(prepared)
	require.NoError(t, err)
	tx, err := f.runtime.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer tx.Rollback()
	require.NoError(t, sink.writePayload(t.Context(), tx, "fixture-session", "fixture-group", revision, prepared))
	require.NoError(t, tx.Commit())
	store := &Store{pg: f.runtime}
	rates, err := store.GetAnalyticsTools(t.Context(), db.AnalyticsFilter{From: "2026-01-01", To: "2026-01-01", Timezone: "UTC"})
	require.NoError(t, err)
	require.Len(t, rates.ByTool, 1)
	tool := rates.ByTool[0]
	assert.Equal(t, 2, tool.CallCount)
	assert.Zero(t, tool.AnalyzedCalls)
	assert.Equal(t, 2, tool.MissingCalls)
	assert.Nil(t, tool.EmptyRate)
	assert.Nil(t, tool.RepeatRate)
	assert.Nil(t, tool.RecoveryRate)
}

func TestRawProjectionEqualContentSettlesSignalsWithoutNewIdentity(t *testing.T) {
	f := newProjectionFixture(t)
	ended := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	now := ended.Add(time.Minute)
	f.sink.options.Now = func() time.Time { return now }
	outcome := projectionOutcome("Implement the requested change")
	outcome.Outcome.Results[0].Result.Session.EndedAt = ended
	outcome.Outcome.Results[0].Result.Messages = append(outcome.Outcome.Results[0].Result.Messages, parser.ParsedMessage{Ordinal: 2, Role: parser.RoleUser, Content: "Continue implementing this change"})
	a, ar := f.accept(t, "device-a", "recent-a", "")
	require.NoError(t, f.sink.Project(t.Context(), f.lease(t, a), a, outcome))
	first, err := f.sink.Resolve(t.Context(), "codex:portable")
	require.NoError(t, err)
	var pending *string
	var outcomeName string
	require.NoError(t, f.runtime.QueryRowContext(t.Context(), `SELECT outcome,signals_pending_since::text FROM sessions WHERE id=$1`, first.SessionID).Scan(&outcomeName, &pending))
	assert.Equal(t, "unknown", outcomeName)
	require.NotNil(t, pending)
	firstPending := *pending
	now = now.Add(time.Minute)
	a, ar = f.accept(t, "device-a", "recent-a-again", ar.Receipt)
	require.NoError(t, f.sink.Project(t.Context(), f.lease(t, a), a, outcome))
	recent, err := f.sink.Resolve(t.Context(), "codex:portable")
	require.NoError(t, err)
	assert.Equal(t, first.SessionID, recent.SessionID)
	assert.Equal(t, first.CorpusRevision, recent.CorpusRevision, "recent no-op receipt")
	require.NoError(t, f.runtime.QueryRowContext(t.Context(), `SELECT signals_pending_since::text FROM sessions WHERE id=$1`, first.SessionID).Scan(&pending))
	require.NotNil(t, pending)
	assert.Equal(t, firstPending, *pending)
	b, br := f.accept(t, "device-b", "equal-recent-b", "")
	require.NoError(t, f.sink.Project(t.Context(), f.lease(t, b), b, outcome))
	shared, err := f.sink.Resolve(t.Context(), "codex:portable")
	require.NoError(t, err)
	assert.Equal(t, RawIdentityUnique, shared.State)
	var outbox int
	require.NoError(t, f.runtime.QueryRowContext(t.Context(), `SELECT count(*) FROM raw_embedding_outbox`).Scan(&outbox))
	now = ended.Add(11 * time.Minute)
	a, ar = f.accept(t, "device-a", "settled-a", ar.Receipt)
	require.NoError(t, f.sink.Project(t.Context(), f.lease(t, a), a, outcome))
	settled, err := f.sink.Resolve(t.Context(), "codex:portable")
	require.NoError(t, err)
	assert.Equal(t, shared.SessionID, settled.SessionID)
	assert.Equal(t, shared.IdentityRevision, settled.IdentityRevision)
	assert.Equal(t, shared.CorpusRevision+1, settled.CorpusRevision)
	var score *int
	require.NoError(t, f.runtime.QueryRowContext(t.Context(), `SELECT outcome,signals_pending_since::text,health_score FROM sessions WHERE id=$1`, settled.SessionID).Scan(&outcomeName, &pending, &score))
	assert.Equal(t, "abandoned", outcomeName)
	assert.Nil(t, pending)
	require.NotNil(t, score)
	assert.Less(t, *score, 100)
	var afterOutbox int
	require.NoError(t, f.runtime.QueryRowContext(t.Context(), `SELECT count(*) FROM raw_embedding_outbox`).Scan(&afterOutbox))
	assert.Equal(t, outbox, afterOutbox)
	// Remove all source proof, then recreate from the durable shared revision.
	empty := outcome
	empty.Outcome.Results = nil
	a, ar = f.accept(t, "device-a", "gone-a", ar.Receipt)
	require.NoError(t, f.sink.Project(t.Context(), f.lease(t, a), a, empty))
	b, _ = f.accept(t, "device-b", "gone-b", br.Receipt)
	require.NoError(t, f.sink.Project(t.Context(), f.lease(t, b), b, empty))
	a, _ = f.accept(t, "device-a", "return-a", ar.Receipt)
	require.NoError(t, f.sink.Project(t.Context(), f.lease(t, a), a, outcome))
	require.NoError(t, f.runtime.QueryRowContext(t.Context(), `SELECT outcome,signals_pending_since::text FROM sessions WHERE id=$1`, settled.SessionID).Scan(&outcomeName, &pending))
	assert.Equal(t, "abandoned", outcomeName)
	assert.Nil(t, pending)
}
