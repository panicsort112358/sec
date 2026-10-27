package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"sec/internal/dcf"
	"sec/internal/sqlite"
)

const usage = `usage: dcf -ticker SYMBOL [options]

Calculate stored DCF models for one ticker and append their valuations to the
SQLite database, which contains SEC data, dcf_models and forecast rows.

Reverse DCF mode uses saved SEC facts to calculate the FCFF growth implied by
a supplied share price, across a range of WACC assumptions.

options:
  -ticker string   company ticker symbol (required)
  -db string       SQLite database file (default SEC_DATABASE_PATH or sec.db)
  -price float     share price; enables reverse DCF mode
  -as-of date      valuation date YYYY-MM-DD (default today)
  -years int       explicit forecast years for reverse DCF (default 5)
  -wacc-min rate   first WACC in reverse DCF sensitivity (default 0.08)
  -wacc-max rate   last WACC in reverse DCF sensitivity (default 0.12)
  -wacc-step rate  WACC sensitivity step (default 0.01)
  -terminal-growth rate  perpetual FCFF growth rate (default 0.025)
`

func run() error {
	ticker := flag.String("ticker", "", "company ticker symbol")
	database := flag.String("db", sqlite.DefaultPath(), "SQLite database file")
	price := flag.Float64("price", 0, "share price; enables reverse DCF mode")
	asOf := flag.String("as-of", time.Now().Format("2006-01-02"), "valuation date YYYY-MM-DD")
	years := flag.Int("years", 5, "explicit forecast years for reverse DCF")
	waccMin := flag.Float64("wacc-min", 0.08, "minimum WACC for reverse DCF")
	waccMax := flag.Float64("wacc-max", 0.12, "maximum WACC for reverse DCF")
	waccStep := flag.Float64("wacc-step", 0.01, "WACC step for reverse DCF")
	terminalGrowth := flag.Float64("terminal-growth", 0.025, "terminal FCFF growth rate for reverse DCF")
	flag.Usage = func() { fmt.Fprint(flag.CommandLine.Output(), usage) }
	flag.CommandLine.SetOutput(os.Stderr)
	flag.Parse()
	priceProvided := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "price" {
			priceProvided = true
		}
	})
	if *ticker == "" {
		flag.Usage()
		return errors.New("provide -ticker")
	}
	if flag.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if priceProvided {
		return runReverse(*database, *ticker, *price, *asOf, *years, *waccMin, *waccMax, *waccStep, *terminalGrowth)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	models, err := dcf.LoadModels(ctx, *database, *ticker)
	if err != nil {
		return err
	}
	if len(models) == 0 {
		return fmt.Errorf("no DCF models found for ticker %s", *ticker)
	}

	var valuations []dcf.Valuation
	var failures []error
	for _, model := range models {
		valuation, err := calculateModel(model)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		valuations = append(valuations, valuation)
	}

	saved, err := dcf.SaveResults(ctx, *database, valuations)
	if err != nil {
		failures = append(failures, err)
		return errors.Join(failures...)
	}
	for i, valuation := range valuations {
		printResult(saved[i].ValuationID, valuation.ModelID, *ticker, valuation.Result)
	}
	return errors.Join(failures...)
}

func runReverse(path, ticker string, price float64, asOf string, years int, waccMin, waccMax, waccStep, terminalGrowth float64) error {
	if !isFinite(price) || price <= 0 {
		return errors.New("price must be positive and finite")
	}
	if !isFinite(waccMin) || !isFinite(waccMax) || !isFinite(waccStep) || waccMin <= 0 || waccMax < waccMin || waccStep <= 0 {
		return errors.New("WACC range must be positive, max must be at least min, and step must be positive")
	}
	if !isFinite(terminalGrowth) || terminalGrowth < 0 || terminalGrowth >= waccMin {
		return errors.New("terminal growth must be nonnegative and below the minimum WACC")
	}
	if math.Floor((waccMax-waccMin)/waccStep)+1 > 100 {
		return errors.New("WACC sensitivity range cannot contain more than 100 steps")
	}
	if years < 1 || years > 30 {
		return errors.New("forecast years must be between 1 and 30")
	}
	if _, err := time.Parse("2006-01-02", asOf); err != nil {
		return errors.New("as-of date must be YYYY-MM-DD")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	inputs, err := dcf.LoadReverseInputs(ctx, path, ticker, asOf)
	if err != nil {
		return err
	}
	fmt.Printf("reverse_dcf ticker=%s price=%s as_of=%s\n", ticker, money(price), asOf)
	fmt.Printf("filing_inputs periods=%d period_range=%s..%s normalized_fcff=%s tax_basis=%q\n",
		inputs.PeriodsUsed, inputs.FirstPeriod, inputs.LastPeriod,
		money(inputs.NormalizedFCFF), inputs.TaxBasis)
	fmt.Printf("equity_bridge balance_sheet_date=%s cash=%s debt=%s diluted_shares=%s diluted_shares_period=%s\n",
		inputs.BalanceDate, money(inputs.Cash), money(inputs.Debt), money(inputs.DilutedShares), inputs.SharesDate)
	fmt.Printf("assumptions forecast_years=%d terminal_growth=%s nwc_method=current_assets_less_cash_and_non_debt_current_liabilities\n",
		years, rate(terminalGrowth))
	targetEV := price*inputs.DilutedShares + inputs.Debt - inputs.Cash
	fmt.Printf("target_enterprise_value=%s\n", money(targetEV))
	found := false
	for i, wacc := 0, waccMin; wacc <= waccMax+waccStep*1e-8; i, wacc = i+1, waccMin+float64(i+1)*waccStep {
		result, err := dcf.SolveImpliedFCFFGrowth(inputs, price, wacc, terminalGrowth, years)
		if err != nil {
			fmt.Printf("wacc=%s result=unavailable reason=%s\n", rate(wacc), err)
			continue
		}
		found = true
		fmt.Printf("wacc=%s implied_fcff_growth=%s implied_enterprise_value=%s\n",
			rate(wacc), rate(result.ImpliedGrowth), money(result.ImpliedValue))
	}
	if !found {
		return errors.New("no implied growth result found across the requested WACC range")
	}
	return nil
}

func calculateModel(model dcf.StoredModel) (dcf.Valuation, error) {
	result, err := dcf.Calculate(model.Model)
	if err != nil {
		return dcf.Valuation{}, fmt.Errorf("model %d: %w", model.ID, err)
	}
	return dcf.Valuation{ModelID: model.ID, Result: result}, nil
}

func printResult(valuationID, modelID int64, ticker string, result dcf.Result) {
	fmt.Printf(
		"valuation_id=%d model_id=%d ticker=%s enterprise_value=%s equity_value=%s",
		valuationID, modelID, ticker,
		money(result.EnterpriseValue), money(result.EquityValue),
	)
	if result.ValuePerShare != nil {
		fmt.Printf(" value_per_share=%s", money(*result.ValuePerShare))
	}
	fmt.Println()
}

func money(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func rate(value float64) string { return strconv.FormatFloat(value*100, 'f', 2, 64) + "%" }

func isFinite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
