# SearchAPI

Field-deployed search and telemetry indexing API in Go.

SearchAPI is a compact Go service that ties PostgreSQL change events to an Elasticsearch-backed query surface. It was originally built as a technical exercise around employee/task search, but the architecture maps cleanly to customer-site deployment work: ingest structured operational records, listen for database-side events, push normalized documents into a search index, and expose simple HTTP endpoints for fast investigation by operators and engineers.

## Why It Fits Forward-Deployed Hardware Work

Modern hardware test teams need the same backend shape: local data capture, event-driven indexing, quick search over runs/assets/operators/tasks, and a clear path from lab prototype to customer workspace. Public descriptions of the target domain emphasize real-time hardware testing, telemetry/log/video/simulation data, edge execution, air-gapped or restricted deployments, and deployment engineers who own customer onboarding from discovery through production handoff. This repository demonstrates the backend instincts behind that work: Go services, Postgres schemas, Elasticsearch queries, HTTP APIs, auth boundaries, runbooks, and field-adaptable integration patterns.

This is not presented as a finished hardware test platform. It is a deliberately small artifact showing how I approach the first mile of a customer deployment: define the data model, stand up the service, wire event notifications, make data searchable, document the install path, and leave clear seams for the next client-specific integration.

## Technical Highlights

- Go HTTP API using `gorilla/mux`
- PostgreSQL-backed source of record
- `pq.Listener` event loop for database notifications
- Elasticsearch indexing and query endpoints
- Basic-auth wrapper scaffold
- CORS support for browser-based operational tools
- SQL bootstrap notes and deployment preflight checklist

## Representative Endpoints

```text
GET /employees
GET /employees/uuid/{uniqid}
GET /employees/empid/{empid}
GET /tasks
GET /tasks/name/{title}
GET /whois
GET /whois/{taskid}

GET /search/tasks/name/{title}
GET /search/employees/uuid/{uniqid}
GET /search/whois/assigned/{taskid}
```

## Deployment Readiness Signals

The repo includes `sql_setup.txt` and `preflight_checklist.txt` because the interesting part of forward-deployed engineering is rarely just writing code. The work is making a service run inside a real environment: credentials, local services, schema shape, operator assumptions, security boundaries, and repeatable startup steps. SearchAPI keeps those concerns visible instead of hiding them behind a demo.

## How I Would Extend It For A Hardware Telemetry Client

1. Replace the employee/task schema with assets, test runs, channels, alarms, procedures, and operator annotations.
2. Move configuration into environment variables or a deployment manifest for on-prem and air-gapped installs.
3. Add stream ingestion from test benches or gateway devices.
4. Add typed validation and replayable event logs for auditability.
5. Package a repeatable customer onboarding path with seed data, health checks, and observability.

## Status

Portfolio reference project. Proprietary license; public viewing only.
