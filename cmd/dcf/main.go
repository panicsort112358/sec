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
	"sec/internal/sqlite"
)

const usage = `usage: dcf -ticker SYMBOL [options]

Calculate stored DCF models for one ticker and append their valuations to the
SQLite database, which contains SEC data, dcf_models and forecast rows.

options:
  -ticker string   company ticker symbol (required)
  -db string       SQLite database file (default SEC_DATABASE_PATH or sec.db)
`

func run() error {
	ticker := flag.String("ticker", "", "company ticker symbol")
	database := flag.String("db", sqlite.DefaultPath(), "SQLite database file")
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
	models, err := dcf.LoadModels(ctx, *database, *ticker)
	if err != nil {
		return err
	}
	if len(models) == 0 {
		return fmt.Errorf("no DCF models found for ticker %s", *ticker)
	}

	var valuations []dcf.Valuation
	var failures []error
	for _, model := range models {
		valuation, err := calculateModel(model)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		valuations = append(valuations, valuation)
	}

	saved, err := dcf.SaveResults(ctx, *database, valuations)
	if err != nil {
		failures = append(failures, err)
		return errors.Join(failures...)
	}
	for i, valuation := range valuations {
		printResult(saved[i].ValuationID, valuation.ModelID, *ticker, valuation.Result)
	}
	return errors.Join(failures...)
}

func calculateModel(model dcf.StoredModel) (dcf.Valuation, error) {
	result, err := dcf.Calculate(model.Model)
	if err != nil {
		return dcf.Valuation{}, fmt.Errorf("model %d: %w", model.ID, err)
	}
	return dcf.Valuation{ModelID: model.ID, Result: result}, nil
}

func printResult(valuationID, modelID int64, ticker string, result dcf.Result) {
	fmt.Printf(
		"valuation_id=%d model_id=%d ticker=%s enterprise_value=%s equity_value=%s",
		valuationID, modelID, ticker,
		money(result.EnterpriseValue), money(result.EquityValue),
	)
	if result.ValuePerShare != nil {
		fmt.Printf(" value_per_share=%s", money(*result.ValuePerShare))
	}
	fmt.Println()
}

func money(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
