package dcf

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"sec/internal/sqlite"
)

// ModelIDs returns all stored model IDs in ascending order.
func ModelIDs(ctx context.Context, path, ticker string) ([]int64, error) {
	query := fmt.Sprintf("SELECT id FROM dcf_models WHERE upper(ticker) = upper(%s) ORDER BY id;\n", quote(ticker))
	output, err := sqlite.Run(ctx, path, query)
	if err != nil {
		return nil, fmt.Errorf("list DCF models: %w", err)
	}
	rows, err := parseCSV(output)
	if err != nil {
		return nil, fmt.Errorf("parse DCF model IDs: %w", err)
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if len(row) != 1 {
			return nil, fmt.Errorf("DCF model ID row has %d fields; expected 1", len(row))
		}
		id, err := strconv.ParseInt(strings.TrimSpace(row[0]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse DCF model ID %q: %w", row[0], err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// LoadModelFromSQLite reads a model and its ordered forecast rows.
func LoadModelFromSQLite(ctx context.Context, path string, id int64) (Model, error) {
	var model Model
	modelSQL := fmt.Sprintf(`SELECT
    strftime('%Y-%m-%d', valuation_date),
    CAST(initial_revenue AS TEXT),
    CAST(initial_operating_nwc_rate AS TEXT),
    COALESCE(CAST(wacc AS TEXT), ''),
    COALESCE(CAST(risk_free_rate AS TEXT), ''),
    COALESCE(CAST(levered_beta AS TEXT), ''),
    COALESCE(CAST(equity_risk_premium AS TEXT), ''),
    COALESCE(CAST(pre_tax_cost_of_debt AS TEXT), ''),
    COALESCE(CAST(capm_tax_rate AS TEXT), ''),
    COALESCE(CAST(market_equity_value AS TEXT), ''),
    COALESCE(CAST(market_debt_value AS TEXT), ''),
    CAST(terminal_growth_rate AS TEXT),
    CAST(stable_roic AS TEXT),
    CAST(excess_cash AS TEXT),
    CAST(non_operating_assets AS TEXT),
    CAST(debt AS TEXT),
    CAST(other_senior_claims AS TEXT),
    CAST(diluted_shares AS TEXT)
FROM dcf_models WHERE id = %d;`, id)
	output, err := sqlite.Run(ctx, path, modelSQL)
	if err != nil {
		return model, fmt.Errorf("load DCF model %d: %w", id, err)
	}
	rows, err := parseCSV(output)
	if err != nil {
		return model, fmt.Errorf("parse DCF model row: %w", err)
	}
	if len(rows) != 1 {
		return model, fmt.Errorf("expected one DCF model with id %d, found %d", id, len(rows))
	}
	row := rows[0]
	if len(row) != 18 {
		return model, fmt.Errorf("DCF model row has %d fields; expected 18", len(row))
	}
	model.ValuationDate = row[0]
	model.InitialRevenue, err = parseFloat(row[1])
	if err != nil {
		return model, fmt.Errorf("initial_revenue: %w", err)
	}
	model.InitialNWCRate, err = parseFloat(row[2])
	if err != nil {
		return model, fmt.Errorf("initial_operating_nwc_rate: %w", err)
	}
	if row[3] != "" {
		value, parseErr := parseFloat(row[3])
		if parseErr != nil {
			return model, fmt.Errorf("wacc: %w", parseErr)
		}
		model.WACC = &value
	}
	capmValues := make([]float64, 7)
	capmPresent := 0
	for i := range capmValues {
		if row[4+i] == "" {
			continue
		}
		capmValues[i], err = parseFloat(row[4+i])
		if err != nil {
			return model, fmt.Errorf("CAPM input %d: %w", i+1, err)
		}
		capmPresent++
	}
	if capmPresent != 0 && capmPresent != len(capmValues) {
		return model, fmt.Errorf("model %d has incomplete CAPM inputs", id)
	}
	if capmPresent == len(capmValues) {
		model.CAPM = &CAPMAssumptions{
			RiskFreeRate: capmValues[0], LeveredBeta: capmValues[1],
			EquityRiskPremium: capmValues[2], CostOfDebt: capmValues[3],
			TaxRate: capmValues[4], EquityValue: capmValues[5], DebtValue: capmValues[6],
		}
	}
	model.Terminal.GrowthRate, err = parseFloat(row[11])
	if err != nil {
		return model, fmt.Errorf("terminal_growth_rate: %w", err)
	}
	model.Terminal.StableROIC, err = parseFloat(row[12])
	if err != nil {
		return model, fmt.Errorf("stable_roic: %w", err)
	}
	bridge := []*float64{&model.Equity.ExcessCash, &model.Equity.NonOperatingAssets, &model.Equity.Debt, &model.Equity.OtherSeniorClaims, &model.Equity.DilutedShares}
	for i, destination := range bridge {
		*destination, err = parseFloat(row[13+i])
		if err != nil {
			return model, fmt.Errorf("equity bridge field %d: %w", i+1, err)
		}
	}

	forecastSQL := fmt.Sprintf(`SELECT
    COALESCE(strftime('%Y-%m-%d', cash_flow_date), ''),
    CAST(revenue_growth AS TEXT), CAST(ebit_margin AS TEXT), CAST(cash_tax_rate AS TEXT),
    CAST(da_as_pct_revenue AS TEXT), CAST(capex_as_pct_revenue AS TEXT),
    CAST(operating_nwc_as_pct_revenue AS TEXT)
FROM dcf_forecast_years WHERE model_id = %d ORDER BY forecast_index;`, id)
	output, err = sqlite.Run(ctx, path, forecastSQL)
	if err != nil {
		return model, fmt.Errorf("load DCF forecast %d: %w", id, err)
	}
	rows, err = parseCSV(output)
	if err != nil {
		return model, fmt.Errorf("parse DCF forecast: %w", err)
	}
	for i, row := range rows {
		if len(row) != 7 {
			return model, fmt.Errorf("forecast row %d has %d fields; expected 7", i+1, len(row))
		}
		year := ForecastYear{Date: row[0]}
		targets := []*float64{&year.RevenueGrowth, &year.EBITMargin, &year.TaxRate, &year.DARate, &year.CapexRate, &year.NWCRate}
		for j, target := range targets {
			*target, err = parseFloat(row[j+1])
			if err != nil {
				return model, fmt.Errorf("forecast row %d field %d: %w", i+1, j+1, err)
			}
		}
		model.Forecast = append(model.Forecast, year)
	}
	return model, nil
}

// SaveResultToSQLite records a valuation and its forecast rows in one transaction.
func SaveResultToSQLite(ctx context.Context, path string, modelID int64, result Result) (int64, error) {
	var sql strings.Builder
	sql.WriteString("BEGIN;\n")
	fmt.Fprintf(&sql, `INSERT INTO dcf_valuations (
    model_id, wacc, cost_of_equity, terminal_date, next_nopat,
    terminal_reinvestment, terminal_fcff, terminal_value,
    pv_explicit_fcff, pv_terminal_value, enterprise_value,
    equity_value, value_per_share
) VALUES (%d, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s);
SELECT last_insert_rowid();
`, modelID, number(result.WACC), nullableNumber(result.CostOfEquity), quote(result.TerminalDate), number(result.NextNOPAT), number(result.TerminalReinvestment), number(result.TerminalFCFF), number(result.TerminalValue), number(result.PVExplicitFCFF), number(result.PVTerminalValue), number(result.EnterpriseValue), number(result.EquityValue), nullableNumber(result.ValuePerShare))
	for i, year := range result.Forecast {
		fmt.Fprintf(&sql, `INSERT INTO dcf_valuation_years (
    valuation_id, forecast_index, cash_flow_date, years_from_valuation,
    revenue, ebit, nopat, da, capex, change_nwc, fcff, present_value
) VALUES ((SELECT MAX(id) FROM dcf_valuations), %d, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s);
`, i+1, quote(year.Date), number(year.YearsFromValuation), number(year.Revenue), number(year.EBIT), number(year.NOPAT), number(year.DA), number(year.Capex), number(year.ChangeNWC), number(year.FCFF), number(year.PresentValue))
	}
	sql.WriteString("COMMIT;\n")
	output, err := sqlite.Run(ctx, path, sql.String())
	if err != nil {
		return 0, fmt.Errorf("save DCF valuation: %w", err)
	}
	rows, err := parseCSV(output)
	if err != nil {
		return 0, fmt.Errorf("parse returned valuation ID: %w", err)
	}
	if len(rows) == 0 || len(rows[0]) != 1 {
		return 0, fmt.Errorf("SQLite did not return a valuation ID")
	}
	id, err := strconv.ParseInt(strings.TrimSpace(rows[0][0]), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse returned valuation ID: %w", err)
	}
	return id, nil
}

func parseCSV(data []byte) ([][]string, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	var rows [][]string
	for {
		row, err := reader.Read()
		if err == io.EOF {
			return rows, nil
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
}

func parseFloat(value string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(value), 64)
}

func number(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}

func nullableNumber(value *float64) string {
	if value == nil {
		return "NULL"
	}
	return number(*value)
}

func quote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
