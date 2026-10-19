// Package sqlite runs SQL scripts through the SQLite command-line client.
package sqlite

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func Run(ctx context.Context, path, script string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("provide a SQLite database path")
	}
	command := exec.CommandContext(ctx, "sqlite3", "-batch", "-bail", "-csv", path)
	command.Stdin = strings.NewReader("PRAGMA foreign_keys = ON;\n" + script)
	var output, diagnostics bytes.Buffer
	command.Stdout = &output
	command.Stderr = &diagnostics
	if err := command.Run(); err != nil {
		if _, ok := err.(*exec.Error); ok {
			return nil, fmt.Errorf("sqlite3 is required on PATH: %w", err)
		}
		return nil, fmt.Errorf("sqlite3 failed: %s: %w", strings.TrimSpace(diagnostics.String()), err)
	}
	return output.Bytes(), nil
}
