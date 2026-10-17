package main

import (
	"context"
	"testing"
)

func TestHandleAskCommand_UsageAndFlags(t *testing.T) {
	ctx := context.Background()

	// 1. No arguments: prints usage
	handleAskCommand(ctx, []string{})

	// 2. Help flag: prints usage
	handleAskCommand(ctx, []string{"--help"})

	// 3. Empty question after flags: prints error & usage
	handleAskCommand(ctx, []string{"-y", "--project", t.TempDir()})

	// 4. JSON flag with question
	handleAskCommand(ctx, []string{"--json", "-y", "--top-k", "2", "test question"})

	// 5. Paid / force-paid flag
	handleAskCommand(ctx, []string{"--paid", "--tier", "paid", "test paid question"})
}
