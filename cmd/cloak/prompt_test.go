package main

import (
	"io"
	"strings"
	"testing"
)

// TestReadLineNoReadAhead is the regression test for the prompt desync bug:
// sequential readLine calls must each return exactly one line even when the
// whole input is already available (a pipe, a paste, a here-doc). A buffered
// reader constructed per call would swallow the lines after the first.
func TestReadLineNoReadAhead(t *testing.T) {
	orig := promptIn
	defer func() { promptIn = orig }()
	promptIn = strings.NewReader("first\nsecond\nthird\n")

	for i, want := range []string{"first", "second", "third"} {
		got, err := readLine("")
		if err != nil {
			t.Fatalf("readLine[%d]: %v", i, err)
		}
		if got != want {
			t.Fatalf("readLine[%d] = %q, want %q", i, got, want)
		}
	}
}

// TestReadLineCRLF verifies a trailing carriage return is trimmed.
func TestReadLineCRLF(t *testing.T) {
	orig := promptIn
	defer func() { promptIn = orig }()
	promptIn = strings.NewReader("value\r\nnext\r\n")

	got, err := readLine("")
	if err != nil {
		t.Fatal(err)
	}
	if got != "value" {
		t.Fatalf("got %q, want %q", got, "value")
	}
}

// TestReadLineEOF verifies behaviour at end of input: a final unterminated
// line is returned with io.EOF, and a subsequent read yields the empty string.
func TestReadLineEOF(t *testing.T) {
	orig := promptIn
	defer func() { promptIn = orig }()
	promptIn = strings.NewReader("only")

	got, err := readLine("")
	if got != "only" || err != io.EOF {
		t.Fatalf("got (%q, %v), want (%q, EOF)", got, err, "only")
	}
	got, _ = readLine("")
	if got != "" {
		t.Fatalf("after EOF got %q, want empty", got)
	}
}

// TestReadMultiline verifies the "." terminator and that the line right after
// it is left intact for the next reader.
func TestReadMultiline(t *testing.T) {
	orig := promptIn
	defer func() { promptIn = orig }()
	promptIn = strings.NewReader("line one\nline two\n.\nafter\n")

	body, err := readMultiline()
	if err != nil {
		t.Fatal(err)
	}
	if body != "line one\nline two\n" {
		t.Fatalf("body = %q", body)
	}
	rest, err := readLine("")
	if err != nil {
		t.Fatalf("readLine after multiline: %v", err)
	}
	if rest != "after" {
		t.Fatalf("line after terminator = %q, want %q", rest, "after")
	}
}
