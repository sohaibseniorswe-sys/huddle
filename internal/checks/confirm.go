package checks

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Confirm asks "Continue? [Y/n]" and reports whether the answer is yes.
// An empty answer means yes; end of input without an answer means no.
func Confirm(in io.Reader, out io.Writer) bool {
	fmt.Fprint(out, "Continue? [Y/n] ")
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(out)
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "", "y", "yes":
		return true
	}
	return false
}
