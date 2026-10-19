-- Apply with: sqlite3 sec.db < migrations/schema.sql

CREATE TABLE dcf_models (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    ticker                  text NOT NULL,
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
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    model_id                bigint NOT NULL REFERENCES dcf_models(id),
    created_at              text NOT NULL DEFAULT CURRENT_TIMESTAMP,
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
-- SEC tables

CREATE TABLE sec_companies (
    cik                 char(10) PRIMARY KEY,
    company_name        text NOT NULL,
    requested_ticker    text NOT NULL,
    updated_at          text NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE sec_filings (
    cik                 char(10) NOT NULL REFERENCES sec_companies(cik) ON DELETE CASCADE,
    accession_number    text NOT NULL,
    company_name        text NOT NULL,
    form                text NOT NULL,
    filing_date         date NOT NULL,
    report_date         date,
    document_url        text NOT NULL,
    saved_at            text NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (cik, accession_number)
);

CREATE INDEX sec_filings_by_date ON sec_filings (cik, filing_date DESC);

CREATE TABLE sec_facts (
    cik                 char(10) NOT NULL REFERENCES sec_companies(cik) ON DELETE CASCADE,
    company_name        text NOT NULL,
    ticker              text NOT NULL,
    concept             text NOT NULL,
    label               text NOT NULL,
    unit                text NOT NULL,
    value               double precision NOT NULL,
    period_start        text NOT NULL DEFAULT '',
    period_end          text NOT NULL,
    filing_date         text NOT NULL,
    form                text NOT NULL,
    accession_number    text NOT NULL,
    frame               text NOT NULL DEFAULT '',
    saved_at            text NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (
        cik, concept, unit, accession_number, period_start, period_end,
        form, filing_date, frame
    )
);

CREATE INDEX sec_facts_by_concept_period ON sec_facts (cik, concept, period_end DESC);

CREATE TABLE sec_documents (
    cik                 char(10) NOT NULL,
    accession_number    text NOT NULL,
    document_text       text NOT NULL,
    fetched_at          text NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (cik, accession_number),
    FOREIGN KEY (cik, accession_number)
        REFERENCES sec_filings(cik, accession_number) ON DELETE CASCADE
);
