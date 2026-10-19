# How the DCF model works

A discounted cash flow model asks a simple question: **what are the cash flows
of the business worth today?** It answers by forecasting cash available to all
capital providers, then discounting those future amounts for the time and risk
involved in receiving them.

This repository implements an assumptions-driven enterprise DCF in
[`internal/dcf/model.go`](internal/dcf/model.go). It keeps the financial
calculation separate from SEC data retrieval: reported facts can inform the
starting point and assumptions, while the user supplies the forecast and
market-based inputs.

## 1. From operations to free cash flow

The model projects revenue one forecast step at a time:

\[
R_t = R_{t-1}(1+g_t)
\]

It estimates operating profit using an EBIT margin, then subtracts cash taxes
to get net operating profit after tax (NOPAT):

\[
EBIT_t = R_t m_t
\qquad\qquad
NOPAT_t = EBIT_t(1-T_t)
\]

NOPAT is an after-tax measure of operating earnings. It excludes interest
because the model values the business before deciding how it is financed.

The model converts NOPAT into free cash flow to the firm (FCFF):

\[
FCFF_t = NOPAT_t + D\&A_t - CapEx_t - \Delta NWC_t
\]

Depreciation and amortization are added back because they reduce accounting
profit without being a current-period cash payment. Capital expenditure is
subtracted because it is cash invested in long-lived assets. An increase in
operating net working capital also consumes cash; a decrease releases it.

For each forecast step, D&A, CapEx, and operating NWC are modeled as a
percentage of revenue. The change in working capital is measured against the
prior period. The first step uses `initial_operating_nwc_as_pct_revenue` to
estimate that prior-period balance.

This is a compact operating model. It does not separately forecast receivables,
inventory, payables, asset vintages, or segment economics; those could replace
the revenue ratios as the model becomes more detailed.

## 2. Discounting cash flows to today

Cash received in the future is worth less than the same amount received today.
The enterprise value is the present value of forecast FCFF plus the present
value of the terminal value:

\[
EV_0 = \sum_{t=1}^{N}\frac{FCFF_t}{(1+WACC)^{\tau_t}}
      + \frac{TV_N}{(1+WACC)^{\tau_N}}
\]

The discount rate is the weighted average cost of capital (WACC): the blended
return required by debt and equity investors. A higher WACC reduces the value
of future cash flows, reflecting greater risk or a higher required return.

The model accepts WACC directly, or derives it from CAPM and capital weights:

\[
R_e = R_f + \beta_L ERP
\]

\[
WACC = \frac{E}{D+E}R_e
     + \frac{D}{D+E}R_d(1-T)
\]

Here, CAPM estimates the cost of equity from a risk-free rate, levered beta,
and equity risk premium. The debt cost is reduced by the tax shield. The
market values of equity and debt determine their weights. These market inputs
are not established by SEC filings alone, so provide them explicitly when
using CAPM. A supplied `wacc` takes precedence over `capm`.

The exponent \(\tau_t\) is the elapsed time from the valuation date to the
cash-flow date, measured in years. This supports irregular dates rather than
assuming every cash flow arrives exactly at year-end. Each forecast row still
applies its growth and operating assumptions once; its date controls the
discounting interval.

## 3. A sustainable terminal value

An explicit forecast cannot reasonably continue forever. The model therefore
estimates value beyond its final forecast date with a growing perpetuity:

\[
TV_N = \frac{FCFF_{N+1}}{WACC-g}
\]

The terminal growth rate \(g\) must be below WACC, or the perpetuity does not
converge. Growth also requires reinvestment. The model links it to stable
return on invested capital (ROIC):

\[
Reinvestment_{N+1} = NOPAT_{N+1}\frac{g}{ROIC_{stable}}
\]

\[
FCFF_{N+1} = NOPAT_{N+1} - Reinvestment_{N+1}
\]

This follows the sustainable-growth approximation \(g \approx ROIC \times
reinvestment\ rate\). For example, at 2.5% perpetual growth and 12% stable
ROIC, about 20.8% of terminal NOPAT must be reinvested. The rest is available
as FCFF. Requiring stable ROIC to be at least the growth rate keeps the
implied reinvestment rate at or below 100%.

The terminal value is placed at the date of the final forecast cash flow and
discounted back to the valuation date. The model uses one explicit forecast
stage followed directly by a stable state; it does not automatically fade
margins, growth, or ROIC through a transition period.

## 4. From enterprise value to equity value

Enterprise value belongs to all capital providers. To estimate the value
attributable to common shareholders, the model applies an equity bridge:

\[
Equity\ Value = Enterprise\ Value + Excess\ Cash + NonOperating\ Assets
              - Debt - Other\ Senior\ Claims
\]

The bridge adds assets not needed to generate the modeled operating cash
flows and subtracts claims senior to common equity. This classification must
be consistent with the cash flows: do not include an asset in operating value
and add it again here, or subtract the same liability twice.

When a diluted share count is supplied, the model calculates:

\[
Value\ Per\ Share = \frac{Equity\ Value}{Diluted\ Shares}
\]

Use a valuation-date diluted share count in consistent units. The engine does
not itself estimate dilution from options, awards, or convertibles.

## 5. Reading the output and choosing inputs

The result reports each forecast year's revenue, EBIT, NOPAT, D&A, CapEx,
change in working capital, FCFF, and present value. It also reports terminal
NOPAT, reinvestment, terminal FCFF and value, enterprise value, equity value,
and—when possible—value per share.

All rates are decimals (`0.10` means 10%). Monetary values and share counts
must use consistent units. The model is only as credible as its assumptions:
use filings to understand historical performance and balance-sheet needs,
then make explicit judgments about future growth, margins, reinvestment,
taxes, and risk. Because terminal value can be a large share of total value,
small changes to WACC, perpetual growth, or stable ROIC can materially change
the result.
