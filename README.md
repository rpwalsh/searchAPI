# SearchAPI

Field-deployed hardware telemetry ingestion, audit, and search API in Go.

SearchAPI is a production-shaped backend for customer-site hardware test environments. It models assets, test runs, telemetry channels, procedures, alarms, operator annotations, and replayable audit events; ingests JSON or NDJSON streams from gateways/test benches; stores canonical state in PostgreSQL; and optionally mirrors audit events into Elasticsearch for fast investigation.

The implementation is designed for forward-deployed engineering work: take a customer workspace from zero to useful, keep the install path explicit, support restricted/on-prem environments, and leave operators with health checks, metrics, seed data, and a searchable event trail.

## What Changed

- Added a dark glass operator console served by the Go backend at `/`.
- Replaced the legacy employee/task demo schema with hardware telemetry domain models.
- Moved all deploy-time settings to environment variables.
- Added typed JSON validation for assets, procedures, runs, channels, samples, alarms, and annotations.
- Added stream ingestion at `POST /api/v1/ingest/stream` for JSON arrays, single objects, or NDJSON.
- Added replay keys on telemetry samples and audit events for deterministic replay/audit behavior.
- Added threshold alarm generation from channel limits.
- Added Postgres migrations, `pg_notify` event publication, and optional Elasticsearch indexing.
- Added health, readiness, metrics, search, event replay, and onboarding seed endpoints.

## Run Locally

```powershell
$env:DATABASE_URL = "postgres://postgres:postgres@localhost:5432/telemetry?sslmode=disable"
$env:SITE_ID = "customer-sea-01"
$env:API_KEY = "change-me"
$env:SEED_ON_START = "true"
go run .
```

Open `http://localhost:8080/` for the operator console. The browser UI is intentionally thin: it only renders backend state and calls the API. Validation, threshold alarms, replay handling, seed data, and search behavior stay on the Go backend.

Elasticsearch is optional. Without it, `/api/v1/search` falls back to PostgreSQL audit-event search.

```powershell
$env:ELASTICSEARCH_URL = "http://localhost:9200"
$env:ELASTICSEARCH_INDEX = "hardware-telemetry-events"
```

## Core Endpoints

```text
GET  /healthz
GET  /readyz
GET  /metrics

GET  /api/v1/assets
POST /api/v1/assets
GET  /api/v1/assets/{id}

GET  /api/v1/procedures
POST /api/v1/procedures
GET  /api/v1/test-runs
POST /api/v1/test-runs
GET  /api/v1/channels
POST /api/v1/channels

POST /api/v1/ingest/stream
GET  /api/v1/alarms
GET  /api/v1/annotations
POST /api/v1/annotations
GET  /api/v1/events
GET  /api/v1/search?q=vibration
POST /api/v1/onboarding/seed
```

When `API_KEY` is set, protected endpoints accept either:

```text
Authorization: Bearer <API_KEY>
X-API-Key: <API_KEY>
```

## Gateway Stream Example

`POST /api/v1/ingest/stream` accepts newline-delimited JSON for gateway-style streaming:

```json
{"run_id":"run_seed_acceptance_001","channel_id":"chan_accel_x","gateway_id":"gateway-sea-01","timestamp":"2026-07-07T19:00:01Z","numeric_value":2.4,"quality":"good","replay_key":"gateway-sea-01-0001"}
{"run_id":"run_seed_acceptance_001","channel_id":"chan_accel_x","gateway_id":"gateway-sea-01","timestamp":"2026-07-07T19:00:02Z","numeric_value":8.1,"quality":"good","replay_key":"gateway-sea-01-0002"}
```

The second sample crosses the seeded acceleration threshold and creates an open alarm.

## Customer Onboarding Path

1. Set `DATABASE_URL`, `SITE_ID`, and `API_KEY` in a site-specific environment file.
2. Start the service once; migrations run automatically.
3. Run `POST /api/v1/onboarding/seed` to create a test stand, procedure, run, channels, and sample telemetry.
4. Confirm `/healthz`, `/readyz`, `/metrics`, `/api/v1/events`, and `/api/v1/search?q=threshold`.
5. Point a gateway or bench script at `/api/v1/ingest/stream` with replay keys enabled.

## Status

Portfolio reference project. Proprietary license; public viewing only.
