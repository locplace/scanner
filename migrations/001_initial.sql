-- DNS LOC Scanner Schema
-- Migration 001: Initial schema

-- Root domains to be scanned
CREATE TABLE root_domains (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain              TEXT NOT NULL UNIQUE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_scanned_at     TIMESTAMPTZ,
    subdomains_scanned  BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX idx_root_domains_last_scanned ON root_domains(last_scanned_at NULLS FIRST);

-- Registered scanner clients
CREATE TABLE scanner_clients (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    token_hash      TEXT NOT NULL UNIQUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_heartbeat  TIMESTAMPTZ
);

-- Active assignments (which domains are being scanned by which client)
CREATE TABLE active_scans (
    root_domain_id  UUID PRIMARY KEY REFERENCES root_domains(id) ON DELETE CASCADE,
    client_id       UUID NOT NULL REFERENCES scanner_clients(id) ON DELETE CASCADE,
    assigned_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_active_scans_client ON active_scans(client_id);
CREATE INDEX idx_active_scans_assigned ON active_scans(assigned_at);

-- Discovered LOC records (stores all RFC 1876 fields)
CREATE TABLE loc_records (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    root_domain_id  UUID NOT NULL REFERENCES root_domains(id) ON DELETE CASCADE,
    fqdn            TEXT NOT NULL UNIQUE,

    -- Raw LOC record text (human-readable format)
    raw_record      TEXT NOT NULL,

    -- Parsed coordinates (converted to decimal degrees)
    latitude        DOUBLE PRECISION NOT NULL,
    longitude       DOUBLE PRECISION NOT NULL,
    altitude_m      DOUBLE PRECISION NOT NULL,

    -- Precision fields (in meters)
    size_m          DOUBLE PRECISION NOT NULL,
    horiz_prec_m    DOUBLE PRECISION NOT NULL,
    vert_prec_m     DOUBLE PRECISION NOT NULL,

    first_seen_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_loc_records_root_domain ON loc_records(root_domain_id);
CREATE INDEX idx_loc_records_coords ON loc_records(latitude, longitude);
