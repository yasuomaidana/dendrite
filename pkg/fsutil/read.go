package fsutil

import "os"

// ReadIfPresent reads a file; a missing file is not an error, it reports present as false.
func ReadIfPresent(path string) (data []byte, present bool, err error) {
	data, err = os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return data, err == nil, err
}
