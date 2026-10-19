// Package psql runs SQL scripts through the PostgreSQL command-line client.
// Using the client keeps this repository free of third-party Go modules.
package psql

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func Run(ctx context.Context, dsn, script string) ([]byte, error) {
	args := []string{"-X", "--quiet", "--no-align", "--tuples-only", "--csv", "-v", "ON_ERROR_STOP=1"}
	if strings.TrimSpace(dsn) != "" {
		args = append(args, "--dbname", dsn)
	}
	command := exec.CommandContext(ctx, "psql", args...)
	command.Stdin = strings.NewReader(script)
	var output, diagnostics bytes.Buffer
	command.Stdout = &output
	command.Stderr = &diagnostics
	if err := command.Run(); err != nil {
		if _, ok := err.(*exec.Error); ok {
			return nil, fmt.Errorf("psql is required on PATH: %w", err)
		}
		return nil, fmt.Errorf("psql failed: %s: %w", strings.TrimSpace(diagnostics.String()), err)
	}
	return output.Bytes(), nil
}
