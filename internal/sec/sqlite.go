package sec

import (
	"context"
	"database/sql"
	"fmt"

	"sec/internal/sqlite"
)

func SaveFacts(ctx context.Context, path, ticker string, facts []FinancialFact) error {
	if len(facts) == 0 {
		return nil
	}
	db, err := sqlite.Open(path)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin SEC data transaction: %w", err)
	}
	defer tx.Rollback()

	if err := saveCompany(ctx, tx, facts[0], ticker); err != nil {
		return err
	}
	statement, err := tx.PrepareContext(ctx, `INSERT INTO sec_facts (
    cik, company_name, ticker, concept, label, unit, value,
    period_start, period_end, filing_date, form, accession_number, frame
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (cik, concept, unit, accession_number, period_start, period_end, form, filing_date, frame)
DO UPDATE SET company_name = excluded.company_name, ticker = excluded.ticker,
    label = excluded.label, value = excluded.value`)
	if err != nil {
		return fmt.Errorf("prepare SEC fact insert: %w", err)
	}
	defer statement.Close()

	for _, fact := range facts {
		if _, err := statement.ExecContext(ctx,
			fact.CIK, fact.Company, ticker, fact.Concept, fact.Label, fact.Unit,
			fact.Value, fact.Start, fact.End, fact.FilingDate, fact.Form,
			fact.Accession, fact.Frame,
		); err != nil {
			return fmt.Errorf("save SEC fact %s (%s): %w", fact.Concept, fact.End, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit SEC data: %w", err)
	}
	return nil
}

func saveCompany(ctx context.Context, tx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, fact FinancialFact, ticker string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO sec_companies (cik, company_name, requested_ticker)
VALUES (?, ?, ?)
ON CONFLICT (cik) DO UPDATE SET
    company_name = excluded.company_name,
    requested_ticker = excluded.requested_ticker,
    updated_at = CURRENT_TIMESTAMP`, fact.CIK, fact.Company, ticker)
	if err != nil {
		return fmt.Errorf("save SEC company: %w", err)
	}
	return nil
}
