package insight

import (
	"context"
	"fmt"
	"path/filepath"

	"Dendrite/pkg/rea/analysis"
)

// Commenter posts a comment on a ticket; *ticketlock.Lifecycle is the real one.
type Commenter interface {
	Comment(ctx context.Context, ticket, kind, body string) error
}

// Summary is the bounded note an analysis leaves on a ticket: what it found and how to read more.
func Summary(res *analysis.Result) string {
	st := res.State
	text := fmt.Sprintf("REA analysis %s: %s (%s, %s), tier %s, stage %s.", st.ID, filepath.Base(st.Binary), st.Kind, st.Format, st.Tier, st.Stage)
	if b := res.Bundle; b != nil {
		text += fmt.Sprintf(" Evidence: ~%d tokens (truncated: %t); dependencies: %d, exports: %d.", b.TokenEstimate, b.Truncated, len(b.CrossReferences), len(b.StringConstants))
	}
	return text + fmt.Sprintf(" Read more with `dendrite retro search %s \"<question>\"`; resume with `dendrite retro resume %s`.", st.ID, st.ID)
}

// PostNote leaves the summary of res on ticket as a note.
func PostNote(ctx context.Context, c Commenter, ticket string, res *analysis.Result) error {
	return c.Comment(ctx, ticket, "note", Summary(res))
}
