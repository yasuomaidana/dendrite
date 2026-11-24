package fsutil

import (
	"errors"
	"os"
	"path/filepath"
)

// AppendLine appends line plus a newline to path in one write and fsyncs it, so a line that was acknowledged
// survives a kill -9 of the writer. The parent directory is created if needed.
func AppendLine(path string, line []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	_, werr := f.Write(append(line[:len(line):len(line)], '\n'))
	return errors.Join(werr, f.Sync(), f.Close())
}
