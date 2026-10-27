package dcf

import (
	"fmt"
	"math"
)

// ReverseInputs are historical filing-derived values used by reverse DCF.
// Monetary values are in the same units as the SEC facts (normally USD).
type ReverseInputs struct {
	AsOfDate       string
	PeriodsUsed    int
	FirstPeriod    string
	LastPeriod     string
	BalanceDate    string
	SharesDate     string
	TaxBasis       string
	NormalizedFCFF float64
	Cash           float64
	Debt           float64
	DilutedShares  float64
}

type ReverseResult struct {
	WACC             float64
	ImpliedGrowth    float64
	TargetEnterprise float64
	ImpliedValue     float64
}

// SolveImpliedFCFFGrowth finds the constant annual explicit-period FCFF
// growth rate that makes the DCF equity value equal pricePerShare.
func SolveImpliedFCFFGrowth(inputs ReverseInputs, pricePerShare, wacc, terminalGrowth float64, years int) (ReverseResult, error) {
	if inputs.NormalizedFCFF <= 0 || !finite(inputs.NormalizedFCFF) {
		return ReverseResult{}, fmt.Errorf("normalized historical FCFF must be positive")
	}
	if inputs.DilutedShares <= 0 || !finite(inputs.DilutedShares) {
		return ReverseResult{}, fmt.Errorf("diluted shares must be positive")
	}
	if !finite(pricePerShare) || pricePerShare <= 0 {
		return ReverseResult{}, fmt.Errorf("price must be positive and finite")
	}
	if !finite(wacc) || wacc <= 0 || !finite(terminalGrowth) || terminalGrowth < 0 || terminalGrowth >= wacc {
		return ReverseResult{}, fmt.Errorf("WACC must be positive and terminal growth must be nonnegative and below WACC")
	}
	if years < 1 || years > 30 {
		return ReverseResult{}, fmt.Errorf("forecast years must be between 1 and 30")
	}

	targetEV := pricePerShare*inputs.DilutedShares + inputs.Debt - inputs.Cash
	if !finite(targetEV) || targetEV <= 0 {
		return ReverseResult{}, fmt.Errorf("price and filing-based equity bridge imply nonpositive enterprise value")
	}
	valueAt := func(growth float64) float64 {
		pv := 0.0
		fcff := inputs.NormalizedFCFF
		for year := 1; year <= years; year++ {
			fcff *= 1 + growth
			pv += fcff / math.Pow(1+wacc, float64(year))
		}
		terminalFCFF := fcff * (1 + terminalGrowth)
		terminalValue := terminalFCFF / (wacc - terminalGrowth)
		pv += terminalValue / math.Pow(1+wacc, float64(years))
		return pv
	}

	low, high := -0.99, 1.0
	for valueAt(high) < targetEV && high < 100 {
		high = high*2 + 0.1
	}
	if valueAt(low) > targetEV || valueAt(high) < targetEV {
		return ReverseResult{}, fmt.Errorf("target enterprise value is outside the solvable growth range (-99%% to %.0f%%)", high*100)
	}
	for i := 0; i < 200; i++ {
		mid := (low + high) / 2
		if valueAt(mid) < targetEV {
			low = mid
		} else {
			high = mid
		}
	}
	growth := (low + high) / 2
	return ReverseResult{
		WACC:             wacc,
		ImpliedGrowth:    growth,
		TargetEnterprise: targetEV,
		ImpliedValue:     valueAt(growth),
	}, nil
}
