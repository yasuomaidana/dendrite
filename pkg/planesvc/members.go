package planesvc

import (
	"context"

	"Dendrite/pkg/tracker/plane"
)

// User is the Plane account the API key belongs to.
type User struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

// Comment is one comment on an issue; CommentHTML is the stored HTML.
type Comment struct {
	ID          string `json:"id"`
	CommentHTML string `json:"comment_html"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// Me returns the user the API key belongs to: whom a claim assigns tickets to.
func (s *Service) Me(ctx context.Context, c *plane.Client) (*User, error) {
	var out struct {
		User User `json:"user"`
	}
	if err := s.call(ctx, false, "user.me", target(c), &out); err != nil {
		return nil, err
	}
	return &out.User, nil
}

// PatchLabels keeps the labels of ref, drops those whose name starts with any of removePrefixes and adds
// the names in add (created in the project when missing). Applying the same call twice leaves the
// same labels, so a re-claim replaces `agent:` and `tier:` labels instead of piling them up. It
// returns the updated issue and the final label names.
func (s *Service) PatchLabels(ctx context.Context, c *plane.Client, ref string, add, removePrefixes []string) (*plane.PlaneIssue, []string, error) {
	params := target(c)
	params["ref"] = ref
	params["add"] = nonNil(add)
	params["remove_prefixes"] = nonNil(removePrefixes)
	var out struct {
		Issue  plane.PlaneIssue `json:"issue"`
		Labels []string         `json:"labels"`
	}
	if err := s.call(ctx, false, "issue.labels_patch", params, &out); err != nil {
		return nil, nil, err
	}
	return &out.Issue, out.Labels, nil
}

// Comments lists the comments of ref, oldest first.
func (s *Service) Comments(ctx context.Context, c *plane.Client, ref string) ([]Comment, error) {
	params := target(c)
	params["ref"] = ref
	var out struct {
		Comments []Comment `json:"comments"`
	}
	if err := s.call(ctx, false, "issue.comments", params, &out); err != nil {
		return nil, err
	}
	return out.Comments, nil
}

// UpdateComment replaces the HTML of one comment of ref; the HTML is stored as given.
func (s *Service) UpdateComment(ctx context.Context, c *plane.Client, ref, commentID, commentHTML string) error {
	params := target(c)
	params["ref"] = ref
	params["comment_id"] = commentID
	params["comment_html"] = commentHTML
	return s.call(ctx, false, "issue.comment_update", params, nil)
}

// nonNil makes a nil list an empty one, so it is sent as [] and not as null.
func nonNil(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}
