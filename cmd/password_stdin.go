package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

const maxPasswordBytes = 64 * 1024

// readPasswordFromStdin reads a single password line from r.
//
// Interactive terminals are rejected without reading so CLI invocations never
// block waiting for input. At most maxPasswordBytes+3 bytes (maxPasswordBytes
// plus a 2-byte CRLF terminator plus 1 overflow byte) are read to guard against
// unbounded streams. Multi-line input is rejected, then at most one trailing
// "\r\n" or "\n" line terminator is stripped, and the resulting password length
// is bounded by maxPasswordBytes. All other bytes (including leading/trailing
// spaces, tabs, quotes, backslashes, punctuation, Unicode, and any other "\r")
// are preserved verbatim. Error messages never include any password bytes.
func readPasswordFromStdin(r io.Reader, isTerminal func(io.Reader) bool) (string, error) {
	if isTerminal != nil && isTerminal(r) {
		return "", errors.New("--password-stdin requires piped or redirected standard input; interactive terminal input is not supported")
	}

	if r == nil {
		return "", errors.New("password read from standard input is empty")
	}

	raw, err := io.ReadAll(io.LimitReader(r, maxPasswordBytes+3))
	if err != nil {
		return "", fmt.Errorf("failed to read password from standard input: %w", err)
	}

	if idx := bytes.IndexByte(raw, '\n'); idx != -1 && idx < len(raw)-1 {
		return "", errors.New("--password-stdin accepts only a single password line")
	}

	if bytes.HasSuffix(raw, []byte("\r\n")) {
		raw = raw[:len(raw)-2]
	} else if bytes.HasSuffix(raw, []byte("\n")) {
		raw = raw[:len(raw)-1]
	}

	if len(raw) > maxPasswordBytes {
		return "", fmt.Errorf("password read from standard input exceeds maximum length of %d bytes", maxPasswordBytes)
	}

	if bytes.IndexByte(raw, 0) != -1 {
		return "", errors.New("password read from standard input must not contain NUL bytes")
	}

	if len(raw) == 0 {
		return "", errors.New("password read from standard input is empty")
	}

	return string(raw), nil
}
