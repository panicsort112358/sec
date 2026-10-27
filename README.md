# sec

## Build and install

Build the two independent executables:

```sh
go build -o sec ./cmd/sec
go build -o dcf ./cmd/dcf
```

Both commands use the SQLite driver bundled with the Go program; they do not
need the SQLite command-line tool. The command below uses `sqlite3` to apply
the schema once.

Before using `sec`, set `SEC_USER_AGENT` to a descriptive agent string with a
contact email, or pass the same value with `-ua`.

`sec` stores SEC data in `sec.db` by default. Choose another file with
`-db path/to/file.db` or `SEC_DATABASE_PATH`.

Initialise the SEC and DCF tables in the same SQLite file before first use:

```sh
sqlite3 sec.db < migrations/schema.sql
```

Fetch company facts relevant to building a DCF:

```sh
./sec -ticker AAPL
```

The command saves selected historical XBRL facts such as revenue, operating
income, cash flow, working-capital balances, debt, cash, and share counts. It
does not infer forecasts or market assumptions; those remain inputs to the DCF
models stored in `dcf_models` and `dcf_forecast_years`. Reverse DCF can solve
for the FCFF growth implied by a market price under explicit discount-rate and
terminal-growth assumptions, as described below.

Inspect the saved facts with the `sqlite3` command-line tool:

```sh
sqlite3 sec.db
```

At the `sqlite>` prompt, list tables, count AAPL facts, and view a few rows:

```sql
.tables
SELECT COUNT(*) FROM sec_facts WHERE ticker = 'AAPL';
SELECT concept, label, unit, value, period_start, period_end
FROM sec_facts
WHERE ticker = 'AAPL'
ORDER BY concept, period_end DESC
LIMIT 8;
```

For example, the count query may return `2585`; the sample rows will look like:

```text
AssetsCurrent|Assets, Current|USD|149818000000.0||2026-06-27
AssetsCurrent|Assets, Current|USD|144114000000.0||2026-03-28
AssetsCurrent|Assets, Current|USD|158104000000.0||2025-12-27
```

The exact count and rows depend on the SEC facts currently saved in your
database. To see the available concepts and the number of rows for each:

```sql
SELECT concept, label, COUNT(*) AS rows
FROM sec_facts
WHERE ticker = 'AAPL'
GROUP BY concept, label
ORDER BY concept;
```

Type `.quit` to leave the SQLite prompt.

## Reverse DCF

Estimate the constant annual free-cash-flow-to-the-firm (FCFF) growth implied
by a share price, using the facts saved for a ticker:

```sh
./dcf -ticker AAPL -price 250
```

The price is supplied by you; the SEC facts do not include a current market
price. The command uses up to the three most recent completed annual filing
periods to calculate normalized FCFF, then solves for the constant FCFF growth
rate over five forecast years that matches the price. It prints the result
across WACCs from 8% to 12% in 1% steps, assuming 2.5% terminal growth.

Refresh the saved facts with the current `sec` executable first, especially
after updating it to fetch the reported pre-tax income fact used for a more
direct tax-rate estimate:

```sh
go build -o sec ./cmd/sec
./sec -ticker AAPL -ua 'Research App contact@example.com'
```

Change the valuation date, forecast horizon, WACC range, step, or terminal
growth with flags:

```sh
./dcf -ticker AAPL -price 250 -as-of 2026-10-27 -years 5 \
  -wacc-min 0.08 -wacc-max 0.12 -wacc-step 0.01 -terminal-growth 0.025
```

Amounts are taken from SEC facts in USD, and the price is in USD per share.
The working-capital change is approximated using current assets less cash and
current liabilities excluding current debt. The share count uses the latest
reported quarterly weighted-average diluted shares when available, otherwise
the latest annual figure. The terminal value uses the supplied terminal
growth rate; results vary materially with WACC and terminal assumptions. This
is an implied-growth calculation, not a forecast of future stock returns.

Create DCF assumptions and forecast rows in `dcf_models` and
`dcf_forecast_years` using SQLite. Set `dcf_models.ticker` to associate each
model with its company; multiple models can represent different scenarios for
the same ticker.

Run valuations for every stored model and append them to that same database:

```sh
./dcf -ticker AAPL
```

`dcf` uses the same `sec.db` default as `sec`. Set `SEC_DATABASE_PATH` or
pass `-db path/to/file.db` to use another database.

The command prints one summary per successful valuation and exits nonzero if
any model fails. Rates are decimals, so `0.10` is 10%. Models accept either a
supplied WACC or CAPM inputs. Market assumptions such as beta, risk-free rate,
and equity risk premium must be supplied separately.

The schema is in `migrations/schema.sql`.
For the DCF equations and assumptions, see [How the DCF model works](DCF.md).
