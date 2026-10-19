# sec

## Build and install

Build the two independent executables:

```sh
go build -o sec ./cmd/sec
go build -o dcf ./cmd/dcf
```

Both commands use the `sqlite3` command-line program, which must be on `PATH`.

Before using `sec`, set `SEC_USER_AGENT` to a descriptive agent string with a
contact email, or pass the same value with `-ua`.

`sec` stores SEC data in `sec.db` by default. Choose another file with
`-db path/to/file.db` or `SEC_DATABASE_PATH`.

Initialize the SEC and DCF tables in the same SQLite file before first use:

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
models stored in `dcf_models` and `dcf_forecast_years`.

Create DCF assumptions and forecast rows in `dcf_models` and
`dcf_forecast_years` using SQLite. Set `dcf_models.ticker` to associate each
model with its company; multiple models can represent different scenarios for
the same ticker.

Run valuations for every stored model and append them to that same database:

```sh
./dcf --ticker AAPL
```

`dcf` uses the same `sec.db` default as `sec`. Set `SEC_DATABASE_PATH` or
pass `-db path/to/file.db` to use another database.

The command prints one summary per successful valuation and exits nonzero if
any model fails. Rates are decimals, so `0.10` is 10%. Models accept either a
supplied WACC or CAPM inputs. Market assumptions such as beta, risk-free rate,
and equity risk premium must be supplied separately.

The schema is in `migrations/schema.sql`.
For the DCF equations and assumptions, see [How the DCF model works](DCF.md).
