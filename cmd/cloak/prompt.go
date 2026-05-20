package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// readPassword prompts for a password from /dev/tty (or stdin if not a TTY).
// If --from-stdin was given, the first line of stdin is returned instead.
func readPassword(prompt string, fromStdin bool) (string, error) {
	if fromStdin {
		s := bufio.NewScanner(os.Stdin)
		if !s.Scan() {
			if err := s.Err(); err != nil {
				return "", err
			}
			return "", fmt.Errorf("no password on stdin")
		}
		return s.Text(), nil
	}
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", fmt.Errorf("stdin is not a terminal; use --from-stdin")
	}
	fmt.Fprint(os.Stderr, prompt)
	bytePassword, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(bytePassword), nil
}

// readLine prompts for a single (echoed) line on stderr and returns the trimmed
// result.
//
// It reads byte-by-byte directly from stdin and never buffers past the newline.
// That matters because interactive forms interleave readLine with readPassword
// (which reads byte-by-byte too, via golang.org/x/term). A buffered reader here
// would consume — and discard — lines destined for the *next* prompt whenever
// input arrives faster than it is read: a pipe, a paste, or a here-doc. With no
// read-ahead the two functions interleave correctly regardless of input speed.
func readLine(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	return readRawLine()
}

// promptIn is the reader readRawLine consumes. It is os.Stdin in production
// and is overridden by tests.
var promptIn io.Reader = os.Stdin

// readRawLine reads one line from promptIn one byte at a time, stopping after
// '\n'. A trailing '\r' is trimmed. On EOF it returns whatever was read so far
// along with io.EOF.
func readRawLine() (string, error) {
	var b []byte
	buf := make([]byte, 1)
	for {
		n, err := promptIn.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				return strings.TrimRight(string(b), "\r"), nil
			}
			b = append(b, buf[0])
		}
		if err != nil {
			if err == io.EOF {
				return strings.TrimRight(string(b), "\r"), io.EOF
			}
			return strings.TrimRight(string(b), "\r"), err
		}
	}
}

// readMultiline reads lines from stdin until a line containing only "." and
// returns them joined with newlines (with a trailing newline). Used for
// multi-line file templates.
func readMultiline() (string, error) {
	var b strings.Builder
	for {
		line, err := readRawLine()
		if line == "." {
			break
		}
		b.WriteString(line)
		b.WriteString("\n")
		if err != nil {
			break
		}
	}
	return b.String(), nil
}
