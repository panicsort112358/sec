package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"sec/internal/dcf"
)

const usage = `usage: dcf --ticker SYMBOL [options]

Calculate stored DCF models for one ticker and append their valuations to the
SQLite database, which contains SEC data, dcf_models and forecast rows.

options:
  -ticker string   company ticker symbol (required)
  -db string       SQLite database file (default SEC_DATABASE_PATH or sec.db)
`

func run() error {
	ticker := flag.String("ticker", "", "company ticker symbol")
	database := flag.String("db", envOr("SEC_DATABASE_PATH", "sec.db"), "SQLite database file")
	flag.Usage = func() { fmt.Fprint(flag.CommandLine.Output(), usage) }
	flag.CommandLine.SetOutput(os.Stderr)
	flag.Parse()
	if *ticker == "" {
		flag.Usage()
		return errors.New("provide -ticker")
	}
	if flag.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	modelIDs, err := dcf.ModelIDs(ctx, *database, *ticker)
	if err != nil {
		return err
	}
	if len(modelIDs) == 0 {
		return fmt.Errorf("no DCF models found for ticker %s", *ticker)
	}

	var failures []error
	for _, modelID := range modelIDs {
		model, err := dcf.LoadModelFromSQLite(ctx, *database, modelID)
		if err != nil {
			failures = append(failures, fmt.Errorf("model %d: %w", modelID, err))
			continue
		}
		result, err := dcf.Calculate(model)
		if err != nil {
			failures = append(failures, fmt.Errorf("model %d: %w", modelID, err))
			continue
		}
		valuationID, err := dcf.SaveResultToSQLite(ctx, *database, modelID, result)
		if err != nil {
			failures = append(failures, fmt.Errorf("model %d: %w", modelID, err))
			continue
		}
		fmt.Printf("valuation_id=%d model_id=%d ticker=%s enterprise_value=%s equity_value=%s", valuationID, modelID, *ticker, strconv.FormatFloat(result.EnterpriseValue, 'f', 2, 64), strconv.FormatFloat(result.EquityValue, 'f', 2, 64))
		if result.ValuePerShare != nil {
			fmt.Printf(" value_per_share=%s", strconv.FormatFloat(*result.ValuePerShare, 'f', 2, 64))
		}
		fmt.Println()
	}
	return errors.Join(failures...)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
