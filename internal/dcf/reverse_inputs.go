package dcf

import (
	"context"
	"fmt"
	"sort"
	"time"

	"sec/internal/sqlite"
)

type storedFact struct {
	concept, unit, start, end, filed, form string
	value                                  float64
}

type annualFlows struct {
	end                                    string
	revenue, ebit, tax, pretax, da, capex  float64
	revenuePriority                        int
	hasRevenue, hasEBIT, hasTax, hasPretax bool
	hasDA, hasCapex                        bool
}

// LoadReverseInputs derives a normalized starting FCFF and equity bridge from
// annual XBRL facts saved by the sec command. NWC is approximated as current
// assets excluding cash, less current liabilities excluding current debt.
func LoadReverseInputs(ctx context.Context, path, ticker, asOf string) (ReverseInputs, error) {
	if _, err := time.Parse("2006-01-02", asOf); err != nil {
		return ReverseInputs{}, fmt.Errorf("as-of date must be YYYY-MM-DD")
	}
	db, err := sqlite.Open(path)
	if err != nil {
		return ReverseInputs{}, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `SELECT concept, unit, period_start, period_end, filing_date, form, value
FROM sec_facts
WHERE upper(ticker) = upper(?) AND filing_date <= ? AND period_end <= ?
  AND concept IN (
    'RevenueFromContractWithCustomerExcludingAssessedTax','Revenues','SalesRevenueNet',
    'OperatingIncomeLoss','IncomeTaxExpenseBenefit',
    'IncomeLossFromContinuingOperationsBeforeIncomeTaxesExtraordinaryItemsNoncontrollingInterest',
    'DepreciationDepletionAndAmortization','PaymentsToAcquirePropertyPlantAndEquipment',
    'AssetsCurrent','LiabilitiesCurrent','CashAndCashEquivalentsAtCarryingValue',
    'ShortTermBorrowings','LongTermDebtCurrent','LongTermDebtNoncurrent','LongTermDebt',
    'WeightedAverageNumberOfDilutedSharesOutstanding'
  )`, ticker, asOf, asOf)
	if err != nil {
		return ReverseInputs{}, fmt.Errorf("load SEC facts for %s: %w", ticker, err)
	}
	defer rows.Close()
	facts := make([]storedFact, 0)
	for rows.Next() {
		var f storedFact
		if err := rows.Scan(&f.concept, &f.unit, &f.start, &f.end, &f.filed, &f.form, &f.value); err != nil {
			return ReverseInputs{}, err
		}
		facts = append(facts, f)
	}
	if err := rows.Err(); err != nil {
		return ReverseInputs{}, err
	}
	if len(facts) == 0 {
		return ReverseInputs{}, fmt.Errorf("no SEC facts found for %s as of %s; run sec -ticker %s first", ticker, asOf, ticker)
	}

	// For duplicate comparative periods, use the latest filing available as of
	// the valuation date. Revenue concepts are tried in a fixed priority order.
	sort.Slice(facts, func(i, j int) bool {
		if facts[i].filed != facts[j].filed {
			return facts[i].filed > facts[j].filed
		}
		return facts[i].concept < facts[j].concept
	})
	annualByEnd := make(map[string]*annualFlows)
	for _, f := range facts {
		if !annualForm(f.form) || f.unit != "USD" || f.start == "" || !annualDuration(f.start, f.end) {
			continue
		}
		a := annualByEnd[f.end]
		if a == nil {
			a = &annualFlows{end: f.end}
			annualByEnd[f.end] = a
		}
		switch f.concept {
		case "RevenueFromContractWithCustomerExcludingAssessedTax", "Revenues", "SalesRevenueNet":
			a.setRevenue(f.concept, f.value)
		case "OperatingIncomeLoss":
			if !a.hasEBIT {
				a.ebit, a.hasEBIT = f.value, true
			}
		case "IncomeTaxExpenseBenefit":
			if !a.hasTax {
				a.tax, a.hasTax = f.value, true
			}
		case "IncomeLossFromContinuingOperationsBeforeIncomeTaxesExtraordinaryItemsNoncontrollingInterest":
			if !a.hasPretax {
				a.pretax, a.hasPretax = f.value, true
			}
		case "DepreciationDepletionAndAmortization":
			if !a.hasDA {
				a.da, a.hasDA = f.value, true
			}
		case "PaymentsToAcquirePropertyPlantAndEquipment":
			if !a.hasCapex {
				a.capex, a.hasCapex = f.value, true
			}
		}
	}

	ends := make([]string, 0, len(annualByEnd))
	for end := range annualByEnd {
		ends = append(ends, end)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ends)))
	if len(ends) < 2 {
		return ReverseInputs{}, fmt.Errorf("need at least two annual filing periods for %s; refresh SEC facts with ./sec -ticker %s", ticker, ticker)
	}

	balances := balanceSnapshots(facts)
	var fcffs []float64
	var usedEnds []string
	usedOperatingIncomeTaxProxy := false
	for i := 0; i < len(ends)-1 && len(fcffs) < 3; i++ {
		a := annualByEnd[ends[i]]
		prior := annualByEnd[ends[i+1]]
		if !(a.hasRevenue && a.hasEBIT && a.hasTax && a.hasDA && a.hasCapex) {
			continue
		}
		currentNWC, ok1 := balances[a.end]
		priorNWC, ok2 := balances[prior.end]
		if !ok1 || !ok2 {
			continue
		}
		taxBase := a.ebit
		if a.hasPretax && a.pretax > 0 {
			taxBase = a.pretax
		} else {
			usedOperatingIncomeTaxProxy = true
		}
		if taxBase <= 0 {
			continue
		}
		taxRate := a.tax / taxBase
		if taxRate < 0 {
			taxRate = 0
		}
		if taxRate > 1 {
			taxRate = 1
		}
		fcff := a.ebit*(1-taxRate) + mathAbs(a.da) - mathAbs(a.capex) - (currentNWC.nwc - priorNWC.nwc)
		if !finite(fcff) {
			continue
		}
		fcffs = append(fcffs, fcff)
		usedEnds = append(usedEnds, a.end)
	}
	if len(fcffs) == 0 {
		return ReverseInputs{}, fmt.Errorf("could not derive annual FCFF for %s; refresh SEC facts with ./sec -ticker %s", ticker, ticker)
	}
	baseFCFF := 0.0
	for _, v := range fcffs {
		baseFCFF += v
	}
	baseFCFF /= float64(len(fcffs))
	if baseFCFF <= 0 {
		return ReverseInputs{}, fmt.Errorf("normalized historical FCFF for %s is not positive", ticker)
	}
	firstPeriod := usedEnds[len(usedEnds)-1]
	lastPeriod := usedEnds[0]

	latestBalanceEnd := ""
	for end := range balances {
		if end <= asOf && end > latestBalanceEnd {
			latestBalanceEnd = end
		}
	}
	if latestBalanceEnd == "" {
		return ReverseInputs{}, fmt.Errorf("no usable balance sheet facts for %s", ticker)
	}
	latestBalance := balances[latestBalanceEnd]
	shares, sharesDate := latestDilutedShares(facts)
	if shares <= 0 {
		return ReverseInputs{}, fmt.Errorf("no recent diluted share count found for %s", ticker)
	}
	taxBasis := "reported pretax income"
	if usedOperatingIncomeTaxProxy {
		taxBasis = "operating income proxy where pretax income was unavailable"
	}
	return ReverseInputs{
		AsOfDate: asOf, PeriodsUsed: len(fcffs), FirstPeriod: firstPeriod,
		LastPeriod: lastPeriod, BalanceDate: latestBalanceEnd, SharesDate: sharesDate,
		TaxBasis: taxBasis, NormalizedFCFF: baseFCFF, Cash: latestBalance.cash,
		Debt: latestBalance.debt, DilutedShares: shares,
	}, nil
}

type balanceSnapshot struct{ cash, debt, nwc float64 }

func balanceSnapshots(facts []storedFact) map[string]balanceSnapshot {
	type values struct{ facts map[string]storedFact }
	byEnd := map[string]*values{}
	for _, f := range facts {
		if f.unit != "USD" || f.start != "" {
			continue
		}
		v := byEnd[f.end]
		if v == nil {
			v = &values{facts: map[string]storedFact{}}
			byEnd[f.end] = v
		}
		if _, exists := v.facts[f.concept]; !exists {
			v.facts[f.concept] = f
		}
	}
	out := map[string]balanceSnapshot{}
	for end, v := range byEnd {
		assets, aok := v.facts["AssetsCurrent"]
		liabilities, lok := v.facts["LiabilitiesCurrent"]
		cash, cok := v.facts["CashAndCashEquivalentsAtCarryingValue"]
		if !aok || !lok || !cok {
			continue
		}
		shortDebt := factValue(v.facts, "ShortTermBorrowings") + factValue(v.facts, "LongTermDebtCurrent")
		longCurrent, hasCurrent := v.facts["LongTermDebtCurrent"]
		longNoncurrent, hasNoncurrent := v.facts["LongTermDebtNoncurrent"]
		debt := factValue(v.facts, "ShortTermBorrowings")
		if hasCurrent && hasNoncurrent {
			debt += longCurrent.value + longNoncurrent.value
		} else if longDebt, ok := v.facts["LongTermDebt"]; ok {
			debt += longDebt.value
		} else {
			debt += factValue(v.facts, "LongTermDebtCurrent") + factValue(v.facts, "LongTermDebtNoncurrent")
		}
		out[end] = balanceSnapshot{
			cash: cash.value,
			debt: debt,
			nwc:  assets.value - cash.value - (liabilities.value - shortDebt),
		}
	}
	return out
}

func factValue(facts map[string]storedFact, concept string) float64 {
	if f, ok := facts[concept]; ok {
		return f.value
	}
	return 0
}

func latestDilutedShares(facts []storedFact) (float64, string) {
	var best storedFact
	for _, f := range facts {
		if f.concept != "WeightedAverageNumberOfDilutedSharesOutstanding" || f.unit != "shares" || f.start == "" {
			continue
		}
		days := durationDays(f.start, f.end)
		if days < 60 || days > 120 {
			continue
		}
		if f.end > best.end || (f.end == best.end && f.filed > best.filed) {
			best = f
		}
	}
	if best.end != "" {
		return best.value, best.end
	}
	for _, f := range facts {
		if f.concept != "WeightedAverageNumberOfDilutedSharesOutstanding" || f.unit != "shares" || !annualDuration(f.start, f.end) {
			continue
		}
		if f.end > best.end || (f.end == best.end && f.filed > best.filed) {
			best = f
		}
	}
	return best.value, best.end
}

func annualForm(form string) bool { return form == "10-K" || form == "20-F" || form == "40-F" }
func annualDuration(start, end string) bool {
	days := durationDays(start, end)
	return days >= 330 && days <= 400
}
func durationDays(start, end string) int {
	a, err1 := time.Parse("2006-01-02", start)
	b, err2 := time.Parse("2006-01-02", end)
	if err1 != nil || err2 != nil || !b.After(a) {
		return 0
	}
	return int(b.Sub(a).Hours() / 24)
}
func revenuePriority(concept string) int {
	switch concept {
	case "RevenueFromContractWithCustomerExcludingAssessedTax":
		return 0
	case "Revenues":
		return 1
	default:
		return 2
	}
}
func mathAbs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// Keep the selected revenue concept priority alongside the value.
func (a *annualFlows) setRevenue(concept string, value float64) {
	priority := revenuePriority(concept)
	if !a.hasRevenue || priority < a.revenuePriority {
		a.revenue, a.revenuePriority, a.hasRevenue = value, priority, true
	}
}
