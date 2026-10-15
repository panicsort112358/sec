package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"
)

func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func run() error {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s -cik CIK [options]\n\n", os.Args[0])
		fmt.Fprintln(flag.CommandLine.Output(), "Read SEC company filings, filing documents, or XBRL facts.")
		fmt.Fprintln(flag.CommandLine.Output(), "Set SEC_USER_AGENT or pass -ua with a descriptive agent and contact email.\n")
		fmt.Fprintln(flag.CommandLine.Output(), "Examples:")
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -cik 320193 -mode filings -form 10-K\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -cik 320193 -mode document -form 10-K\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -cik 320193 -mode document -accession 0000320193-24-000123\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -cik 320193 -mode facts -form 10-K\n\n", os.Args[0])
		fmt.Fprintln(flag.CommandLine.Output(), "Options:")
		flag.PrintDefaults()
	}
	flag.CommandLine.SetOutput(os.Stderr)
	flag.CommandLine.Usage = flag.Usage
	cikFlag := flag.String(
		"cik", "", "SEC Central Index Key, e.g. 320193",
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

	if *cikFlag == "" {
		flag.Usage()
		return errors.New("provide -cik")
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

	cik, err := normalizeCIK(*cikFlag)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 90*time.Second,
	)
	defer cancel()

	client := NewClient(*agent)

	if *mode == "filings" || *mode == "document" {
		data, err := client.Submissions(ctx, cik)
		if err != nil {
			return err
		}
		if *mode == "filings" {
			return writeJSON(ExtractFilings(data, cik, *form, *limit))
		}
		filings := ExtractFilings(data, cik, *form, 0)
		var selected *Filing
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
	return writeJSON(ExtractFacts(
		data, cik, *form, *limit,
	))
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
