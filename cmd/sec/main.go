package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"sec/internal/sec"
)

const usage = `usage: sec [options]

modes:
  filings     list company filings
  document    print a filing document as text
  facts       list extracted XBRL facts

options:
  -ticker string       company ticker symbol, e.g. AAPL
  -mode string         filings, document, or facts (default "filings")
  -accession string    accession number for document mode
  -form string         optional form filter, e.g. 10-K or 10-Q
  -limit int           maximum output records; 0 means unlimited (default 20)
  -ua string           descriptive User-Agent including contact email (default SEC_USER_AGENT)

examples:
  sec -ticker AAPL -mode filings -form 10-K
  sec -ticker AAPL -mode document -form 10-K
  sec -ticker AAPL -mode document -accession 0000320193-24-000123
  sec -ticker AAPL -mode facts -form 10-K
`

func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func run() error {
	flag.Usage = func() {
		fmt.Fprint(flag.CommandLine.Output(), usage)
		fmt.Fprintln(flag.CommandLine.Output(), "Set SEC_USER_AGENT to a descriptive agent with contact email, or pass -ua.")
	}
	flag.CommandLine.SetOutput(os.Stderr)
	flag.CommandLine.Usage = flag.Usage
	tickerFlag := flag.String(
		"ticker", "", "company ticker symbol, e.g. AAPL",
	)
	mode := flag.String(
		"mode", "filings", "filings, document, or facts",
	)
	accession := flag.String("accession", "", "accession number for document mode")
	form := flag.String(
		"form", "", "optional form filter, e.g. 10-K or 10-Q",
	)
	limit := flag.Int(
		"limit", 20, "maximum output records; 0 means unlimited",
	)
	agent := flag.String(
		"ua", os.Getenv("SEC_USER_AGENT"),
		"descriptive User-Agent including contact email",
	)
	flag.Parse()

	if *tickerFlag == "" {
		flag.Usage()
		return errors.New("provide -ticker")
	}
	if *agent == "" {
		return errors.New(
			"provide -ua 'Research App contact@example.com' " +
				"or set SEC_USER_AGENT",
		)
	}
	if *limit < 0 {
		return errors.New("-limit cannot be negative")
	}
	if *mode != "filings" && *mode != "document" && *mode != "facts" {
		return errors.New("-mode must be filings, document, or facts")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 90*time.Second,
	)
	defer cancel()

	client := sec.NewClient(*agent)
	cik, err := client.ResolveTicker(ctx, *tickerFlag)
	if err != nil {
		return err
	}

	if *mode == "filings" || *mode == "document" {
		data, err := client.Submissions(ctx, cik)
		if err != nil {
			return err
		}
		if *mode == "filings" {
			return writeJSON(sec.ExtractFilings(data, cik, *form, *limit))
		}
		filings := sec.ExtractFilings(data, cik, *form, 0)
		var selected *sec.Filing
		for i := range filings {
			if *accession == "" || filings[i].AccessionNumber == *accession {
				selected = &filings[i]
				break
			}
		}
		if selected == nil {
			return fmt.Errorf("no matching filing found")
		}
		document, err := client.Document(ctx, selected.DocumentURL)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(os.Stdout, document)
		return err
	}

	data, err := client.Facts(ctx, cik)
	if err != nil {
		return err
	}
	return writeJSON(sec.ExtractFacts(
		data, cik, *form, *limit,
	))
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
