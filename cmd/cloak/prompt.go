package main

import (
	"bufio"
	"fmt"
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

// readLine prompts for a single (echoed) line on stderr and returns the trimmed result.
func readLine(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
