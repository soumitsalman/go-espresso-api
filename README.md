# Espresso API & MCP

**Version:** 0.1

Espresso is a curated business intelligence product suite. This service exposes the underlying data store over a read-only HTTP API with optional semantic search via an embedding service.

---

## Data model

A **sip** is the basic unit of information. Each sip is stored with a UUID primary key, a `created` timestamp, and a JSON **digest** whose shape depends on the sip kind:

| Kind | Description |
|------|-------------|
| **tick** | Micro-level data points such as market performance for a given day |
| **event** | A self-contained set of related micro actions and ticks (for example a court ruling and its local fallout) |
| **signal** | Larger derived intelligence synthesized from related events and ticks (for example cross-domain market and policy outlook) |

All sip identifiers are **UUIDs** (RFC 4122), for example `339366bc-464d-582f-8132-6875ccc814d2`. Pass them as strings in query parameters and path segments.

List endpoints do not return the raw database row. The router **flattens** each sip's digest and merges `id` and `created` into every JSON object. The documented response fields in `router/types.go` are the stable, commonly present keys; individual records may include additional pipeline-specific keys.

Interactive schema documentation is available at `/swagger/index.html` after running the server (generated from `router/routes.go` and `router/types.go` via [swaggo](https://github.com/swaggo/swag)).

---

## General notes

- All endpoints return JSON unless noted otherwise. Success responses use `200` or `204` (empty result set). Errors use `400`, `401`, `429`, or `500` with a body like `{ "error": "..." }`.
- Authentication is optional. When `API_KEYS` is set, each request must include a matching header (see [Configuration](#configuration)).
- Concurrency is limited by an in-memory queue; excess requests wait rather than fail immediately.
- Protected routes accept only `GET` and `OPTIONS` (CORS enabled).

---

## Quick start

```bash
BASE_URL="http://localhost:8080"
API_KEY="my-secret"                              # only if API key enforcement is enabled
AUTH='-H "X-API-KEY: '"$API_KEY"'"'             # omit when API_KEYS is unset
```

### Health check

```bash
curl -s "$BASE_URL/health" | jq
```

```json
{ "status": "alive" }
```

---

## Routes

### Tags

```bash
curl -s $AUTH "$BASE_URL/tags?limit=20&offset=0" | jq
```

| | |
|---|---|
| **Method** | `GET` |
| **Path** | `/tags` |
| **Description** | Paginated list of unique tag strings extracted from event and signal sips. Use these values with the `tags` query parameter on `/events` and `/signals`. |

**Query parameters**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit` | int | 16 | Page size (1–128) |
| `offset` | int | 0 | Number of items to skip |

**Response:** JSON array of strings, e.g. `["gerrymandering", "market_volatility", "supreme_court"]`.

---

### Events

```bash
curl -s $AUTH \
  "$BASE_URL/events?tags=supreme_court,gerrymandering&from=2026-05-01&limit=5" | jq
```

Semantic search example:

```bash
curl -s $AUTH \
  "$BASE_URL/events?q=voting+rights+redistricting&acc=0.8&limit=5" | jq
```

Fetch by UUID:

```bash
curl -s $AUTH \
  "$BASE_URL/events?ids=339366bc-464d-582f-8132-6875ccc814d2" | jq
```

| | |
|---|---|
| **Method** | `GET` |
| **Path** | `/events` |
| **Description** | Event-kind sips, sorted by `created` descending. When `from` is omitted, results are limited to roughly the last 7 days. |

**Query parameters**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `ids` | CSV UUIDs | — | Fetch specific event sips |
| `tags` | CSV strings | — | Tag filters (AND across supplied tags) |
| `q` | string | — | Semantic search query (max 1024 chars; requires embedder) |
| `acc` | float | 0.75 | Minimum embedding similarity for `q` (0.0–1.0; higher = stricter) |
| `from` | date | ~7 days ago | Include events created on or after `YYYY-MM-DD` |
| `limit` | int | 16 | Page size (1–128) |
| `offset` | int | 0 | Pagination offset |

**Response:** JSON array of flattened event digests. Example element:

```json
{
  "id": "339366bc-464d-582f-8132-6875ccc814d2",
  "created": "2026-05-19T06:00:00-04:00",
  "briefing": "Discussion on recent Supreme Court rulings affecting redistricting...",
  "event_type": "political_analysis",
  "impact_level": "high",
  "future_outlook": "Concerns about erosion of Black political influence...",
  "key_events": [
    "Voting rights activism in 1987",
    "Supreme Court's Louisiana v. Callais decision"
  ],
  "cross_domain_impacts": [
    "Legislative actions influencing redistricting",
    "Judicial changes altering court mandates on fair representation"
  ],
  "people": ["michael_watts", "rep_bennie_thompson"],
  "regions": ["mississippi", "deep_south"],
  "tags": ["voter_suppression", "gerrymandering", "supreme_court"]
}
```

Additional digest keys may be present beyond those shown above.

---

### Signals

```bash
curl -s $AUTH \
  "$BASE_URL/signals?tags=market_volatility,ai_taxation&from=2026-06-01&limit=5" | jq
```

| | |
|---|---|
| **Method** | `GET` |
| **Path** | `/signals` |
| **Description** | Signal-kind sips (derived intelligence from related events and ticks), sorted by `created` descending. Supports the same filters as `/events`. |

**Query parameters:** Same as [Events](#events).

**Response:** JSON array of flattened signal digests. Example element:

```json
{
  "id": "e7d7571a-13f0-56f0-8563-50863b79c781",
  "created": "2026-06-02T14:02:00-04:00",
  "briefing": "On 2026-06-02, U.S. lawmakers and the Trump administration debated AI sovereign-wealth...",
  "impact_level": "high",
  "forecast": "Short-term: Market volatility will persist, AI regulatory scrutiny will intensify...",
  "events": [
    "2026-06-01: Senator Bernie Sanders introduced a 50% ownership tax on major AI firms"
  ],
  "impacts": [
    "9.3% market sell-off across tech and financial sectors.",
    "Decline in consumer confidence and increased credit-card delinquency."
  ],
  "drivers": [
    "High inflation and rising consumer costs driven by supply-chain bottlenecks and geopolitical tensions."
  ],
  "impacted_domains": ["finance", "technology", "cybersecurity", "labor", "healthcare", "energy", "policy"],
  "tags": ["ai_sovereign_wealth_fund", "ai_taxation", "inflation", "market_volatility"]
}
```

Additional digest keys may be present beyond those shown above.

---

### Related sips

```bash
curl -s $AUTH \
  "$BASE_URL/related/same_as?ids=b07049b5-54c0-50b0-a620-d3aea3f8a173&limit=10" | jq
```

| | |
|---|---|
| **Method** | `GET` |
| **Path** | `/related/{relationship}` |
| **Description** | Sips linked to the supplied UUIDs through the requested relationship. |

**Path parameters**

| Parameter | Values | Description |
|-----------|--------|-------------|
| `relationship` | `same_as`, `derived_from` | `same_as` finds equivalent or duplicate records; `derived_from` finds downstream records generated from the source sip |

**Query parameters**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `ids` | CSV UUIDs | *required* | Source sip UUIDs |
| `limit` | int | 16 | Page size (1–128) |
| `offset` | int | 0 | Pagination offset |

**Response:** JSON array of flattened digests. Each item follows the [Event](#events) or [Signal](#signals) field set depending on the related record's kind.

---

### Other endpoints

| Path | Description |
|------|-------------|
| `GET /favicon.ico` | Static favicon image |
| `GET /swagger/index.html` | Swagger UI (OpenAPI spec from `docs/`) |

---

## Development

### Prerequisites

- Go 1.26+
- PostgreSQL with the Espresso schema and pgvector extension
- A TEI-compatible embedding service (included in `docker-compose.yml` as `tei`)

### Build and run

```bash
go mod download
go build -o espressoapi .

# or
make run
```

```bash
export PORT=8080
export PG_CONNECTION_STRING="postgres://user:pass@localhost:5432/espresso?sslmode=disable"
export EMBEDDER_BASE_URL="http://localhost:10000"
export EMBEDDER_API_KEY=""          # optional
export EMBEDDER_MODEL=""            # optional
export MAX_CONCURRENT_REQUESTS=512  # optional; defaults to 512 in router
export API_KEYS="X-API-KEY=secret"  # optional; semicolon-separated Header=Value pairs

./espressoapi
```

Environment variables can also be loaded from a `.env` file (see `main.go` and `docker-compose.yml`).

### Docker Compose

```bash
docker compose up --build
```

This starts the API on port `8080` and a local TEI embedder on port `10000`. Place secrets and the database connection string in `.env`.

### Regenerate OpenAPI docs

After changing swagger annotations in `router/`:

```bash
go run github.com/swaggo/swag/cmd/swag@v1.16.4 init \
  -g router/routes.go -o docs --parseDependency --parseInternal
```

### Tests

Integration tests live under `tests/` and require a reachable database (configured via `.env`):

```bash
go test ./tests/...
```

---

## Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `PG_CONNECTION_STRING` | yes | — | PostgreSQL DSN |
| `EMBEDDER_BASE_URL` | yes | — | Base URL of the embedding service (gRPC/HTTP per `nlp/embedder.go`) |
| `EMBEDDER_API_KEY` | no | — | API key for the embedder |
| `EMBEDDER_MODEL` | no | — | Model name passed to the embedder |
| `PORT` | no | `8080` | HTTP listen port |
| `MAX_CONCURRENT_REQUESTS` | no | `512` | Max in-flight protected requests |
| `API_KEYS` | no | — | Semicolon-separated `Header=Value` pairs; when unset, auth is disabled |

**API key format:** `API_KEYS="X-API-KEY=secret;Authorization=Bearer token"`

When `API_KEYS` is empty, the server accepts unauthenticated requests. Set it before exposing the service publicly.

---

## Project layout

| Path | Purpose |
|------|---------|
| `main.go` | Entry point, env loading, server startup |
| `router/` | HTTP routes, swagger annotations, response types (`Event`, `Signal`) |
| `cupboard/` | PostgreSQL access layer and persistence types (`Sip`, `Source`, `Relation`) |
| `nlp/` | Remote embedder client |
| `docs/` | Generated OpenAPI spec (`swag init`) |
| `tests/` | Integration and stress tests |

---

## License

MIT — see [`LICENSE`](LICENSE).
