package store

import (
	"errors"
	"time"
)

var (
	// ErrKeyNotFound is returned when a requested key does not exist.
	ErrKeyNotFound = errors.New("key not found in store")
	// ErrADRNotFound is returned when a requested ADR record is not found.
	ErrADRNotFound = errors.New("ADR record not found")
	// ErrInvalidADR is returned when an ADR record lacks required fields.
	ErrInvalidADR = errors.New("invalid ADR record: missing title or ticket_id")
)

// ADR represents an Architectural Decision Record capturing design and reuse provenance.
type ADR struct {
	ID           string    `json:"id" yaml:"id"`
	TicketID     string    `json:"ticket_id" yaml:"ticket_id"`
	Title        string    `json:"title" yaml:"title"`
	Status       string    `json:"status" yaml:"status"` // Proposed, Accepted, Superceded, Rejected
	Date         time.Time `json:"date" yaml:"date"`
	TierSelected string    `json:"tier_selected" yaml:"tier_selected"`
	PackageName  string    `json:"package_name" yaml:"package_name"`
	SourceURI    string    `json:"source_uri" yaml:"source_uri"`
	Rationale    string    `json:"rationale" yaml:"rationale"`
	Context      string    `json:"context" yaml:"context"`
	Evaluation   string    `json:"evaluation" yaml:"evaluation"`
	Consequences string    `json:"consequences" yaml:"consequences"`
	FilePath     string    `json:"file_path,omitempty" yaml:"-"`
}
