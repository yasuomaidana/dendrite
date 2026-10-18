package agentjobs

import "Dendrite/pkg/workcopy"

// settledStatuses are job states whose worktree no longer belongs to the job.
var settledStatuses = map[Status]bool{
	StatusCommitted: true,
	StatusCancelled: true,
	StatusFailed:    true,
}

// WorktreeInUse returns a predicate reporting whether a worktree path is still
// used by a job that has not settled, so working-copy cleanup never removes it.
// When the ledger cannot be read every path is reported as in use.
func (l *Ledger) WorktreeInUse() func(path string) bool {
	jobs, err := l.List()
	if err != nil {
		return func(string) bool { return true }
	}
	active := make(map[string]bool)
	for _, job := range jobs {
		if job.Worktree != "" && !settledStatuses[job.Status] {
			active[workcopy.Canon(job.Worktree)] = true
		}
	}
	return func(path string) bool { return active[workcopy.Canon(path)] }
}
