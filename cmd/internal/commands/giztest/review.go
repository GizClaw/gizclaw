package giztest

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

func runReview(in io.Reader, out io.Writer, message string) error {
	if f, ok := in.(*os.File); ok {
		info, err := f.Stat()
		if err != nil || info.Mode()&os.ModeCharDevice == 0 {
			return fmt.Errorf("review requires an attached terminal")
		}
	}
	if message == "" {
		message = "Confirm observed behavior"
	}
	fmt.Fprintf(out, "%s [pass/fail]: ", message)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(line), "pass") {
		return nil
	}
	return fmt.Errorf("review rejected")
}
