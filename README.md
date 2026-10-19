# sec

## Build and install

Build the executable in the repository directory:

```sh
go build -o sec ./cmd/sec
```

Install it into your Go binary directory (usually `$(go env GOPATH)/bin`):

```sh
go install ./cmd/sec
```

Make sure that directory is on your `PATH` to run `sec` from anywhere.

Set `SEC_USER_AGENT` to a descriptive agent string with a contact email before
running commands, or pass the same value with `-ua`.

Read a filing as prose:

```sh
./sec -ticker AAPL -mode document -form 10-K
```

Choose a filing by accession number:

```sh
./sec -ticker AAPL -mode document -accession 0000320193-24-000123
```

Read extracted XBRL facts as JSON:

```sh
./sec -ticker AAPL -mode facts -form 10-K
```

Calculate a DCF from explicit assumptions (rates are decimals, so `0.10` is
10%). The model accepts either a supplied WACC or CAPM inputs. SEC data can
inform the historical inputs, but market assumptions such as beta, risk-free
rate and equity risk premium must be supplied separately.

For the model's equations, assumptions, and interpretation, see [How the DCF
model works](DCF.md).

```sh
./sec -mode dcf -input model.json
```

Example `model.json`:

```json
{
  "valuation_date": "2025-12-31",
  "initial_revenue": 100000,
  "initial_operating_nwc_as_pct_revenue": 0.08,
  "wacc": 0.09,
  "forecast": [
    {"revenue_growth": 0.08, "ebit_margin": 0.22, "cash_tax_rate": 0.21, "da_as_pct_revenue": 0.04, "capex_as_pct_revenue": 0.05, "operating_nwc_as_pct_revenue": 0.09},
    {"revenue_growth": 0.07, "ebit_margin": 0.23, "cash_tax_rate": 0.21, "da_as_pct_revenue": 0.04, "capex_as_pct_revenue": 0.045, "operating_nwc_as_pct_revenue": 0.09},
    {"revenue_growth": 0.06, "ebit_margin": 0.24, "cash_tax_rate": 0.21, "da_as_pct_revenue": 0.04, "capex_as_pct_revenue": 0.04, "operating_nwc_as_pct_revenue": 0.09}
  ],
  "terminal": {"growth_rate": 0.025, "stable_roic": 0.12},
  "equity_bridge": {"excess_cash": 10000, "non_operating_assets": 0, "debt": 20000, "other_senior_claims": 0, "diluted_shares": 1000}
}
```

Forecast dates default to annual anniversaries of the valuation date; set a
year's `date` to use a different cash-flow date. The engine discounts using
actual elapsed days, calculates FCFF from EBIT, cash taxes, D&A, CapEx and the
change in operating NWC, and derives terminal reinvestment from stable ROIC.
All monetary amounts and share counts must use consistent units. A zero
`diluted_shares` omits per-share value.

Set `SEC_USER_AGENT` to a descriptive agent string with a contact email, or
pass `-ua 'Research App contact@example.com'`.
