package switcher

import (
	"fmt"
	"os"
)

// Transaction manages in-memory backups of files to ensure atomic updates with automatic rollback.
type Transaction struct {
	backups  map[string][]byte
	nonExist map[string]bool
	files    []string
}

// NewTransaction initializes a new empty transaction.
func NewTransaction() *Transaction {
	return &Transaction{
		backups:  make(map[string][]byte),
		nonExist: make(map[string]bool),
		files:    []string{},
	}
}

// Snapshot records the state of a file before any write operation.
func (tx *Transaction) Snapshot(path string) error {
	if _, recorded := tx.backups[path]; recorded {
		return nil
	}
	if tx.nonExist[path] {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			tx.nonExist[path] = true
			tx.files = append(tx.files, path)
			return nil
		}
		return fmt.Errorf("snapshotting %s: %w", path, err)
	}

	tx.backups[path] = data
	tx.files = append(tx.files, path)
	return nil
}

// Rollback restores all files to their pre-transaction states.
func (tx *Transaction) Rollback() error {
	var errs []error
	for path, data := range tx.backups {
		if err := os.WriteFile(path, data, 0644); err != nil {
			errs = append(errs, fmt.Errorf("restoring %s: %w", path, err))
		}
	}
	for path := range tx.nonExist {
		_ = os.Remove(path)
	}
	if len(errs) > 0 {
		return fmt.Errorf("rollback errors: %v", errs)
	}
	return nil
}

// Commit clears transaction backups upon successful completion.
func (tx *Transaction) Commit() {
	tx.backups = make(map[string][]byte)
	tx.nonExist = make(map[string]bool)
	tx.files = nil
}
