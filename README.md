# DNS LOC Record Scanner

A distributed system to scan domains for DNS LOC (Location) records as defined in RFC 1876.

## Architecture

The scanner uses a batch-based architecture to efficiently scan the entire internet for LOC records:

1. **Domain Source**: Uses the [tb0hdan/domains](https://github.com/tb0hdan/domains) project which maintains ~1.7 billion FQDNs
2. **Batch Queue**: PostgreSQL-backed queue with `FOR UPDATE SKIP LOCKED` for efficient work distribution
3. **In-Memory Processing**: XZ-compressed domain files are downloaded and decompressed in memory (no disk I/O)
4. **Distributed Scanning**: Multiple scanner workers can claim batches and process them in parallel

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Coordination Server                              │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌────────────┐     │
│  │ Admin API   │  │ Scanner API │  │ Public API  │  │  Feeder    │     │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └─────┬──────┘     │
│         └────────────────┼────────────────┼────────────────┘            │
│                   ┌──────▼──────┐                                       │
│                   │  PostgreSQL │  ◄── domain_files, scan_batches       │
│                   └─────────────┘                                       │
└─────────────────────────────────────────────────────────────────────────┘
                           │
          ┌────────────────┼────────────────┐
          ▼                ▼                ▼
   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
   │  Scanner 1  │  │  Scanner 2  │  │  Scanner N  │
   │  (workers)  │  │  (workers)  │  │  (workers)  │
   └─────────────┘  └─────────────┘  └─────────────┘
```

## Components

- **Coordinator Server**: PostgreSQL-backed API server that manages the batch queue and stores results
- **Feeder**: Background process that downloads domain files from GitHub and creates batches
- **Reaper**: Background process that resets stale batches (from dead scanners)
- **Scanner Workers**: Distributed workers that claim batches and perform DNS LOC lookups

## Prerequisites

- Go 1.21+
- PostgreSQL 14+

## Quick Start with Docker

```bash
# Start PostgreSQL and coordinator
docker compose up -d

# Register a scanner client (get a token)
curl -X POST http://localhost:8080/api/admin/clients \
  -H "X-Admin-Key: secret-admin-key" \
  -H "Content-Type: application/json" \
  -d '{"name": "scanner-1"}'
# Returns: {"id":"...","name":"scanner-1","token":"<YOUR_TOKEN>"}

# Trigger file discovery (optional - happens automatically on startup)
curl -X POST http://localhost:8080/api/admin/discover-files \
  -H "X-Admin-Key: secret-admin-key"

# Run the scanner
COORDINATOR_URL=http://localhost:8080 \
SCANNER_TOKEN=<YOUR_TOKEN> \
go run ./cmd/scanner
```

## Building

```bash
# Build both binaries
go build -o coordinator ./cmd/coordinator
go build -o scanner ./cmd/scanner

# Or with Docker
docker build -f Dockerfile.coordinator -t loc-coordinator .
docker build -f Dockerfile.scanner -t loc-scanner .
```

## Configuration

### Coordinator

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `DATABASE_URL` | `postgres://localhost:5432/locscanner?sslmode=disable` | PostgreSQL connection string |
| `ADMIN_API_KEY` | (required) | API key for admin endpoints |
| `LISTEN_ADDR` | `:8080` | HTTP listen address |
| `METRICS_ADDR` | `:9090` | Prometheus metrics address |
| `METRICS_INTERVAL` | `15s` | How often to update gauge metrics |
| `HEARTBEAT_TIMEOUT` | `2m` | Time before scanner considered dead |
| `REAPER_INTERVAL` | `60s` | How often to check for stale batches |
| `BATCH_TIMEOUT` | `10m` | Time before stale batches are reset |
| `BATCH_SIZE` | `1000` | Number of FQDNs per batch |
| `MAX_PENDING_BATCHES` | `20` | Maximum pending batches in queue |
| `FEEDER_POLL_INTERVAL` | `5s` | How often feeder checks for capacity |
| `GITHUB_TOKEN` | (optional) | GitHub PAT for LFS downloads (see below) |

**Note on `GITHUB_TOKEN`**: The domain files are stored in Git LFS. Without a token, downloads may fail if the repository's LFS quota is exceeded. With a token, bandwidth is charged to your GitHub account instead. Create a [Personal Access Token](https://github.com/settings/tokens) (no special scopes needed for public repos).

### Scanner

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `COORDINATOR_URL` | `http://localhost:8080` | Coordinator API URL |
| `SCANNER_TOKEN` | (required) | Token from client registration |
| `WORKER_COUNT` | `4` | Number of parallel workers |
| `HEARTBEAT_INTERVAL` | `30s` | Heartbeat frequency |
| `DNS_WORKERS` | `10` | Concurrent DNS lookups per batch |
| `DNS_TIMEOUT` | `5s` | DNS query timeout |
| `METRICS_ADDR` | `:9090` | Prometheus metrics address |

## API Endpoints

### Admin (requires `X-Admin-Key` header)

- `POST /api/admin/clients` - Register a scanner client
- `GET /api/admin/clients` - List scanner clients
- `DELETE /api/admin/clients/{id}` - Remove a scanner client
- `POST /api/admin/discover-files` - Trigger domain file discovery from GitHub
- `POST /api/admin/reset-scan` - Reset all files to pending for a full re-scan

### Scanner (requires `Authorization: Bearer <token>`)

- `POST /api/scanner/jobs` - Request a batch of FQDNs to scan
- `POST /api/scanner/heartbeat` - Send keepalive
- `POST /api/scanner/results` - Submit scan results for a batch

### Public (no auth)

- `GET /api/public/records` - List discovered LOC records (paginated)
- `GET /api/public/records.geojson` - Get LOC records as GeoJSON
- `GET /api/public/stats` - Get scanning statistics and progress

## Example: View Results

```bash
# Get statistics (includes file/batch progress)
curl http://localhost:8080/api/public/stats | jq

# List LOC records
curl "http://localhost:8080/api/public/records?limit=100" | jq

# Filter by domain
curl "http://localhost:8080/api/public/records?domain=nikhef.nl" | jq

# Get GeoJSON for mapping
curl http://localhost:8080/api/public/records.geojson -o records.geojson
```

## Domain Files

The scanner automatically discovers and processes domain files from the [tb0hdan/domains](https://github.com/tb0hdan/domains) project on GitHub. These files contain:

- ~1.7 billion unique FQDNs
- Organized as XZ-compressed text files (one FQDN per line)
- Updated periodically by the domains project

The feeder downloads each file in memory, decompresses it, and creates batches of FQDNs for scanners to process.

## Test Domains

These domains are known to have LOC records:

- alink.net
- caida.org
- chagas.eti.br
- ckdhr.com
- distributed.net (rc5stats.distributed.net)
- goldenglow.com.au (www.goldenglow.com.au)
- nikhef.nl
- vrx.net
- yahoo.com

## LOC Record Format

LOC records (RFC 1876) contain:

| Field | Description |
|-------|-------------|
| Latitude | Position (degrees, minutes, seconds) |
| Longitude | Position (degrees, minutes, seconds) |
| Altitude | Height above sea level (meters) |
| Size | Diameter of enclosing sphere (meters) |
| Horizontal Precision | Accuracy of lat/long (meters) |
| Vertical Precision | Accuracy of altitude (meters) |

Example: `52 22 23.000 N 4 53 32.000 E -2.00m 1m 10000m 10m`

## Metrics

Both coordinator and scanner expose Prometheus metrics:

### Coordinator Metrics (`:9090/metrics`)

**Gauges (Database State)**
- `locplace_domain_files_total/pending/processing/complete` - File processing status
- `locplace_batches_pending/in_flight` - Batch queue status
- `locplace_loc_records_total` - Total LOC records found
- `locplace_domains_with_loc` - Unique root domains with LOC
- `locplace_scanners_total/active` - Scanner client status

**Counters (Work Done)**
- `locplace_scan_completions_total` - Batches completed
- `locplace_domains_checked_total` - FQDNs checked
- `locplace_loc_discoveries_total` - LOC records discovered
- `locplace_reaper_batches_released_total` - Stale batches reset

### Scanner Metrics (`:9090/metrics`)

- `scanner_getjobs_duration_seconds` - Time to fetch batches
- `scanner_dns_duration_seconds` - Time for DNS lookups
- `scanner_submit_duration_seconds` - Time to submit results
- `scanner_fqdns_processed_total` - FQDNs processed
- `scanner_loc_records_found_total` - LOC records found
