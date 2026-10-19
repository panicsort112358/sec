package dcf

import (
	"context"
	"database/sql"
	"fmt"

	"sec/internal/sqlite"
)

type StoredModel struct {
	ID    int64
	Model Model
}

type Valuation struct {
	ModelID int64
	Result  Result
}

type SavedValuation struct {
	ModelID     int64
	ValuationID int64
}

type modelRow struct {
	id                 int64
	valuationDate      sql.NullString
	initialRevenue     float64
	initialNWCRate     float64
	wacc               sql.NullFloat64
	riskFreeRate       sql.NullFloat64
	leveredBeta        sql.NullFloat64
	equityRiskPremium  sql.NullFloat64
	costOfDebt         sql.NullFloat64
	capmTaxRate        sql.NullFloat64
	marketEquityValue  sql.NullFloat64
	marketDebtValue    sql.NullFloat64
	terminalGrowthRate float64
	stableROIC         float64
	excessCash         float64
	nonOperatingAssets float64
	debt               float64
	otherSeniorClaims  float64
	dilutedShares      float64
	forecastIndex      sql.NullInt64
	forecastDate       sql.NullString
	revenueGrowth      sql.NullFloat64
	ebitMargin         sql.NullFloat64
	cashTaxRate        sql.NullFloat64
	daRate             sql.NullFloat64
	capexRate          sql.NullFloat64
	nwcRate            sql.NullFloat64
}

// LoadModels reads all models and forecasts for one ticker in a single query.
func LoadModels(ctx context.Context, path, ticker string) ([]StoredModel, error) {
	db, err := sqlite.Open(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `SELECT
    m.id,
    strftime('%Y-%m-%d', m.valuation_date),
    m.initial_revenue,
    m.initial_operating_nwc_rate,
    m.wacc,
    m.risk_free_rate,
    m.levered_beta,
    m.equity_risk_premium,
    m.pre_tax_cost_of_debt,
    m.capm_tax_rate,
    m.market_equity_value,
    m.market_debt_value,
    m.terminal_growth_rate,
    m.stable_roic,
    m.excess_cash,
    m.non_operating_assets,
    m.debt,
    m.other_senior_claims,
    m.diluted_shares,
    f.forecast_index,
    strftime('%Y-%m-%d', f.cash_flow_date),
    f.revenue_growth,
    f.ebit_margin,
    f.cash_tax_rate,
    f.da_as_pct_revenue,
    f.capex_as_pct_revenue,
    f.operating_nwc_as_pct_revenue
FROM dcf_models AS m
LEFT JOIN dcf_forecast_years AS f ON f.model_id = m.id
WHERE upper(m.ticker) = upper(?)
ORDER BY m.id, f.forecast_index`, ticker)
	if err != nil {
		return nil, fmt.Errorf("load DCF models for %s: %w", ticker, err)
	}
	defer rows.Close()

	models := make([]StoredModel, 0)
	for rows.Next() {
		var row modelRow
		if err := row.scan(rows); err != nil {
			return nil, fmt.Errorf("read DCF model for %s: %w", ticker, err)
		}
		if len(models) == 0 || models[len(models)-1].ID != row.id {
			model, err := row.toModel()
			if err != nil {
				return nil, fmt.Errorf("model %d: %w", row.id, err)
			}
			models = append(models, StoredModel{ID: row.id, Model: model})
		}
		if row.forecastIndex.Valid {
			year, err := row.forecastYear()
			if err != nil {
				return nil, fmt.Errorf("model %d forecast %d: %w", row.id, row.forecastIndex.Int64, err)
			}
			last := len(models) - 1
			models[last].Model.Forecast = append(models[last].Model.Forecast, year)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read DCF models for %s: %w", ticker, err)
	}
	return models, nil
}

func (r *modelRow) scan(rows *sql.Rows) error {
	return rows.Scan(
		&r.id, &r.valuationDate, &r.initialRevenue, &r.initialNWCRate,
		&r.wacc, &r.riskFreeRate, &r.leveredBeta, &r.equityRiskPremium,
		&r.costOfDebt, &r.capmTaxRate, &r.marketEquityValue, &r.marketDebtValue,
		&r.terminalGrowthRate, &r.stableROIC, &r.excessCash,
		&r.nonOperatingAssets, &r.debt, &r.otherSeniorClaims, &r.dilutedShares,
		&r.forecastIndex, &r.forecastDate, &r.revenueGrowth, &r.ebitMargin,
		&r.cashTaxRate, &r.daRate, &r.capexRate, &r.nwcRate,
	)
}

func (r modelRow) toModel() (Model, error) {
	if !r.valuationDate.Valid {
		return Model{}, fmt.Errorf("valuation date is missing")
	}
	m := Model{
		ValuationDate:  r.valuationDate.String,
		InitialRevenue: r.initialRevenue,
		InitialNWCRate: r.initialNWCRate,
		WACC:           nullFloat(r.wacc),
		Terminal: TerminalAssumptions{
			GrowthRate: r.terminalGrowthRate,
			StableROIC: r.stableROIC,
		},
		Equity: EquityBridge{
			ExcessCash:         r.excessCash,
			NonOperatingAssets: r.nonOperatingAssets,
			Debt:               r.debt,
			OtherSeniorClaims:  r.otherSeniorClaims,
			DilutedShares:      r.dilutedShares,
		},
	}
	capm := []sql.NullFloat64{r.riskFreeRate, r.leveredBeta, r.equityRiskPremium, r.costOfDebt, r.capmTaxRate, r.marketEquityValue, r.marketDebtValue}
	present := 0
	for _, value := range capm {
		if value.Valid {
			present++
		}
	}
	if present != 0 && present != len(capm) {
		return Model{}, fmt.Errorf("incomplete CAPM inputs")
	}
	if present == len(capm) {
		m.CAPM = &CAPMAssumptions{
			RiskFreeRate:      r.riskFreeRate.Float64,
			LeveredBeta:       r.leveredBeta.Float64,
			EquityRiskPremium: r.equityRiskPremium.Float64,
			CostOfDebt:        r.costOfDebt.Float64,
			TaxRate:           r.capmTaxRate.Float64,
			EquityValue:       r.marketEquityValue.Float64,
			DebtValue:         r.marketDebtValue.Float64,
		}
	}
	return m, nil
}

func (r modelRow) forecastYear() (ForecastYear, error) {
	values := []sql.NullFloat64{r.revenueGrowth, r.ebitMargin, r.cashTaxRate, r.daRate, r.capexRate, r.nwcRate}
	for i, value := range values {
		if !value.Valid {
			return ForecastYear{}, fmt.Errorf("forecast field %d is null", i+1)
		}
	}
	year := ForecastYear{
		RevenueGrowth: r.revenueGrowth.Float64,
		EBITMargin:    r.ebitMargin.Float64,
		TaxRate:       r.cashTaxRate.Float64,
		DARate:        r.daRate.Float64,
		CapexRate:     r.capexRate.Float64,
		NWCRate:       r.nwcRate.Float64,
	}
	if r.forecastDate.Valid {
		year.Date = r.forecastDate.String
	}
	return year, nil
}

// SaveResults stores a batch of valuations in one transaction.
func SaveResults(ctx context.Context, path string, valuations []Valuation) ([]SavedValuation, error) {
	if len(valuations) == 0 {
		return nil, nil
	}
	db, err := sqlite.Open(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin DCF valuation transaction: %w", err)
	}
	defer tx.Rollback()

	valuationInsert, err := tx.PrepareContext(ctx, `INSERT INTO dcf_valuations (
    model_id, wacc, cost_of_equity, terminal_date, next_nopat,
    terminal_reinvestment, terminal_fcff, terminal_value,
    pv_explicit_fcff, pv_terminal_value, enterprise_value,
    equity_value, value_per_share
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return nil, fmt.Errorf("prepare DCF valuation insert: %w", err)
	}
	defer valuationInsert.Close()

	yearInsert, err := tx.PrepareContext(ctx, `INSERT INTO dcf_valuation_years (
    valuation_id, forecast_index, cash_flow_date, years_from_valuation,
    revenue, ebit, nopat, da, capex, change_nwc, fcff, present_value
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return nil, fmt.Errorf("prepare DCF forecast insert: %w", err)
	}
	defer yearInsert.Close()

	saved := make([]SavedValuation, 0, len(valuations))
	for _, valuation := range valuations {
		result := valuation.Result
		insertResult, err := valuationInsert.ExecContext(ctx,
			valuation.ModelID, result.WACC, result.CostOfEquity, result.TerminalDate,
			result.NextNOPAT, result.TerminalReinvestment, result.TerminalFCFF,
			result.TerminalValue, result.PVExplicitFCFF, result.PVTerminalValue,
			result.EnterpriseValue, result.EquityValue, result.ValuePerShare,
		)
		if err != nil {
			return nil, fmt.Errorf("save valuation for model %d: %w", valuation.ModelID, err)
		}
		valuationID, err := insertResult.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("read valuation ID for model %d: %w", valuation.ModelID, err)
		}
		for i, year := range result.Forecast {
			if _, err := yearInsert.ExecContext(ctx,
				valuationID, i+1, year.Date, year.YearsFromValuation,
				year.Revenue, year.EBIT, year.NOPAT, year.DA, year.Capex,
				year.ChangeNWC, year.FCFF, year.PresentValue,
			); err != nil {
				return nil, fmt.Errorf("save forecast year %d for model %d: %w", i+1, valuation.ModelID, err)
			}
		}
		saved = append(saved, SavedValuation{ModelID: valuation.ModelID, ValuationID: valuationID})
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit DCF valuations: %w", err)
	}
	return saved, nil
}

func nullFloat(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	result := value.Float64
	return &result
}
