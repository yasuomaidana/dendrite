package gate

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Scope is what a change touched: the files, and the lines it added or changed in each.
type Scope struct {
	lines map[string][][2]int
}

// Files lists the touched files.
func (s *Scope) Files() []string {
	out := make([]string, 0, len(s.lines))
	for f := range s.lines {
		out = append(out, f)
	}
	return out
}

// HasFile reports whether the change touched path.
func (s *Scope) HasFile(path string) bool {
	_, ok := s.lines[path]
	return ok
}

// Touches reports whether the change added or changed a line of path within [start, end]. A file the change
// only deleted from or renamed counts as touched on every line.
func (s *Scope) Touches(path string, start, end int) bool {
	ranges, ok := s.lines[path]
	if !ok {
		return false
	}
	if len(ranges) == 0 {
		return true
	}
	for _, r := range ranges {
		if r[0] <= end && start <= r[1] {
			return true
		}
	}
	return false
}

var (
	diffHunk = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)
	diffName = regexp.MustCompile(`^diff --git a/.+ b/(.+)$`)
)

// addDiff adds the files and added lines of a `-U0` patch to the scope.
func (s *Scope) addDiff(patch string) {
	file := ""
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case diffName.MatchString(line):
			file = diffName.FindStringSubmatch(line)[1]
			if _, ok := s.lines[file]; !ok {
				s.lines[file] = nil
			}
		case file != "" && diffHunk.MatchString(line):
			m := diffHunk.FindStringSubmatch(line)
			start, _ := strconv.Atoi(m[1])
			count := 1
			if m[2] != "" {
				count, _ = strconv.Atoi(m[2])
			}
			if count > 0 {
				s.lines[file] = append(s.lines[file], [2]int{start, start + count - 1})
			}
		}
	}
}

func git(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	return string(out), err
}

// ChangesSince is what the working copy at dir changed since ref: committed changes since it branched from ref,
// and uncommitted ones.
func ChangesSince(dir, ref string) *Scope {
	s := &Scope{lines: map[string][][2]int{}}
	if patch, err := git(dir, "diff", "-U0", "--no-color", ref+"...HEAD"); err == nil {
		s.addDiff(patch)
	}
	if patch, err := git(dir, "diff", "-U0", "--no-color", "HEAD"); err == nil {
		s.addDiff(patch)
	}
	return s
}

// ChangesOfTicket is the work done for a ticket: what its branch changed against develop and what is uncommitted,
// and, once the branch is merged, the commits whose subject starts with [TICKET].
func ChangesOfTicket(dir, ticket string) *Scope {
	var s *Scope
	for _, base := range []string{"origin/develop", "develop"} {
		if s = ChangesSince(dir, base); len(s.lines) > 0 {
			break
		}
	}
	shas, _ := TicketCommits(dir, ticket)
	if len(shas) == 0 {
		return s
	}
	// The line numbers of an old commit's patch are those of that commit's files, and later commits shift
	// them. Everything is expressed in HEAD's numbering instead: the diff from before the ticket's first commit
	// to HEAD, limited to the files its commits touched.
	files := map[string]bool{}
	for _, sha := range shas {
		if names, err := git(dir, "show", "--name-only", "--format=", sha); err == nil {
			for _, f := range strings.Fields(names) {
				files[f] = true
			}
		}
	}
	args := []string{"diff", "-U0", "--no-color", shas[0] + "^", "HEAD", "--"}
	for f := range files {
		args = append(args, f)
	}
	if patch, err := git(dir, args...); err == nil {
		s.addDiff(patch)
	} else { // the first commit has no parent
		patch, _ := git(dir, "show", "-U0", "--no-color", "--format=", "--patch", shas[0])
		s.addDiff(patch)
	}
	return s
}

// TicketCommits lists, oldest first, the commits whose subject starts with [TICKET]: their hashes and
// "short-hash subject" lines.
func TicketCommits(dir, ticket string) (shas, lines []string) {
	out, err := git(dir, "log", "--reverse", "--no-merges", "--format=%H %s", "--fixed-strings", "--grep=["+ticket+"]")
	if err != nil {
		return nil, nil
	}
	for _, l := range strings.Split(out, "\n") {
		sha, subject, ok := strings.Cut(strings.TrimSpace(l), " ")
		if ok && strings.HasPrefix(subject, "["+ticket+"]") {
			shas, lines = append(shas, sha), append(lines, sha[:min(7, len(sha))]+" "+subject)
		}
	}
	return shas, lines
}
