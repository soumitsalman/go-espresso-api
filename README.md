# Espresso API & MCP

**Version:** 0.1

Espresso is a curated business intelligence product suite. This service exposes the underlying data store over a read-only HTTP API with optional semantic search via an embedding service.

---

## Data model

A **sip** is the basic unit of information. Each sip is stored with a UUID primary key, a `created` timestamp, and a JSON **digest** whose shape depends on the sip kind:

| Kind | Description |
|------|-------------|
| **action** | Micro-level data points or actions such as market performance for a given day |
| **event** | A self-contained set of related micro actions and actions (for example a court ruling and its local fallout) |
| **signal** | Larger derived intelligence synthesized from related events and actions (for example cross-domain market and policy outlook) |

All sip identifiers are **UUIDs** (RFC 4122), for example `339366bc-464d-582f-8132-6875ccc814d2`. Pass them as strings in query parameters and path segments.

List endpoints do not return the raw database row. The router **flattens** each sip's digest and merges `id` and `created` into every JSON object. The documented response fields in `router/types.go` are the stable, commonly present keys; individual records may include additional pipeline-specific keys.

### Database schema

The API expects a PostgreSQL database initialized by the Espresso ingestion pipeline (`pycoffeemaker/pycupboard/pgcupboard.py`). The schema below matches that module's `_INIT_STMTS`.

**Extensions**

| Extension | Purpose |
|-----------|---------|
| `vector` | pgvector for sip embeddings (`vector(384)`) |
| `pg_trgm` | Trigram support (used by the pipeline) |

**Helper function**

```sql
CREATE OR REPLACE FUNCTION immutable_tags_to_text(tags text[])
RETURNS text
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $$
    SELECT array_to_string(COALESCE(tags, '{}'), ' ');
$$;
```

#### Table: `sips`

Primary store for all sip kinds. Rows are immutable after insert (`ON CONFLICT DO NOTHING` on ingest).

| Column | Type | Notes |
|--------|------|-------|
| `id` | `UUID` | Primary key (deterministic from `url` at ingest) |
| `kind` | `TEXT` | `action`, `event`, or `signal` |
| `created` | `TIMESTAMPTZ` | Creation time; drives default list ordering |
| `source` | `UUID` | Optional FK to `sources.id` (not enforced) |
| `embedding` | `vector(384)` | Dense vector for semantic search (`q` / `acc` on list routes) |
| `tags` | `TEXT[]` | Facet tags; AND-filtered on `/events` and `/signals` |
| `tags_fts` | `tsvector` | Generated stored column: `to_tsvector('simple', immutable_tags_to_text(tags))` |
| `digest` | `JSONB` | Kind-specific payload; flattened in API responses |
| `url` | `TEXT` | Canonical content URL (used to derive `id`) |
| `base_url` | `TEXT` | Publisher base URL (used to derive `source`) |

**Indexes on `sips`:** `url`, `base_url`, `kind`, `created`, `source`, `tags` (GIN on `tags_fts`), HNSW on `embedding` (`vector_cosine_ops`, `m = 16`, `ef_construction = 64`).

#### Table: `sources`

Publishers referenced by `sips.source`.

| Column | Type | Notes |
|--------|------|-------|
| `id` | `UUID` | Primary key (deterministic from `base_url` at ingest) |
| `base_url` | `TEXT` | Unique publisher origin |
| `domain_name` | `TEXT` | Hostname |
| `site_name` | `TEXT` | Display name |
| `description` | `TEXT` | Publisher blurb |
| `favicon` | `TEXT` | Favicon URL |
| `rss_feed` | `TEXT` | RSS feed URL |

**Index on `sources`:** `base_url`.

#### Table: `relations`

Directed edges between sips. `from_id` and `to_id` reference `sips.id` by convention only (no FK constraint, for ingest performance).

| Column | Type | Notes |
|--------|------|-------|
| `from_id` | `UUID` | Source sip |
| `to_id` | `UUID` | Target sip |
| `relationship` | `TEXT` | Edge type; API exposes `same_as` and `derived_from` via `/related/{relationship}` |

**Unique constraint:** `(from_id, to_id, relationship)`.

**Indexes on `relations`:** `from_id`, `to_id`, `relationship`.

```mermaid
erDiagram
    sources ||--o{ sips : "source"
    sips ||--o{ relations : "from_id"
    sips ||--o{ relations : "to_id"

    sources {
        uuid id PK
        text base_url
        text domain_name
        text site_name
    }

    sips {
        uuid id PK
        text kind
        timestamptz created
        uuid source
        vector embedding
        text_array tags
        jsonb digest
        text url
    }

    relations {
        uuid from_id
        uuid to_id
        text relationship
    }
```

Interactive schema documentation is available at `/swagger/index.html` after running the server (generated from `router/routes.go` and `router/types.go` via [swaggo](https://github.com/swaggo/swag)).

---

## General notes

- List endpoints (`/tags`, `/events`, `/signals`, `/related/{relationship}`) accept an optional `response_type` query parameter: `json` (default) or `text`. Both return the same underlying data; `text` renders it as flat plain text without JSON syntax, which reduces token cost for MCPs and AI agents. See [Response format](#response-format-response_type) below.
- Success responses use `200` or `204` (empty result set). Errors use `400`, `401`, `429`, or `500` with a JSON body like `{ "error": "..." }`.
- Authentication is optional. When `API_KEYS` is set, each request must include a matching header (see [Configuration](#configuration)).
- Concurrency is limited by an in-memory queue; excess requests wait rather than fail immediately.
- Protected routes accept only `GET` and `OPTIONS` (CORS enabled).

### Response format (`response_type`)

| Value | Content-Type | Description |
|-------|--------------|-------------|
| `json` | `application/json` | Default. Structured JSON arrays (or a JSON string array for `/tags`). |
| `text` | `text/plain` | Same data as flat plain text — no braces, quotes, or JSON field names. Useful for MCP tools and LLM context where JSON syntax adds unnecessary tokens. |

**Sip list routes** (`/events`, `/signals`, `/related/{relationship}`): each sip is one block of `key: value` lines, records separated by a blank line. Tag-like digest fields (`regions`, `people`, `tags`, etc.) are rolled into a single `related_to:` line.

```text
date: 2026-06-07
briefing: Analysis of upcoming council tax revaluation in Wales...
event_type: policy_reform
impact_level: high
actions: Cooperation Agreement signed in 2021, Re-evaluation scheduled for April 2028
related_to: wales, public_policy, council_tax

date: 2026-06-07
briefing: Analysts note surging investor optimism...
...
```

**Tags** (`/tags`): comma-separated tag strings instead of a JSON array.

```bash
curl -s $AUTH "$BASE_URL/events?tags=market_trends&from=2026-06-01&response_type=text"
curl -s $AUTH "$BASE_URL/signals?q=market+volatility&acc=0.75&limit=5&response_type=text"
curl -s $AUTH "$BASE_URL/tags?limit=20&response_type=text"
```

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
| `response_type` | string | `json` | `json` or `text` (comma-separated plain text) |
| `limit` | int | 16 | Page size (1–128) |
| `offset` | int | 0 | Number of items to skip |

**Response:** JSON array of strings by default, e.g. `["gerrymandering", "market_volatility", "supreme_court"]`. With `response_type=text`, a single comma-separated string.

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

Plain-text response (same filters; lower token cost for MCPs / agents):

```bash
curl -s $AUTH \
  "$BASE_URL/events?tags=supreme_court,gerrymandering&from=2026-05-01&response_type=text"
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
| `response_type` | string | `json` | `json` or `text` (flat plain-text digests) |
| `limit` | int | 16 | Page size (1–128) |
| `offset` | int | 0 | Pagination offset |

**Response:** JSON array of flattened event digests when `response_type=json` (default). Example elements:

```json
[
  {
    "id": "091726f8-421a-566d-9db8-339625f2ed9e",
    "reported": "2026-06-28T22:49:07Z",
    "briefing": "On June 28, 2026, three U.S. firefighters died while battling rapidly spreading wildfires near the Colorado-Utah border; approximately 100 sq km burned. Temperatures reached 34°C with strong winds, prompting mass evacuations. The Snyder Fire merged with others, causing significant damage to infrastructure like ski resorts. Causes include severe drought and human factors. This incident reflects escalating regional wildfire risks driven by climate change.",
    "event_type": "wildfire_outbreak",
    "impact_level": "high",
    "future_outlook": "Continued extreme fire seasons expected without mitigation efforts.",
    "actions": [
      "2026-06-28 Firefighters died and injuries occurred at Wyoming-Utah border.",
      "2026-06-28 Large wildfire destroyed parts of ski resorts."
    ],
    "cross_domain_impacts": [
      "public_safety: Increased risk of civilian casualties.",
      "tourism: Disruption of winter sports facilities.",
      "environmental: Habitat loss in mountainous areas."
    ],
    "companies": ["us_federal", "us_state"],
    "regions": ["colorado", "utah"],
    "macro_context": "western_us_climate_crisis",
    "tags": ["wildfire", "climate_change", "us", "emergency_response"]
  },
  {
    "id": "6b4562d2-0a5c-540f-a39e-f1d5cad4ee5f",
    "reported": "2026-06-28T21:52:00Z",
    "briefing": "On June 28, 2026, U.S.-Iran governments reached an emergency agreement halting military clashes amid regional tensions. The pause prevents broader war while resuming negotiations by June 30 focused on Ormuz Strait security. This diplomatic step averts immediate market shocks but depends on both sides lowering aggressive rhetoric for tangible outcomes.",
    "event_type": "diplomatic_ceasefire",
    "impact_level": "high",
    "future_outlook": "Talks may stabilize but risk re-escalation without sustained engagement.",
    "actions": [
      "2026-06-28 Governments of United States and Iran agree to cease fire",
      "2026-06-30 Bilateral talks scheduled in Oruz"
    ],
    "cross_domain_impacts": [
      "energy_markets: Reduced oil volatility expected",
      "security_framework: Gulf stability improved temporarily"
    ],
    "products": ["petróleo"],
    "regions": ["gulf", "ormuz"],
    "macro_context": "middle_east_conflict_resolution",
    "tags": ["diplomacy", "gulf", "iran", "us", "fuel"]
  }
]
```

With `response_type=text`, the same record is returned as a flat plain-text block (see [Response format](#response-format-response_type)).

Additional digest keys may be present beyond those shown above.

---

### Signals

```bash
curl -s $AUTH \
  "$BASE_URL/signals?tags=market_volatility,ai_taxation&from=2026-06-01&limit=5" | jq
```

Semantic search as plain text:

```bash
curl -s $AUTH \
  "$BASE_URL/signals?q=market+volatility&acc=0.75&limit=5&response_type=text"
```

| | |
|---|---|
| **Method** | `GET` |
| **Path** | `/signals` |
| **Description** | Signal-kind sips (derived intelligence from related events and actions), sorted by `created` descending. Supports the same filters as `/events`. |

**Query parameters:** Same as [Events](#events) (including `response_type`).

**Response:** JSON array of flattened signal digests when `response_type=json` (default). Example element:

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

With `response_type=text`, each signal is a flat plain-text digest block (see [Response format](#response-format-response_type)).

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
| `response_type` | string | `json` | `json` or `text` (flat plain-text digests) |
| `limit` | int | 16 | Page size (1–128) |
| `offset` | int | 0 | Pagination offset |

**Response:** JSON array of flattened digests when `response_type=json` (default). Each item follows the [Event](#events) or [Signal](#signals) field set depending on the related record's kind. With `response_type=text`, plain-text digest blocks as described in [Response format](#response-format-response_type).

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
