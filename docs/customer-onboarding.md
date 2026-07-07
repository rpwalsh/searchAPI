# Customer Onboarding Runbook

This runbook is written for a forward-deployed engineer bringing SearchAPI into
a restricted customer workspace.

## 1. Confirm Trust Boundary

- Identify whether the site is cloud-connected, self-hosted, or fully air-gapped.
- Decide whether Elasticsearch is allowed. It is optional.
- Assign `SITE_ID`, operator network ingress, and API key handling.

## 2. Provision Dependencies

- PostgreSQL 14+ inside the customer environment.
- Optional Elasticsearch/OpenSearch-compatible endpoint for audit-event search.
- Gateway/test bench able to send JSON or NDJSON over HTTP.

## 3. Configure

Start from `deploy/onprem.env.example`.

Required:

- `SITE_ID`
- `API_ADDR`
- `API_KEY`
- `DATABASE_URL`

Optional:

- `ELASTICSEARCH_URL`
- `ELASTICSEARCH_API_KEY`
- `ELASTICSEARCH_INDEX`

## 4. First Boot

The service runs migrations automatically.

Smoke checks:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
```

Seed the workspace:

```bash
curl -X POST http://localhost:8080/api/v1/onboarding/seed \
  -H "Authorization: Bearer $API_KEY"
```

## 5. Gateway Test

```bash
curl -X POST http://localhost:8080/api/v1/ingest/stream \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/x-ndjson" \
  --data-binary @examples/gateway-stream.ndjson
```

Confirm:

```bash
curl -H "Authorization: Bearer $API_KEY" http://localhost:8080/api/v1/alarms
curl -H "Authorization: Bearer $API_KEY" "http://localhost:8080/api/v1/search?q=threshold"
curl -H "Authorization: Bearer $API_KEY" http://localhost:8080/api/v1/events
```

## 6. Handoff Criteria

- A seeded test run is visible.
- Gateway samples are accepted with deterministic replay keys.
- A threshold crossing creates an alarm.
- Metrics increment after ingestion.
- Audit events are searchable.
- Operator annotations can be attached to a run.
