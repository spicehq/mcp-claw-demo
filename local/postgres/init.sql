-- Seed CRM data for the openclaw-mcp-demo local stack.
-- This script runs once on first container start (postgres-image convention).

CREATE TABLE IF NOT EXISTS public.accounts (
    account_id     TEXT        PRIMARY KEY,
    name           TEXT        NOT NULL,
    owner          TEXT        NOT NULL,
    plan           TEXT        NOT NULL,
    mrr            NUMERIC(12, 2) NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL,
    last_touch_at  TIMESTAMPTZ NOT NULL
);

INSERT INTO public.accounts (account_id, name, owner, plan, mrr, created_at, last_touch_at) VALUES
    ('acct_001', 'Northwind Trading',     'alice@acme.example',   'Enterprise', 48000.00, '2024-02-14T10:12:00Z', '2026-05-12T14:03:00Z'),
    ('acct_002', 'Contoso Logistics',     'alice@acme.example',   'Enterprise', 32500.00, '2024-04-30T09:00:00Z', '2026-05-15T11:22:00Z'),
    ('acct_003', 'Globex Robotics',       'bob@acme.example',     'Enterprise', 27000.00, '2024-06-02T08:45:00Z', '2026-05-10T16:30:00Z'),
    ('acct_004', 'Initech Holdings',      'bob@acme.example',     'Pro',         8400.00, '2024-09-18T13:20:00Z', '2026-05-17T09:11:00Z'),
    ('acct_005', 'Soylent Foods',         'carol@acme.example',   'Pro',         6900.00, '2024-11-01T15:00:00Z', '2026-05-16T17:55:00Z'),
    ('acct_006', 'Umbrella Biotech',      'carol@acme.example',   'Enterprise', 51200.00, '2023-08-20T11:30:00Z', '2026-05-18T08:40:00Z'),
    ('acct_007', 'Acme Subsidiary EMEA',  'dave@acme.example',    'Pro',         9800.00, '2025-01-09T07:15:00Z', '2026-05-13T13:05:00Z'),
    ('acct_008', 'Stark Industries',      'dave@acme.example',    'Enterprise', 64000.00, '2023-03-11T10:00:00Z', '2026-05-18T19:45:00Z'),
    ('acct_009', 'Wonka Confections',     'erin@acme.example',    'Starter',     1200.00, '2025-12-04T12:00:00Z', '2026-05-11T10:20:00Z'),
    ('acct_010', 'Hooli Cloud',           'erin@acme.example',    'Starter',      900.00, '2026-02-22T16:30:00Z', '2026-05-17T15:10:00Z'),
    ('acct_011', 'Pied Piper Compression','frank@acme.example',   'Pro',         7200.00, '2025-07-08T14:00:00Z', '2026-05-14T12:00:00Z'),
    ('acct_012', 'Cyberdyne Systems',     'frank@acme.example',   'Enterprise', 45000.00, '2024-01-05T11:00:00Z', '2026-05-18T07:35:00Z');

CREATE INDEX accounts_plan_idx  ON public.accounts (plan);
CREATE INDEX accounts_owner_idx ON public.accounts (owner);

-- Read-only role for Spice. Matches the CRM_PG_USER/CRM_PG_PASS in .env.example.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'spice_ro') THEN
        CREATE ROLE spice_ro LOGIN PASSWORD 'spice_ro';
    END IF;
END
$$;

GRANT CONNECT ON DATABASE crm        TO spice_ro;
GRANT USAGE   ON SCHEMA   public     TO spice_ro;
GRANT SELECT  ON public.accounts     TO spice_ro;
