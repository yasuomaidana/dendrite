package mcpserver

import (
	"regexp"
)

var ticketNumRegex = regexp.MustCompile(`\[?(DEN-\d+)\]?`)
