// Package dcf provides an assumptions-driven, unlevered discounted cash flow model.
// It deliberately has no dependency on SEC response types: callers normalize
// reported data and supply forecast assumptions separately.
package dcf

import (
	"fmt"
	"math"
	"time"
)

type ForecastYear struct {
	Date          string  `json:"date,omitempty"` // cash-flow date; defaults to year end
	RevenueGrowth float64 `json:"revenue_growth"`
	EBITMargin    float64 `json:"ebit_margin"`
	TaxRate       float64 `json:"cash_tax_rate"`
	DARate        float64 `json:"da_as_pct_revenue"`
	CapexRate     float64 `json:"capex_as_pct_revenue"`
	NWCRate       float64 `json:"operating_nwc_as_pct_revenue"`
}

type CAPMAssumptions struct {
	RiskFreeRate      float64 `json:"risk_free_rate"`
	LeveredBeta       float64 `json:"levered_beta"`
	EquityRiskPremium float64 `json:"equity_risk_premium"`
	CostOfDebt        float64 `json:"pre_tax_cost_of_debt"`
	TaxRate           float64 `json:"tax_rate"`
	EquityValue       float64 `json:"market_equity_value"`
	DebtValue         float64 `json:"market_debt_value"`
}

type TerminalAssumptions struct {
	GrowthRate float64 `json:"growth_rate"`
	StableROIC float64 `json:"stable_roic"`
}

type EquityBridge struct {
	ExcessCash         float64 `json:"excess_cash"`
	NonOperatingAssets float64 `json:"non_operating_assets"`
	Debt               float64 `json:"debt"`
	OtherSeniorClaims  float64 `json:"other_senior_claims"`
	DilutedShares      float64 `json:"diluted_shares"`
}

type Model struct {
	ValuationDate  string              `json:"valuation_date"` // YYYY-MM-DD
	InitialRevenue float64             `json:"initial_revenue"`
	InitialNWCRate float64             `json:"initial_operating_nwc_as_pct_revenue"`
	WACC           *float64            `json:"wacc,omitempty"`
	CAPM           *CAPMAssumptions    `json:"capm,omitempty"`
	Forecast       []ForecastYear      `json:"forecast"`
	Terminal       TerminalAssumptions `json:"terminal"`
	Equity         EquityBridge        `json:"equity_bridge"`
}

type YearResult struct {
	Date               string  `json:"date"`
	YearsFromValuation float64 `json:"years_from_valuation"`
	Revenue            float64 `json:"revenue"`
	EBIT               float64 `json:"ebit"`
	NOPAT              float64 `json:"nopat"`
	DA                 float64 `json:"da"`
	Capex              float64 `json:"capex"`
	ChangeNWC          float64 `json:"change_nwc"`
	FCFF               float64 `json:"fcff"`
	PresentValue       float64 `json:"present_value"`
}

type Result struct {
	WACC                 float64      `json:"wacc"`
	CostOfEquity         *float64     `json:"cost_of_equity,omitempty"`
	Forecast             []YearResult `json:"forecast"`
	TerminalDate         string       `json:"terminal_date"`
	NextNOPAT            float64      `json:"next_nopat"`
	TerminalReinvestment float64      `json:"terminal_reinvestment"`
	TerminalFCFF         float64      `json:"terminal_fcff"`
	TerminalValue        float64      `json:"terminal_value"`
	PVExplicitFCFF       float64      `json:"pv_explicit_fcff"`
	PVTerminalValue      float64      `json:"pv_terminal_value"`
	EnterpriseValue      float64      `json:"enterprise_value"`
	EquityValue          float64      `json:"equity_value"`
	ValuePerShare        *float64     `json:"value_per_share,omitempty"`
}

func Calculate(m Model) (Result, error) {
	var result Result

	valuationDate, wacc, costOfEquity, err := validateModel(m)
	if err != nil {
		return result, err
	}
	result.WACC = wacc
	result.CostOfEquity = costOfEquity

	forecast, presentValue, terminalDate, err := projectForecast(m, valuationDate, wacc)
	if err != nil {
		return result, err
	}
	result.Forecast = forecast
	result.PVExplicitFCFF = presentValue

	if err := addTerminalValue(&result, m.Terminal, valuationDate, terminalDate, wacc); err != nil {
		return result, err
	}
	if err := addEquityValue(&result, m.Equity); err != nil {
		return result, err
	}
	return result, nil
}

func validateModel(m Model) (time.Time, float64, *float64, error) {
	if m.InitialRevenue <= 0 || !finite(m.InitialRevenue) {
		return time.Time{}, 0, nil, fmt.Errorf("initial_revenue must be positive and finite")
	}
	if len(m.Forecast) == 0 {
		return time.Time{}, 0, nil, fmt.Errorf("forecast must contain at least one year")
	}
	valuation, err := time.Parse("2006-01-02", m.ValuationDate)
	if err != nil {
		return time.Time{}, 0, nil, fmt.Errorf("valuation_date must be YYYY-MM-DD: %w", err)
	}
	wacc, costEquity, err := calculateWACC(m)
	if err != nil {
		return time.Time{}, 0, nil, err
	}
	if !finite(m.InitialNWCRate) {
		return time.Time{}, 0, nil, fmt.Errorf("initial operating NWC rate must be finite")
	}
	return valuation, wacc, costEquity, nil
}

func projectForecast(m Model, valuation time.Time, wacc float64) ([]YearResult, float64, time.Time, error) {
	forecast := make([]YearResult, 0, len(m.Forecast))
	presentValue := 0.0
	prevRevenue, prevNWC := m.InitialRevenue, m.InitialRevenue*m.InitialNWCRate
	prevDate := valuation
	for i, a := range m.Forecast {
		if err := validateYear(a, i); err != nil {
			return nil, 0, time.Time{}, err
		}
		date := prevDate.AddDate(1, 0, 0)
		if a.Date != "" {
			parsedDate, err := time.Parse("2006-01-02", a.Date)
			if err != nil {
				return nil, 0, time.Time{}, fmt.Errorf("forecast[%d].date must be YYYY-MM-DD", i)
			}
			date = parsedDate
		}
		if !date.After(prevDate) {
			return nil, 0, time.Time{}, fmt.Errorf("forecast dates must be strictly after valuation date and increasing")
		}
		revenue := prevRevenue * (1 + a.RevenueGrowth)
		ebit := revenue * a.EBITMargin
		nopat := ebit * (1 - a.TaxRate)
		da, capex := revenue*a.DARate, revenue*a.CapexRate
		nwc := revenue * a.NWCRate
		changeNWC := nwc - prevNWC
		fcff := nopat + da - capex - changeNWC
		years := date.Sub(valuation).Hours() / (24 * 365.2425)
		pv := fcff / math.Pow(1+wacc, years)
		forecast = append(forecast, YearResult{
			Date:               date.Format("2006-01-02"),
			YearsFromValuation: years,
			Revenue:            revenue,
			EBIT:               ebit,
			NOPAT:              nopat,
			DA:                 da,
			Capex:              capex,
			ChangeNWC:          changeNWC,
			FCFF:               fcff,
			PresentValue:       pv,
		})
		presentValue += pv
		prevRevenue, prevNWC, prevDate = revenue, nwc, date
	}
	return forecast, presentValue, prevDate, nil
}

func addTerminalValue(result *Result, assumptions TerminalAssumptions, valuationDate, terminalDate time.Time, wacc float64) error {
	if !finite(assumptions.GrowthRate) || !finite(assumptions.StableROIC) || assumptions.StableROIC <= 0 {
		return fmt.Errorf("terminal stable_roic must be positive and terminal growth_rate finite")
	}
	if assumptions.GrowthRate < 0 || assumptions.GrowthRate >= wacc {
		return fmt.Errorf("terminal growth_rate must be nonnegative and below WACC")
	}
	if assumptions.StableROIC < assumptions.GrowthRate {
		return fmt.Errorf("terminal stable_roic must be at least terminal growth_rate")
	}

	lastYear := result.Forecast[len(result.Forecast)-1]
	nextNOPAT := lastYear.NOPAT * (1 + assumptions.GrowthRate)
	reinvestment := nextNOPAT * assumptions.GrowthRate / assumptions.StableROIC
	terminalFCFF := nextNOPAT - reinvestment
	terminalValue := terminalFCFF / (wacc - assumptions.GrowthRate)
	years := terminalDate.Sub(valuationDate).Hours() / (24 * 365.2425)
	presentValue := terminalValue / math.Pow(1+wacc, years)

	result.TerminalDate = terminalDate.Format("2006-01-02")
	result.NextNOPAT = nextNOPAT
	result.TerminalReinvestment = reinvestment
	result.TerminalFCFF = terminalFCFF
	result.TerminalValue = terminalValue
	result.PVTerminalValue = presentValue
	result.EnterpriseValue = result.PVExplicitFCFF + presentValue
	return nil
}

func addEquityValue(result *Result, bridge EquityBridge) error {
	bridgeValues := []float64{bridge.ExcessCash, bridge.NonOperatingAssets, bridge.Debt, bridge.OtherSeniorClaims, bridge.DilutedShares}
	for _, value := range bridgeValues {
		if !finite(value) {
			return fmt.Errorf("equity bridge values must be finite")
		}
	}
	if bridge.DilutedShares < 0 {
		return fmt.Errorf("diluted_shares cannot be negative or nonfinite")
	}
	result.EquityValue = result.EnterpriseValue + bridge.ExcessCash + bridge.NonOperatingAssets - bridge.Debt - bridge.OtherSeniorClaims
	if bridge.DilutedShares > 0 {
		valuePerShare := result.EquityValue / bridge.DilutedShares
		result.ValuePerShare = &valuePerShare
	}
	return nil
}

func calculateWACC(m Model) (float64, *float64, error) {
	if m.WACC != nil {
		if !finite(*m.WACC) || *m.WACC <= 0 {
			return 0, nil, fmt.Errorf("wacc must be positive and finite")
		}
		return *m.WACC, nil, nil
	}
	c := m.CAPM
	if c == nil {
		return 0, nil, fmt.Errorf("provide wacc or capm assumptions")
	}
	vals := []float64{c.RiskFreeRate, c.LeveredBeta, c.EquityRiskPremium, c.CostOfDebt, c.TaxRate, c.EquityValue, c.DebtValue}
	for _, v := range vals {
		if !finite(v) {
			return 0, nil, fmt.Errorf("CAPM inputs must be finite")
		}
	}
	if c.CostOfDebt < 0 {
		return 0, nil, fmt.Errorf("pre_tax_cost_of_debt cannot be negative")
	}
	if c.EquityValue < 0 || c.DebtValue < 0 || c.EquityValue+c.DebtValue <= 0 || c.TaxRate < 0 || c.TaxRate > 1 {
		return 0, nil, fmt.Errorf("CAPM capital values must be nonnegative, total capital positive, and tax_rate between 0 and 1")
	}
	re := c.RiskFreeRate + c.LeveredBeta*c.EquityRiskPremium
	w := c.EquityValue/(c.EquityValue+c.DebtValue)*re + c.DebtValue/(c.EquityValue+c.DebtValue)*c.CostOfDebt*(1-c.TaxRate)
	if !finite(w) || w <= 0 {
		return 0, nil, fmt.Errorf("calculated WACC must be positive and finite")
	}
	return w, &re, nil
}

func validateYear(a ForecastYear, i int) error {
	vals := []float64{a.RevenueGrowth, a.EBITMargin, a.TaxRate, a.DARate, a.CapexRate, a.NWCRate}
	for _, v := range vals {
		if !finite(v) {
			return fmt.Errorf("forecast[%d] assumptions must be finite", i)
		}
	}
	if a.RevenueGrowth <= -1 || a.TaxRate < 0 || a.TaxRate > 1 || a.DARate < 0 || a.CapexRate < 0 {
		return fmt.Errorf("forecast[%d] has invalid growth, tax, D&A, or capex assumption", i)
	}
	return nil
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
