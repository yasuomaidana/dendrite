package memory

// Clip cuts s to at most n bytes on a character boundary and marks the cut; a string that fits is returned as is.
func Clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n] + "..."
}
