-- Apply with: psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/001_dcf.sql

CREATE TABLE dcf_models (
    id                      bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    valuation_date          date NOT NULL,
    initial_revenue         double precision NOT NULL CHECK (initial_revenue > 0),
    initial_operating_nwc_rate double precision NOT NULL,
    wacc                    double precision CHECK (wacc > 0),

    risk_free_rate          double precision,
    levered_beta            double precision,
    equity_risk_premium     double precision,
    pre_tax_cost_of_debt    double precision,
    capm_tax_rate           double precision,
    market_equity_value     double precision,
    market_debt_value       double precision,

    terminal_growth_rate    double precision NOT NULL,
    stable_roic             double precision NOT NULL CHECK (stable_roic > 0),

    excess_cash             double precision NOT NULL DEFAULT 0,
    non_operating_assets    double precision NOT NULL DEFAULT 0,
    debt                    double precision NOT NULL DEFAULT 0,
    other_senior_claims     double precision NOT NULL DEFAULT 0,
    diluted_shares          double precision NOT NULL DEFAULT 0 CHECK (diluted_shares >= 0),

    CHECK (
        (risk_free_rate IS NULL AND levered_beta IS NULL AND equity_risk_premium IS NULL
         AND pre_tax_cost_of_debt IS NULL AND capm_tax_rate IS NULL
         AND market_equity_value IS NULL AND market_debt_value IS NULL)
        OR
        (risk_free_rate IS NOT NULL AND levered_beta IS NOT NULL AND equity_risk_premium IS NOT NULL
         AND pre_tax_cost_of_debt IS NOT NULL AND capm_tax_rate IS NOT NULL
         AND market_equity_value IS NOT NULL AND market_debt_value IS NOT NULL)
    ),
    CHECK (wacc IS NOT NULL OR risk_free_rate IS NOT NULL)
);

CREATE TABLE dcf_forecast_years (
    model_id                        bigint NOT NULL REFERENCES dcf_models(id) ON DELETE CASCADE,
    forecast_index                  integer NOT NULL CHECK (forecast_index > 0),
    cash_flow_date                  date,
    revenue_growth                  double precision NOT NULL,
    ebit_margin                     double precision NOT NULL,
    cash_tax_rate                   double precision NOT NULL CHECK (cash_tax_rate BETWEEN 0 AND 1),
    da_as_pct_revenue               double precision NOT NULL CHECK (da_as_pct_revenue >= 0),
    capex_as_pct_revenue            double precision NOT NULL CHECK (capex_as_pct_revenue >= 0),
    operating_nwc_as_pct_revenue    double precision NOT NULL,
    PRIMARY KEY (model_id, forecast_index)
);

CREATE TABLE dcf_valuations (
    id                      bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    model_id                bigint NOT NULL REFERENCES dcf_models(id),
    created_at              timestamptz NOT NULL DEFAULT now(),
    wacc                    double precision NOT NULL,
    cost_of_equity          double precision,
    terminal_date           date NOT NULL,
    next_nopat              double precision NOT NULL,
    terminal_reinvestment   double precision NOT NULL,
    terminal_fcff           double precision NOT NULL,
    terminal_value          double precision NOT NULL,
    pv_explicit_fcff        double precision NOT NULL,
    pv_terminal_value       double precision NOT NULL,
    enterprise_value        double precision NOT NULL,
    equity_value             double precision NOT NULL,
    value_per_share         double precision
);

CREATE TABLE dcf_valuation_years (
    valuation_id            bigint NOT NULL REFERENCES dcf_valuations(id) ON DELETE CASCADE,
    forecast_index          integer NOT NULL,
    cash_flow_date          date NOT NULL,
    years_from_valuation    double precision NOT NULL,
    revenue                 double precision NOT NULL,
    ebit                    double precision NOT NULL,
    nopat                   double precision NOT NULL,
    da                      double precision NOT NULL,
    capex                   double precision NOT NULL,
    change_nwc              double precision NOT NULL,
    fcff                    double precision NOT NULL,
    present_value           double precision NOT NULL,
    PRIMARY KEY (valuation_id, forecast_index)
);
