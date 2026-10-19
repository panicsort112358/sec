package dcf

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"sec/internal/psql"
)

// LoadModelFromPostgres reads one model and its ordered forecast rows by using
// the psql client. This keeps the Go module dependency-free.
func LoadModelFromPostgres(ctx context.Context, dsn string, id int64) (Model, error) {
	var model Model
	modelSQL := fmt.Sprintf(`SELECT
    to_char(valuation_date, 'YYYY-MM-DD'),
    initial_revenue::text,
    initial_operating_nwc_rate::text,
    COALESCE(wacc::text, ''),
    COALESCE(risk_free_rate::text, ''),
    COALESCE(levered_beta::text, ''),
    COALESCE(equity_risk_premium::text, ''),
    COALESCE(pre_tax_cost_of_debt::text, ''),
    COALESCE(capm_tax_rate::text, ''),
    COALESCE(market_equity_value::text, ''),
    COALESCE(market_debt_value::text, ''),
    terminal_growth_rate::text,
    stable_roic::text,
    excess_cash::text,
    non_operating_assets::text,
    debt::text,
    other_senior_claims::text,
    diluted_shares::text
FROM dcf_models WHERE id = %d;`, id)
	output, err := psql.Run(ctx, dsn, modelSQL)
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
    COALESCE(to_char(cash_flow_date, 'YYYY-MM-DD'), ''),
    revenue_growth::text, ebit_margin::text, cash_tax_rate::text,
    da_as_pct_revenue::text, capex_as_pct_revenue::text,
    operating_nwc_as_pct_revenue::text
FROM dcf_forecast_years WHERE model_id = %d ORDER BY forecast_index;`, id)
	output, err = psql.Run(ctx, dsn, forecastSQL)
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

// SaveResultToPostgres records a valuation and its annual forecast output in a
// single transaction, returning the generated valuation ID.
func SaveResultToPostgres(ctx context.Context, dsn string, modelID int64, result Result) (int64, error) {
	var sql strings.Builder
	sql.WriteString("BEGIN;\n")
	fmt.Fprintf(&sql, `INSERT INTO dcf_valuations (
    model_id, wacc, cost_of_equity, terminal_date, next_nopat,
    terminal_reinvestment, terminal_fcff, terminal_value,
    pv_explicit_fcff, pv_terminal_value, enterprise_value,
    equity_value, value_per_share
) VALUES (%d, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
RETURNING id AS valuation_id \gset
`, modelID, number(result.WACC), nullableNumber(result.CostOfEquity), quote(result.TerminalDate), number(result.NextNOPAT), number(result.TerminalReinvestment), number(result.TerminalFCFF), number(result.TerminalValue), number(result.PVExplicitFCFF), number(result.PVTerminalValue), number(result.EnterpriseValue), number(result.EquityValue), nullableNumber(result.ValuePerShare))
	for i, year := range result.Forecast {
		fmt.Fprintf(&sql, `INSERT INTO dcf_valuation_years (
    valuation_id, forecast_index, cash_flow_date, years_from_valuation,
    revenue, ebit, nopat, da, capex, change_nwc, fcff, present_value
) VALUES (:valuation_id, %d, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s);
`, i+1, quote(year.Date), number(year.YearsFromValuation), number(year.Revenue), number(year.EBIT), number(year.NOPAT), number(year.DA), number(year.Capex), number(year.ChangeNWC), number(year.FCFF), number(year.PresentValue))
	}
	sql.WriteString("COMMIT;\nSELECT :valuation_id;\n")
	output, err := psql.Run(ctx, dsn, sql.String())
	if err != nil {
		return 0, fmt.Errorf("save DCF valuation: %w", err)
	}
	lines := strings.Fields(string(output))
	if len(lines) == 0 {
		return 0, fmt.Errorf("PostgreSQL did not return a valuation ID")
	}
	id, err := strconv.ParseInt(lines[len(lines)-1], 10, 64)
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
