
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

const secBase = "https://data.sec.gov"

type Client struct {
	http      *http.Client
	userAgent string
}

func NewClient(userAgent string) *Client {
	return &Client{
		http: &http.Client{
			Timeout: 20 * time.Second,
		},
		userAgent: userAgent,
	}
}

func (c *Client) Get(
	ctx context.Context,
	url string,
	dst any,
) error {
	var lastErr error

	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<uint(attempt-1)) *
				500 * time.Millisecond

			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}

		req, err := http.NewRequestWithContext(
			ctx, http.MethodGet, url, nil,
		)
		if err != nil {
			return err
		}

		req.Header.Set("User-Agent", c.userAgent)
		req.Header.Set("Accept", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, readErr := io.ReadAll(
			io.LimitReader(resp.Body, 64<<20),
		)
		resp.Body.Close()

		if readErr != nil {
			lastErr = readErr
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode >= 500 {
			lastErr = fmt.Errorf(
				"SEC returned HTTP %d", resp.StatusCode,
			)
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf(
				"SEC returned HTTP %d: %s",
				resp.StatusCode, strings.TrimSpace(string(body)),
			)
		}

		if err := json.Unmarshal(body, dst); err != nil {
			return fmt.Errorf("decode SEC response: %w", err)
		}

		return nil
	}

	return fmt.Errorf("request failed after retries: %w", lastErr)
}

type Submissions struct {
	Name     string `json:"name"`
	CIK      string  `json:"cik"`
	Tickers  []string `json:"tickers"`
	Filings  struct {
		Recent RecentFilings `json:"recent"`
	} `json:"filings"`
}

type RecentFilings struct {
	AccessionNumber []string `json:"accessionNumber"`
	Form            []string `json:"form"`
	FilingDate      []string `json:"filingDate"`
	ReportDate      []string `json:"reportDate"`
	PrimaryDocument []string `json:"primaryDocument"`
}

type Filing struct {
	Company         string `json:"company"`
	CIK             string `json:"cik"`
	Form            string `json:"form"`
	FilingDate      string `json:"filing_date"`
	ReportDate      string `json:"report_date,omitempty"`
	AccessionNumber string `json:"accession_number"`
	DocumentURL     string `json:"document_url"`
}

type FactEntry struct {
	Accession string  `json:"accn"`
	End       string  `json:"end"`
	Start     string  `json:"start,omitempty"`
	Form      string  `json:"form"`
	Filed     string  `json:"filed"`
	FY        *int    `json:"fy,omitempty"`
	FP        string  `json:"fp,omitempty"`
	Frame     string  `json:"frame,omitempty"`
	Value     float64 `json:"val"`
}

type CompanyFacts struct {
	CIK    int    `json:"cik"`
	Entity string `json:"entityName"`
	Facts  map[string]map[string]struct {
		Label       string `json:"label"`
		Description string `json:"description"`
		Units       map[string][]FactEntry `json:"units"`
	} `json:"facts"`
}

type FinancialFact struct {
	Company   string  `json:"company"`
	CIK       string  `json:"cik"`
	Concept   string  `json:"concept"`
	Label     string  `json:"label"`
	Unit      string  `json:"unit"`
	Value     float64 `json:"value"`
	Start     string  `json:"period_start,omitempty"`
	End       string  `json:"period_end"`
	FilingDate string `json:"filing_date"`
	Form      string  `json:"form"`
	Accession string  `json:"accession_number"`
	Frame     string  `json:"frame,omitempty"`
}

var validCIK = regexp.MustCompile(`^[0-9]{1,10}$`)

func normalizeCIK(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !validCIK.MatchString(raw) {
		return "", fmt.Errorf("invalid CIK %q", raw)
	}
	return strings.Repeat("0", 10-len(raw)) + raw, nil
}

func (c *Client) Submissions(
	ctx context.Context, cik string,
) (*Submissions, error) {
	var result Submissions
	url := secBase + "/submissions/CIK" + cik + ".json"

	if err := c.Get(ctx, url, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) Facts(
	ctx context.Context, cik string,
) (*CompanyFacts, error) {
	var result CompanyFacts
	url := secBase + "/api/xbrl/companyfacts/CIK" +
		cik + ".json"

	if err := c.Get(ctx, url, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func ExtractFilings(
	s *Submissions,
	cik, form string,
	limit int,
) []Filing {
	r := s.Filings.Recent
	var out []Filing

	for i, f := range r.Form {
		if form != "" && f != form {
			continue
		}
		if i >= len(r.AccessionNumber) ||
			i >= len(r.FilingDate) ||
			i >= len(r.PrimaryDocument) {
			continue
		}

		acc := r.AccessionNumber[i]
		pathAcc := strings.ReplaceAll(acc, "-", "")
		doc := r.PrimaryDocument[i]

		if doc == "" || strings.Contains(doc, "/") ||
			strings.Contains(doc, "\\") {
			continue
		}

		reportDate := ""
		if i < len(r.ReportDate) {
			reportDate = r.ReportDate[i]
		}

		out = append(out, Filing{
			Company:         s.Name,
			CIK:             cik,
			Form:            f,
			FilingDate:      r.FilingDate[i],
			ReportDate:      reportDate,
			AccessionNumber: acc,
			DocumentURL: fmt.Sprintf(
				"https://www.sec.gov/Archives/edgar/data/%s/%s/%s",
				strings.TrimLeft(cik, "0"), pathAcc, doc,
			),
		})

		if limit > 0 && len(out) >= limit {
			break
		}
	}

	return out
}

var concepts = []string{
	"RevenueFromContractWithCustomerExcludingAssessedTax",
	"Revenues",
	"SalesRevenueNet",
	"NetIncomeLoss",
	"Assets",
	"Liabilities",
	"StockholdersEquity",
	"OperatingIncomeLoss",
	"NetCashProvidedByUsedInOperatingActivities",
	"CashAndCashEquivalentsAtCarryingValue",
}

func ExtractFacts(
	data *CompanyFacts,
	cik string,
	form string,
	limit int,
) []FinancialFact {
	var out []FinancialFact

	usgaap, ok := data.Facts["us-gaap"]
	if !ok {
		return out
	}

	for _, concept := range concepts {
		def, ok := usgaap[concept]
		if !ok {
			continue
		}

		for unit, entries := range def.Units {
			for _, e := range entries {
				if form != "" && e.Form != form {
					continue
				}

				out = append(out, FinancialFact{
					Company:    data.Entity,
					CIK:        cik,
					Concept:    concept,
					Label:      def.Label,
					Unit:       unit,
					Value:      e.Value,
					Start:      e.Start,
					End:        e.End,
					FilingDate: e.Filed,
					Form:       e.Form,
					Accession:  e.Accession,
					Frame:      e.Frame,
				})
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].End != out[j].End {
			return out[i].End > out[j].End
		}
		if out[i].Concept != out[j].Concept {
			return out[i].Concept < out[j].Concept
		}
		return out[i].FilingDate > out[j].FilingDate
	})

	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}

	return out
}

func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func run() error {
	cikFlag := flag.String(
		"cik", "", "SEC Central Index Key, e.g. 320193",
	)
	mode := flag.String(
		"mode", "filings", "filings or facts",
	)
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
	if *mode != "filings" && *mode != "facts" {
		return errors.New("-mode must be filings or facts")
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

	if *mode == "filings" {
		data, err := client.Submissions(ctx, cik)
		if err != nil {
			return err
		}
		return writeJSON(ExtractFilings(
			data, cik, *form, *limit,
		))
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

