package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"sec/internal/sec"
)

const usage = `usage: sec -ticker SYMBOL [options]

Fetch SEC XBRL facts used to prepare a DCF model and save them to SQLite.

options:
  -ticker string   company ticker symbol, e.g. AAPL (required)
  -db string       SQLite database file (default SEC_DATABASE_PATH or sec.db)
  -ua string       SEC User-Agent including contact email (default SEC_USER_AGENT)
`

func run() error {
	ticker := flag.String("ticker", "", "company ticker symbol, e.g. AAPL")
	database := flag.String("db", envOr("SEC_DATABASE_PATH", "sec.db"), "SQLite database file")
	userAgent := flag.String("ua", os.Getenv("SEC_USER_AGENT"), "SEC User-Agent including contact email")
	flag.Usage = func() { fmt.Fprint(flag.CommandLine.Output(), usage) }
	flag.CommandLine.SetOutput(os.Stderr)
	flag.Parse()
	if *ticker == "" {
		return errors.New("provide -ticker")
	}
	if *userAgent == "" {
		return errors.New("provide -ua 'Research App contact@example.com' or set SEC_USER_AGENT")
	}
	if flag.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	client := sec.NewClient(*userAgent)
	cik, err := client.ResolveTicker(ctx, *ticker)
	if err != nil {
		return err
	}
	factsData, err := client.Facts(ctx, cik)
	if err != nil {
		return fmt.Errorf("fetch SEC company facts: %w", err)
	}
	facts := sec.ExtractFacts(factsData, cik)
	if err := sec.SaveFacts(ctx, *database, *ticker, facts); err != nil {
		return err
	}
	fmt.Printf("saved_dcf_facts=%d ticker=%s cik=%s database=%s\n", len(facts), *ticker, cik, *database)
	return nil
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
