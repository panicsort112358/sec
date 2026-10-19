-- Run with: psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f examples/create_dcf_model.sql

INSERT INTO dcf_models (
    valuation_date,
    initial_revenue,
    initial_operating_nwc_rate,
    wacc,
    terminal_growth_rate,
    stable_roic,
    excess_cash,
    debt,
    diluted_shares
) VALUES (
    DATE '2025-12-31',
    100000,
    0.08,
    0.09,
    0.025,
    0.12,
    10000,
    20000,
    1000
)
RETURNING id AS model_id \gset

INSERT INTO dcf_forecast_years (
    model_id, forecast_index, cash_flow_date, revenue_growth, ebit_margin,
    cash_tax_rate, da_as_pct_revenue, capex_as_pct_revenue,
    operating_nwc_as_pct_revenue
) VALUES
    (:model_id, 1, DATE '2026-12-31', 0.08, 0.22, 0.21, 0.04, 0.05, 0.09),
    (:model_id, 2, DATE '2027-12-31', 0.07, 0.23, 0.21, 0.04, 0.045, 0.09),
    (:model_id, 3, DATE '2028-12-31', 0.06, 0.24, 0.21, 0.04, 0.04, 0.09);

\echo Created DCF model :model_id
