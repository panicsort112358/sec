-- Apply with:
-- psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/002_sec.sql

CREATE TABLE sec_companies (
    cik                 char(10) PRIMARY KEY,
    company_name        text NOT NULL,
    requested_ticker    text NOT NULL,
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sec_filings (
    cik                 char(10) NOT NULL REFERENCES sec_companies(cik) ON DELETE CASCADE,
    accession_number    text NOT NULL,
    company_name        text NOT NULL,
    form                text NOT NULL,
    filing_date         date NOT NULL,
    report_date         date,
    document_url        text NOT NULL,
    saved_at            timestamptz NOT NULL DEFAULT now(),
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
    saved_at            timestamptz NOT NULL DEFAULT now(),
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
    fetched_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (cik, accession_number),
    FOREIGN KEY (cik, accession_number)
        REFERENCES sec_filings(cik, accession_number) ON DELETE CASCADE
);
