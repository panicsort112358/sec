# sec

## Build and install

Build the two independent executables in the repository directory. The Go
programs use only the standard library; both call the PostgreSQL `psql`
command-line client at runtime.

```sh
go build -o sec ./cmd/sec
go build -o dcf ./cmd/dcf
```

Install them into your Go binary directory (usually `$(go env GOPATH)/bin`):

```sh
go install ./cmd/sec ./cmd/dcf
```

Make sure that directory is on your `PATH` to run either command from anywhere.

Before using `sec`, set `SEC_USER_AGENT` to a descriptive agent string with a
contact email, or pass the same value with `-ua`.

Configure `DATABASE_URL`, or the standard `PGHOST`, `PGPORT`, `PGUSER`,
`PGDATABASE`, and `PGPASSWORD` variables for `psql`. Both commands also accept
a connection string with `-dsn`.

Fetch and save recent filings:

```sh
./sec -ticker AAPL -mode filings -form 10-K
```

Fetch and save the text of a filing, optionally selecting it by accession:

```sh
./sec -ticker AAPL -mode document -accession 0000320193-24-000123
```

Fetch and save extracted XBRL facts:

```sh
./sec -ticker AAPL -mode facts -form 10-K
```

`sec` stores company records, filings, facts, and document text in PostgreSQL
instead of writing JSON or filing text to stdout. It prints a short save
summary. Apply the SEC schema migration once:

```sh
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/002_sec.sql
```

Calculate a DCF with the separate `dcf` executable. It loads assumptions and
forecast rows from PostgreSQL and stores each valuation run there. Rates are
decimals, so `0.10` is 10%. The model accepts either a supplied WACC or CAPM
inputs. SEC data can inform historical inputs, but market assumptions such as
beta, risk-free rate and equity risk premium must be supplied separately.

For the model's equations, assumptions, and interpretation, see [How the DCF
model works](DCF.md).

Apply the DCF schema migration once:

```sh
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/001_dcf.sql
```

Create a model row and its forecast rows in `dcf_models` and
`dcf_forecast_years`, then run it by its returned ID:

```sh
dcf -model-id 42
```

[`examples/create_dcf_model.sql`](examples/create_dcf_model.sql) inserts a
sample model and forecast, then prints the generated model ID.

The schemas are in
[`migrations/001_dcf.sql`](migrations/001_dcf.sql) and
[`migrations/002_sec.sql`](migrations/002_sec.sql).

Forecast dates default to annual anniversaries of the valuation date; set a
row's `cash_flow_date` to use a different date. The engine discounts using
actual elapsed days, calculates FCFF from EBIT, cash taxes, D&A, CapEx and the
change in operating NWC, and derives terminal reinvestment from stable ROIC.
All monetary amounts and share counts must use consistent units. A zero
`diluted_shares` omits per-share value. Each run is saved in `dcf_valuations`
and `dcf_valuation_years`.
