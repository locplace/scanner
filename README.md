# DNS LOC Record Scanner

A distributed system to scan domains for DNS LOC (Location) records as defined in RFC 1876.

## Components

- **Coordinator Server**: PostgreSQL-backed API server that manages job distribution and stores results
- **Scanner Workers**: Distributed workers that discover subdomains and scan for LOC records

## Prerequisites

- Go 1.21+
- PostgreSQL 14+
- [subfinder](https://github.com/projectdiscovery/subfinder) installed in PATH

Install subfinder:
```bash
go install -v github.com/projectdiscovery/subfinder/v2/cmd/subfinder@latest
```

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

# Add domains to scan
curl -X POST http://localhost:8080/api/admin/domains \
  -H "X-Admin-Key: secret-admin-key" \
  -H "Content-Type: application/json" \
  -d '{"domains": ["nikhef.nl", "caida.org", "ckdhr.com"]}'

# Run the scanner (requires subfinder in PATH)
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
| `ADMIN_API_KEY` | `changeme` | API key for admin endpoints |
| `LISTEN_ADDR` | `:8080` | HTTP listen address |
| `JOB_TIMEOUT` | `10m` | Time before stale jobs are released |
| `HEARTBEAT_TIMEOUT` | `2m` | Time before client considered dead |
| `REAPER_INTERVAL` | `60s` | How often to check for stale jobs |

### Scanner

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `COORDINATOR_URL` | `http://localhost:8080` | Coordinator API URL |
| `SCANNER_TOKEN` | (required) | Token from client registration |
| `WORKER_COUNT` | `4` | Number of parallel workers |
| `BATCH_SIZE` | `3` | Domains per worker per fetch |
| `HEARTBEAT_INTERVAL` | `30s` | Heartbeat frequency |
| `DNS_WORKERS` | `10` | Concurrent DNS lookups |
| `DNS_TIMEOUT` | `5s` | DNS query timeout |
| `SUBFINDER_THREADS` | `10` | Subfinder concurrency |
| `SUBFINDER_TIMEOUT` | `30` | Subfinder source timeout (seconds) |
| `SUBFINDER_MAX_TIME` | `5` | Max enumeration time (minutes) |

## API Endpoints

### Admin (requires `X-Admin-Key` header)

- `POST /api/admin/domains` - Add domains to scan
- `POST /api/admin/clients` - Register a scanner client
- `GET /api/admin/clients` - List scanner clients
- `DELETE /api/admin/clients/{id}` - Remove a scanner client

### Scanner (requires `Authorization: Bearer <token>`)

- `POST /api/scanner/jobs` - Request domains to scan
- `POST /api/scanner/heartbeat` - Send keepalive
- `POST /api/scanner/results` - Submit scan results

### Public (no auth)

- `GET /api/public/records` - List discovered LOC records
- `GET /api/public/stats` - Get scanning statistics

## Example: View Results

```bash
# Get statistics
curl http://localhost:8080/api/public/stats | jq

# List LOC records
curl "http://localhost:8080/api/public/records?limit=100" | jq

# Filter by domain
curl "http://localhost:8080/api/public/records?domain=nikhef.nl" | jq
```

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

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Coordination Server                          │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                 │
│  │ Admin API   │  │ Scanner API │  │ Public API  │                 │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘                 │
│         └────────────────┼────────────────┘                         │
│                   ┌──────▼──────┐                                   │
│                   │  PostgreSQL │                                   │
│                   └─────────────┘                                   │
└─────────────────────────────────────────────────────────────────────┘
                           │
          ┌────────────────┼────────────────┐
          ▼                ▼                ▼
   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
   │  Scanner 1  │  │  Scanner 2  │  │  Scanner N  │
   │  (workers)  │  │  (workers)  │  │  (workers)  │
   └─────────────┘  └─────────────┘  └─────────────┘
```
