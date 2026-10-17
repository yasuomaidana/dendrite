package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"Dendrite/pkg/cli"
)

func TestExitCodes(t *testing.T) {
	ctx := context.Background()
	reg := cli.NewRegistry()
	reg.Register(&cli.Command{
		Name: "that-cmd",
		Run: func(ctx context.Context, args []string) error {
			return errors.New("boom")
		},
	})

	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{
			name:     "nil",
			err:      nil,
			expected: 0,
		},
		{
			name:     "unknown name bogus",
			err:      reg.Execute(ctx, []string{"bogus"}),
			expected: 2,
		},
		{
			name:     "wrapped generic error",
			err:      fmt.Errorf("wrapped: %w", errors.New("generic")),
			expected: 1,
		},
		{
			name:     "that-cmd returning boom",
			err:      reg.Execute(ctx, []string{"that-cmd"}),
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitCodeFor(tt.err); got != tt.expected {
				t.Fatalf("exitCodeFor(%v) = %d; want %d", tt.err, got, tt.expected)
			}
		})
	}

	if got := exitCodeFor(reg.Execute(ctx, []string{"that-cmd"})); got != 1 {
		t.Fatalf("exitCodeFor(reg.Execute(ctx, []string{\"that-cmd\"})) = %d; want 1", got)
	}
}
