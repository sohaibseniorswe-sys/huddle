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

// ChooseReopen tells the human that control issue number is closed while
// the issues in open are not, and asks what to do: "1) Reopen and
// continue" (true) or "2) Leave it and exit" (false). An empty answer
// means 1; end of input without an answer means 2. Any other answer asks
// again.
func ChooseReopen(in io.Reader, out io.Writer, number int, open []int) bool {
	nums := make([]string, len(open))
	for i, n := range open {
		nums[i] = fmt.Sprintf("#%d", n)
	}
	fmt.Fprintf(out, "Control issue #%d is closed but %d issues are still open (%s)\n", number, len(open), strings.Join(nums, ", "))
	fmt.Fprintln(out, "  1) Reopen and continue")
	fmt.Fprintln(out, "  2) Leave it and exit")
	r := bufio.NewReader(in)
	for {
		fmt.Fprint(out, "Choose [1/2] ")
		line, err := r.ReadString('\n')
		if err != nil && line == "" {
			fmt.Fprintln(out)
			return false
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "", "1", "reopen":
			return true
		case "2", "leave":
			return false
		}
	}
}
