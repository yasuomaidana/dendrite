package tracker

import (
	"testing"
)

func TestParseStatus(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Status
		wantErr bool
	}{
		// Backlog
		{name: "backlog lower", input: "backlog", want: StatusBacklog, wantErr: false},
		{name: "backlog upper", input: "BACKLOG", want: StatusBacklog, wantErr: false},
		{name: "backlog mixed", input: "Backlog", want: StatusBacklog, wantErr: false},

		// Todo / To do
		{name: "todo lower", input: "todo", want: StatusTodo, wantErr: false},
		{name: "TODO upper", input: "TODO", want: StatusTodo, wantErr: false},
		{name: "to do space", input: "to do", want: StatusTodo, wantErr: false},
		{name: "To Do mixed", input: "To Do", want: StatusTodo, wantErr: false},
		{name: "to-do hyphen", input: "to-do", want: StatusTodo, wantErr: false},
		{name: "to_do underscore", input: "to_do", want: StatusTodo, wantErr: false},

		// In progress / wip
		{name: "in progress space", input: "in progress", want: StatusInProgress, wantErr: false},
		{name: "In-Progress prompt separator", input: "In-Progress", want: StatusInProgress, wantErr: false},
		{name: "in_progress underscore", input: "in_progress", want: StatusInProgress, wantErr: false},
		{name: "IN_PROGRESS upper", input: "IN_PROGRESS", want: StatusInProgress, wantErr: false},
		{name: "wip lower", input: "wip", want: StatusInProgress, wantErr: false},
		{name: "WIP upper", input: "WIP", want: StatusInProgress, wantErr: false},

		// In review / review / to verify / verify
		{name: "in review space", input: "in review", want: StatusInReview, wantErr: false},
		{name: "IN_REVIEW upper", input: "IN_REVIEW", want: StatusInReview, wantErr: false},
		{name: "in-review hyphen", input: "in-review", want: StatusInReview, wantErr: false},
		{name: "in_review underscore", input: "in_review", want: StatusInReview, wantErr: false},
		{name: "review lower", input: "review", want: StatusInReview, wantErr: false},
		{name: "REVIEW upper", input: "REVIEW", want: StatusInReview, wantErr: false},
		{name: "to verify space", input: "to verify", want: StatusInReview, wantErr: false},
		{name: "To Verify title", input: "To Verify", want: StatusInReview, wantErr: false},
		{name: "to_verify prompt separator", input: "to_verify", want: StatusInReview, wantErr: false},
		{name: "to-verify hyphen", input: "to-verify", want: StatusInReview, wantErr: false},
		{name: "verify lower", input: "verify", want: StatusInReview, wantErr: false},
		{name: "VERIFY upper", input: "VERIFY", want: StatusInReview, wantErr: false},

		// Done / complete / completed
		{name: "done lower", input: "done", want: StatusDone, wantErr: false},
		{name: "DONE upper with spaces", input: "  DONE ", want: StatusDone, wantErr: false},
		{name: "complete lower", input: "complete", want: StatusDone, wantErr: false},
		{name: "COMPLETE upper", input: "COMPLETE", want: StatusDone, wantErr: false},
		{name: "completed lower", input: "completed", want: StatusDone, wantErr: false},
		{name: "COMPLETED upper", input: "COMPLETED", want: StatusDone, wantErr: false},

		// Unknown / invalid
		{name: "unknown status", input: "unknown", want: "", wantErr: true},
		{name: "empty string", input: "", want: "", wantErr: true},
		{name: "whitespace only", input: "   ", want: "", wantErr: true},
		{name: "random invalid text", input: "invalid_state", want: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseStatus(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseStatus(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseStatus(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
