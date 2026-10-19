package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"sec/internal/dcf"
)

const usage = `usage: dcf [options]

Calculate a stored DCF model and save its valuation to PostgreSQL.

options:
  -model-id int   ID of the model in dcf_models (required)
  -dsn string     PostgreSQL connection string (default DATABASE_URL or libpq environment)

examples:
  dcf -model-id 42
  dcf -model-id 42 -dsn 'postgres://user@localhost/research?sslmode=disable'
`

func run() error {
	modelID := flag.Int64("model-id", 0, "ID of the model in dcf_models")
	dsn := flag.String("dsn", os.Getenv("DATABASE_URL"), "PostgreSQL connection string")
	flag.Usage = func() { fmt.Fprint(flag.CommandLine.Output(), usage) }
	flag.CommandLine.SetOutput(os.Stderr)
	flag.Parse()
	if *modelID <= 0 {
		return fmt.Errorf("provide a positive -model-id")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	model, err := dcf.LoadModelFromPostgres(ctx, *dsn, *modelID)
	if err != nil {
		return err
	}
	result, err := dcf.Calculate(model)
	if err != nil {
		return err
	}
	valuationID, err := dcf.SaveResultToPostgres(ctx, *dsn, *modelID, result)
	if err != nil {
		return err
	}
	fmt.Printf("valuation_id=%d model_id=%d enterprise_value=%s equity_value=%s", valuationID, *modelID, strconv.FormatFloat(result.EnterpriseValue, 'f', 2, 64), strconv.FormatFloat(result.EquityValue, 'f', 2, 64))
	if result.ValuePerShare != nil {
		fmt.Printf(" value_per_share=%s", strconv.FormatFloat(*result.ValuePerShare, 'f', 2, 64))
	}
	fmt.Println()
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
