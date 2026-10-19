package sec

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

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
		Label       string                 `json:"label"`
		Description string                 `json:"description"`
		Units       map[string][]FactEntry `json:"units"`
	} `json:"facts"`
}

type FinancialFact struct {
	Company    string  `json:"company"`
	CIK        string  `json:"cik"`
	Concept    string  `json:"concept"`
	Label      string  `json:"label"`
	Unit       string  `json:"unit"`
	Value      float64 `json:"value"`
	Start      string  `json:"period_start,omitempty"`
	End        string  `json:"period_end"`
	FilingDate string  `json:"filing_date"`
	Form       string  `json:"form"`
	Accession  string  `json:"accession_number"`
	Frame      string  `json:"frame,omitempty"`
}

var validCIK = regexp.MustCompile(`^[0-9]{1,10}$`)

func NormaliseCIK(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !validCIK.MatchString(raw) {
		return "", fmt.Errorf("invalid CIK %q", raw)
	}
	return strings.Repeat("0", 10-len(raw)) + raw, nil
}

var concepts = []string{
	"RevenueFromContractWithCustomerExcludingAssessedTax",
	"Revenues",
	"SalesRevenueNet",
	"AssetsCurrent",
	"LiabilitiesCurrent",
	"CashAndCashEquivalentsAtCarryingValue",
	"ShortTermBorrowings",
	"LongTermDebtCurrent",
	"LongTermDebtNoncurrent",
	"LongTermDebt",
	"OperatingIncomeLoss",
	"IncomeTaxExpenseBenefit",
	"NetIncomeLoss",
	"DepreciationDepletionAndAmortization",
	"PaymentsToAcquirePropertyPlantAndEquipment",
	"NetCashProvidedByUsedInOperatingActivities",
	"StockholdersEquity",
	"EntityCommonStockSharesOutstanding",
	"WeightedAverageNumberOfDilutedSharesOutstanding",
}

func ExtractFacts(data *CompanyFacts, cik string) []FinancialFact {
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
				if !dcfFilingForm(e.Form) {
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

	return out
}

func dcfFilingForm(form string) bool {
	switch form {
	case "10-K", "10-Q", "20-F", "40-F", "6-K":
		return true
	default:
		return false
	}
}
