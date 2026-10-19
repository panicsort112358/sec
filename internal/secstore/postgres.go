// Package secstore persists normalized SEC results in PostgreSQL through psql.
package secstore

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"sec/internal/psql"
	"sec/internal/sec"
)

func SaveFilings(ctx context.Context, dsn, ticker string, filings []sec.Filing) error {
	if len(filings) == 0 {
		return nil
	}
	var script strings.Builder
	script.WriteString("BEGIN;\n")
	writeCompany(&script, filings[0].CIK, filings[0].Company, ticker)
	for _, filing := range filings {
		fmt.Fprintf(&script, `INSERT INTO sec_filings (
    cik, accession_number, company_name, form, filing_date, report_date, document_url
) VALUES (%s, %s, %s, %s, %s, %s, %s)
ON CONFLICT (cik, accession_number) DO UPDATE SET
    company_name = EXCLUDED.company_name,
    form = EXCLUDED.form,
    filing_date = EXCLUDED.filing_date,
    report_date = EXCLUDED.report_date,
    document_url = EXCLUDED.document_url;
`, quote(filing.CIK), quote(filing.AccessionNumber), quote(filing.Company), quote(filing.Form), quote(filing.FilingDate), nullableText(filing.ReportDate), quote(filing.DocumentURL))
	}
	script.WriteString("COMMIT;\n")
	if _, err := psql.Run(ctx, dsn, script.String()); err != nil {
		return fmt.Errorf("save SEC filings: %w", err)
	}
	return nil
}

func SaveFacts(ctx context.Context, dsn, ticker string, facts []sec.FinancialFact) error {
	if len(facts) == 0 {
		return nil
	}
	var script strings.Builder
	script.WriteString("BEGIN;\n")
	writeCompany(&script, facts[0].CIK, facts[0].Company, ticker)
	for _, fact := range facts {
		fmt.Fprintf(&script, `INSERT INTO sec_facts (
    cik, company_name, ticker, concept, label, unit, value,
    period_start, period_end, filing_date, form, accession_number, frame
) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
ON CONFLICT (cik, concept, unit, accession_number, period_start, period_end, form, filing_date, frame)
DO UPDATE SET company_name = EXCLUDED.company_name, ticker = EXCLUDED.ticker,
    label = EXCLUDED.label, value = EXCLUDED.value;
`, quote(fact.CIK), quote(fact.Company), quote(ticker), quote(fact.Concept), quote(fact.Label), quote(fact.Unit), number(fact.Value), quote(fact.Start), quote(fact.End), quote(fact.FilingDate), quote(fact.Form), quote(fact.Accession), quote(fact.Frame))
	}
	script.WriteString("COMMIT;\n")
	if _, err := psql.Run(ctx, dsn, script.String()); err != nil {
		return fmt.Errorf("save SEC facts: %w", err)
	}
	return nil
}

func SaveDocument(ctx context.Context, dsn, ticker string, filing sec.Filing, document string) error {
	var script strings.Builder
	script.WriteString("BEGIN;\n")
	writeCompany(&script, filing.CIK, filing.Company, ticker)
	fmt.Fprintf(&script, `INSERT INTO sec_filings (
    cik, accession_number, company_name, form, filing_date, report_date, document_url
) VALUES (%s, %s, %s, %s, %s, %s, %s)
ON CONFLICT (cik, accession_number) DO UPDATE SET
    company_name = EXCLUDED.company_name,
    form = EXCLUDED.form,
    filing_date = EXCLUDED.filing_date,
    report_date = EXCLUDED.report_date,
    document_url = EXCLUDED.document_url;
`, quote(filing.CIK), quote(filing.AccessionNumber), quote(filing.Company), quote(filing.Form), quote(filing.FilingDate), nullableText(filing.ReportDate), quote(filing.DocumentURL))
	fmt.Fprintf(&script, `INSERT INTO sec_documents (cik, accession_number, document_text)
VALUES (%s, %s, %s)
ON CONFLICT (cik, accession_number) DO UPDATE SET
    document_text = EXCLUDED.document_text,
    fetched_at = now();
`, quote(filing.CIK), quote(filing.AccessionNumber), quote(document))
	script.WriteString("COMMIT;\n")
	if _, err := psql.Run(ctx, dsn, script.String()); err != nil {
		return fmt.Errorf("save SEC document: %w", err)
	}
	return nil
}

func writeCompany(script *strings.Builder, cik, name, ticker string) {
	fmt.Fprintf(script, `INSERT INTO sec_companies (cik, company_name, requested_ticker)
VALUES (%s, %s, %s)
ON CONFLICT (cik) DO UPDATE SET
    company_name = EXCLUDED.company_name,
    requested_ticker = EXCLUDED.requested_ticker,
    updated_at = now();
`, quote(cik), quote(name), quote(ticker))
}

func quote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "'", "''")
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, "\r", `\r`)
	value = strings.ReplaceAll(value, "\t", `\t`)
	return "E'" + value + "'"
}
func nullableText(value string) string {
	if value == "" {
		return "NULL"
	}
	return quote(value)
}
func number(value float64) string { return strconv.FormatFloat(value, 'g', -1, 64) }
