package rpc

import "context"

// Profile is the account behind the access token.
type Profile struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Fullname string `json:"fullname"`
}

// Penpot speaks camelCase JSON when asked for application/json (kebab-case in transit). Team, Project and File carry the fields the triad glue reads; Penpot returns more.
type Team struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Project struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	TeamID string `json:"teamId"`
}

type File struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Revn      int            `json:"revn"`
	VersionID string         `json:"vern,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

// UpdateFileParams is the payload of update-file: Changes are Penpot change objects, applied on top of Revn.
type UpdateFileParams struct {
	ID        string           `json:"id"`
	SessionID string           `json:"sessionId"`
	Revn      int              `json:"revn"`
	Changes   []map[string]any `json:"changes"`
}

// GetProfile answers who the token belongs to (the cheapest proof that the token works).
func (c *Client) GetProfile(ctx context.Context) (Profile, error) {
	var p Profile
	return p, c.Call(ctx, "get-profile", nil, &p)
}

// GetTeams lists the teams the profile belongs to.
func (c *Client) GetTeams(ctx context.Context) ([]Team, error) {
	var t []Team
	return t, c.Call(ctx, "get-teams", nil, &t)
}

// GetProjects lists the projects of a team.
func (c *Client) GetProjects(ctx context.Context, teamID string) ([]Project, error) {
	var p []Project
	return p, c.Call(ctx, "get-projects", map[string]any{"teamId": teamID}, &p)
}

// GetFile reads one file, including its data.
func (c *Client) GetFile(ctx context.Context, id string) (File, error) {
	var f File
	return f, c.Call(ctx, "get-file", map[string]any{"id": id}, &f)
}

// UpdateFile applies changes to a file.
func (c *Client) UpdateFile(ctx context.Context, p UpdateFileParams) error {
	return c.Call(ctx, "update-file", p, nil)
}
