# DNS LOC Record Scanner - Implementation Plan (Revised)

## Overview

A distributed system to scan domains for DNS LOC (Location) records, consisting of:
1. **Coordination Server** - PostgreSQL-backed API server managing jobs and storing results
2. **Scanner Workers** - Distributed workers that discover subdomains and scan for LOC records

---

## Research Findings

### RFC 1876 LOC Record Format

LOC records have **7 fields** (not just lat/long/altitude):

| Field | Size | Description |
|-------|------|-------------|
| VERSION | 8 bits | Protocol version (must be 0) |
| SIZE | 8 bits | Diameter of sphere enclosing the entity (XeY cm encoding) |
| HORIZ_PRE | 8 bits | Horizontal precision (XeY cm encoding) |
| VERT_PRE | 8 bits | Vertical precision (XeY cm encoding) |
| LATITUDE | 32 bits | Milliseconds of arc, 2^31 = equator |
| LONGITUDE | 32 bits | Milliseconds of arc, 2^31 = prime meridian |
| ALTITUDE | 32 bits | Centimeters, base = 100km below WGS84 |

**Text format:** `42 21 54.000 N 71 06 18.000 W -24.00m 30m 10000m 10m`
- Degrees minutes seconds for lat/long
- Altitude in meters (can be negative)
- Size, horizontal precision, vertical precision in meters

**XeY encoding:** Upper 4 bits = mantissa (0-9), lower 4 bits = exponent (0-9)
- `0x12` = 1×10² = 100 cm = 1m
- `0x16` = 1×10⁶ = 10km

### ZDNS Library API

```go
import (
    "github.com/zmap/zdns/src/zdns"
    "github.com/miekg/dns"
)

// Each resolver handles ONE lookup at a time (not thread-safe)
// Create multiple resolvers for concurrent operations
config := zdns.NewResolverConfig()
config.ExternalNameServersV4 = []zdns.NameServer{{IP: net.ParseIP("8.8.8.8"), Port: 53}}
resolver, _ := zdns.InitResolver(config)
defer resolver.Close()

question := &zdns.Question{Type: dns.TypeLOC, Class: dns.ClassINET, Name: "example.com"}
result, _, status, err := resolver.ExternalLookup(ctx, question, nil)

// LOCAnswer contains all RFC 1876 fields
type LOCAnswer struct {
    Version     uint8   // Always 0
    Size        uint8   // XeY encoded
    HorizPre    uint8   // XeY encoded
    VertPre     uint8   // XeY encoded
    Latitude    uint32  // Raw wire format
    Longitude   uint32  // Raw wire format
    Altitude    uint32  // Raw wire format
    Coordinates string  // Human-readable: "42 21 54.000 N 71 06 18.000 W -24.00m 1m 10000m 10m"
}
```

### Subfinder Library API

```go
import "github.com/projectdiscovery/subfinder/v2/pkg/runner"

opts := &runner.Options{
    Sources: []string{"crtsh", "hackertarget", "rapiddns", "waybackarchive"}, // Free sources
    Threads: 10,
    Timeout: 30,
    MaxEnumerationTime: 5,
}
subfinder, _ := runner.NewRunner(opts)

var buf bytes.Buffer
sourceMap, _ := subfinder.EnumerateSingleDomainWithCtx(ctx, "example.com", []io.Writer{&buf})
// buf contains newline-separated subdomains
// sourceMap has source -> count mapping
```

**Free sources (no API keys):** crtsh, hackertarget, rapiddns, waybackarchive, dnsdumpster,
alienvault, anubis, commoncrawl, digitorus, threatminer

---

## Architecture Decisions

### Q5: Should heartbeat verify worker progress?

**Decision: No.** The heartbeat just confirms the scanner process is alive. If a worker is stuck on a slow domain (e.g., thousands of subdomains), that's expected behavior, not a failure. The job timeout (configurable, default 10min) handles truly stuck jobs. If a scanner dies completely, heartbeat stops and jobs are released.

### Q6: Job-ID vs per-node granularity?

**Decision: Per-node with domain tracking.** Simplify by removing explicit job IDs:

1. Scanner requests N domains
2. Server marks those domains as "assigned to scanner X at time T"
3. Scanner submits results referencing domains by name
4. Server releases domains and records results
5. If scanner dies (heartbeat timeout), all its assigned domains are released

This eliminates job ID bookkeeping. The `jobs` table just tracks `(client_id, root_domain_id, assigned_at)`.

### Q7: Scanner internal architecture?

**Decision: Independent workers with shared tracker.**

```
┌─────────────────────────────────────────────────────────────┐
│                      Scanner Process                         │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │         Heartbeat Goroutine (every 30s)                │ │
│  │   - Reads active domains from tracker                  │ │
│  │   - Sends heartbeat to coordinator                     │ │
│  └────────────────────────────────────────────────────────┘ │
│                                                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │
│  │ Worker 1 │  │ Worker 2 │  │ Worker 3 │  │ Worker N │    │
│  │          │  │          │  │          │  │          │    │
│  │ fetch    │  │ fetch    │  │ fetch    │  │ fetch    │    │
│  │ subfind  │  │ subfind  │  │ subfind  │  │ subfind  │    │
│  │ zdns     │  │ zdns     │  │ zdns     │  │ zdns     │    │
│  │ submit   │  │ submit   │  │ submit   │  │ submit   │    │
│  │ loop     │  │ loop     │  │ loop     │  │ loop     │    │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘    │
│       │             │             │             │           │
│       └─────────────┴──────┬──────┴─────────────┘           │
│                            │                                │
│  ┌─────────────────────────▼──────────────────────────────┐ │
│  │              Active Domain Tracker (sync.Map)          │ │
│  │   - Workers register/unregister domains they're on     │ │
│  │   - Heartbeat reads current set                        │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

Each worker is independent:
1. Requests a small batch from coordinator (batch size configurable, default 3)
2. Registers domains in tracker
3. Runs subfinder on each domain
4. Runs zdns LOC queries on root + all subdomains
5. Submits results to coordinator
6. Unregisters domains from tracker
7. Loops

No channels needed between workers. The only shared state is the tracker (for heartbeat) and the HTTP client (thread-safe).

### Q8: Lost job submission after heartbeat timeout?

**Decision: Accept idempotently.** If scanner A times out and scanner B picks up the domain, then A finally submits:
- Accept the submission (work was done, might as well use it)
- LOC records are upserted by FQDN (unique constraint), so duplicates just update `last_seen`
- Mark domain as scanned (idempotent)
- No harm, no special handling needed

### Q9: worker_count vs batch_size?

**Decision: Expose only `worker_count`, default batch_size internally.**

- `WORKER_COUNT` (default 4): Number of parallel workers
- Batch size fixed at 3 domains per worker per fetch (reasonable default)
- Power users can set `BATCH_SIZE` env var if they really want to tune it

The ratio doesn't matter much in practice - what matters is total parallelism (workers × domains × subdomains × DNS queries).

---

## Database Schema (Revised)

```sql
-- Root domains to be scanned
CREATE TABLE root_domains (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain              TEXT NOT NULL UNIQUE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_scanned_at     TIMESTAMPTZ,
    subdomains_scanned  BIGINT NOT NULL DEFAULT 0  -- Running total for stats
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

-- Discovered LOC records (stores all 7 RFC 1876 fields)
CREATE TABLE loc_records (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    root_domain_id  UUID NOT NULL REFERENCES root_domains(id) ON DELETE CASCADE,
    fqdn            TEXT NOT NULL UNIQUE,

    -- Raw LOC record text (human-readable format from zdns)
    raw_record      TEXT NOT NULL,

    -- Parsed coordinates (converted to decimal for querying)
    latitude        DOUBLE PRECISION NOT NULL,
    longitude       DOUBLE PRECISION NOT NULL,
    altitude_m      DOUBLE PRECISION NOT NULL,

    -- Precision fields (in meters, decoded from XeY)
    size_m          DOUBLE PRECISION NOT NULL,
    horiz_prec_m    DOUBLE PRECISION NOT NULL,
    vert_prec_m     DOUBLE PRECISION NOT NULL,

    first_seen_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_loc_records_root_domain ON loc_records(root_domain_id);
CREATE INDEX idx_loc_records_coords ON loc_records(latitude, longitude);
```

**Changes from v1:**
- Removed `scan_count` from root_domains
- Added `subdomains_scanned` counter to root_domains (for stats without storing each subdomain)
- All IDs are UUIDs
- Renamed `jobs` to `active_scans` (clearer intent, no job_id needed)
- LOC records now store all 7 fields: raw_record, lat, long, altitude, size, horiz_prec, vert_prec
- Precision values stored in meters (decoded from XeY format)

---

## API Specification (Revised)

### Admin Endpoints (X-Admin-Key header)

#### POST /api/admin/domains
```json
// Request
{"domains": ["example.com", "example.org"]}

// Response
{"inserted": 2, "duplicates": 0}
```

#### POST /api/admin/clients
```json
// Request
{"name": "scanner-us-east-1"}

// Response
{"id": "uuid", "name": "scanner-us-east-1", "token": "secret-token"}
```

#### GET /api/admin/clients
```json
// Response
{
  "clients": [{
    "id": "uuid",
    "name": "scanner-us-east-1",
    "created_at": "2025-01-15T10:00:00Z",
    "last_heartbeat": "2025-01-15T12:30:00Z",
    "active_domains": 5,
    "is_alive": true
  }]
}
```

#### DELETE /api/admin/clients/:id
Removes client and releases all its active scans.

---

### Scanner Endpoints (Authorization: Bearer <token>)

#### POST /api/scanner/jobs
Request domains to scan.

```json
// Request
{"count": 3}

// Response
{
  "domains": [
    {"domain": "example.com"},
    {"domain": "example.org"}
  ]
}
```

Server assigns domains with oldest `last_scanned_at` (NULL first) that aren't in `active_scans`.

#### POST /api/scanner/heartbeat
```json
// Request
{"active_domains": ["example.com", "example.org"]}

// Response
{"ok": true}
```

#### POST /api/scanner/results
```json
// Request
{
  "results": [{
    "domain": "example.com",
    "subdomains_scanned": 150,
    "loc_records": [{
      "fqdn": "server1.example.com",
      "raw_record": "52 22 23.000 N 4 53 32.000 E -2.00m 1m 10000m 10m",
      "latitude": 52.3730556,
      "longitude": 4.8922222,
      "altitude_m": -2.0,
      "size_m": 1.0,
      "horiz_prec_m": 10000.0,
      "vert_prec_m": 10.0
    }]
  }]
}

// Response
{"accepted": 1}
```

---

### Public Endpoints (no auth)

#### GET /api/public/records?limit=100&offset=0&domain=example.com
```json
{
  "records": [{
    "fqdn": "server1.example.com",
    "root_domain": "example.com",
    "raw_record": "52 22 23.000 N 4 53 32.000 E -2.00m 1m 10000m 10m",
    "latitude": 52.3730556,
    "longitude": 4.8922222,
    "altitude_m": -2.0,
    "size_m": 1.0,
    "horiz_prec_m": 10000.0,
    "vert_prec_m": 10.0,
    "first_seen_at": "2025-01-10T08:00:00Z",
    "last_seen_at": "2025-01-15T12:00:00Z"
  }],
  "total": 42,
  "limit": 100,
  "offset": 0
}
```

#### GET /api/public/stats
```json
{
  "total_root_domains": 1000,
  "scanned_root_domains": 850,
  "pending_root_domains": 150,
  "in_progress_root_domains": 12,
  "total_subdomains_scanned": 125000,
  "active_scanners": 5,
  "total_loc_records": 42,
  "unique_root_domains_with_loc": 15
}
```

---

## LOC Record Parsing

ZDNS returns `LOCAnswer.Coordinates` as a human-readable string like:
```
"52 22 23.000 N 4 53 32.000 E -2.00m 1m 10000m 10m"
```

We need to parse this to extract:
- Latitude/Longitude as decimal degrees
- Altitude, size, horiz_prec, vert_prec in meters

**Parser logic:**
```go
// Parse "52 22 23.000 N 4 53 32.000 E -2.00m 1m 10000m 10m"
// Format: d1 m1 s1 N/S d2 m2 s2 E/W alt size hp vp

func ParseLOCCoordinates(raw string) (*LOCData, error) {
    // Regex or sscanf to extract components
    // Convert DMS to decimal: degrees + minutes/60 + seconds/3600
    // Apply hemisphere sign (S/W = negative)
    // Strip 'm' suffix from altitude/size/precision values
}
```

ZDNS also provides raw uint8/uint32 fields, but the `Coordinates` string is easier to parse than decoding XeY format manually.

---

## Configuration

### Coordinator
```
DATABASE_URL=postgres://user:pass@localhost:5432/locscanner?sslmode=disable
ADMIN_API_KEY=secret-admin-key
LISTEN_ADDR=:8080
JOB_TIMEOUT=10m           # Configurable per user request
HEARTBEAT_TIMEOUT=2m
REAPER_INTERVAL=60s
```

### Scanner
```
COORDINATOR_URL=http://localhost:8080
SCANNER_TOKEN=client-token-from-registration
WORKER_COUNT=4            # Number of parallel workers
BATCH_SIZE=3              # Domains per worker per fetch (optional, default 3)
HEARTBEAT_INTERVAL=30s
```

---

## Project Structure

```
loc-scanner/
├── cmd/
│   ├── coordinator/
│   │   └── main.go
│   └── scanner/
│       └── main.go
├── internal/
│   ├── coordinator/
│   │   ├── server.go
│   │   ├── handlers/
│   │   │   ├── admin.go
│   │   │   ├── scanner.go
│   │   │   └── public.go
│   │   ├── middleware/
│   │   │   └── auth.go
│   │   ├── db/
│   │   │   ├── db.go
│   │   │   ├── domains.go
│   │   │   ├── clients.go
│   │   │   ├── scans.go
│   │   │   └── records.go
│   │   └── reaper/
│   │       └── reaper.go
│   └── scanner/
│       ├── scanner.go
│       ├── worker.go
│       ├── tracker.go
│       ├── coordinator.go    # API client
│       ├── subfinder.go
│       ├── dns.go            # zdns wrapper
│       └── loc.go            # LOC record parsing
├── pkg/
│   └── api/
│       └── types.go
├── migrations/
│   └── 001_initial.sql
├── go.mod
├── docker-compose.yml
├── Dockerfile.coordinator
└── Dockerfile.scanner
```

---

## Implementation Order

### Phase 1: Foundation
1. Initialize Go module with dependencies
2. Define shared API types in `pkg/api/types.go`
3. Create database migration
4. Implement LOC record parser

### Phase 2: Coordinator
5. Database connection and repositories
6. Authentication middleware
7. Admin handlers
8. Scanner handlers
9. Public handlers
10. Dead scan reaper
11. Wire up HTTP server

### Phase 3: Scanner
12. Coordinator API client
13. Active domain tracker
14. Subfinder wrapper
15. ZDNS wrapper
16. Worker implementation
17. Heartbeat goroutine
18. Main scanner orchestration

### Phase 4: Testing
19. Docker compose for local dev
20. Test with sample domains
21. Dockerfiles for deployment

---

## Dependencies

```go
// Coordinator
github.com/jackc/pgx/v5
github.com/go-chi/chi/v5
golang.org/x/crypto/sha256 // For token hashing

// Scanner
github.com/projectdiscovery/subfinder/v2
github.com/zmap/zdns
github.com/miekg/dns
```

---

## Test Domains

Known to have LOC records:
- alink.net
- caida.org
- chagas.eti.br
- ckdhr.com
- distributed.net (rc5stats.distributed.net)
- goldenglow.com.au (www.goldenglow.com.au)
- nikhef.nl
- vrx.net
- yahoo.com
