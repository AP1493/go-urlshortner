# go-urlshortner

A URL shortening service written in Go with Fiber, GORM, PostgreSQL and Redis.

Built as a solution to the [URL Shortening Service](https://roadmap.sh/projects/url-shortening-service)
project from roadmap.sh.

## Stack

| Piece | Choice |
|---|---|
| Language | Go 1.26 |
| HTTP | [Fiber v2](https://github.com/gofiber/fiber) |
| ORM | [GORM](https://gorm.io) + `gorm.io/driver/postgres` |
| Database | PostgreSQL 16 |
| Cache | Redis 7.2 via [go-redis v9](https://github.com/redis/go-redis) |
| Config | `.env` via [godotenv](https://github.com/joho/godotenv) |
| Runtime | Docker Compose |

## Layout

```
cmd/main.go                  entrypoint: config, DB, routes, graceful shutdown
internal/server/server.go    Fiber app construction (timeouts, app name)
internal/postgres/           connection, pooling, AutoMigrate
internal/redis/              Redis client, startup ping
internal/models/url.go       URL model
internal/routes/routes.go    route table
internal/handlers/           request handlers (the CRUD, health, redirect cache)
```

## Configuration

All settings come from `.env` in the repo root. Docker Compose reads this file
twice: once to interpolate `${VAR}` inside `docker-compose.yml`, and again via
`env_file:` to inject the variables into the app container.

```dotenv
POSTGRES_USER=urlshortener
POSTGRES_PASSWORD=devpassword
POSTGRES_DB=urlshortener

DB_HOST=postgres
DB_PORT=5432
```

`DB_HOST` is the Compose **service name**, not `localhost` — inside the app
container `localhost` refers to the app container itself.

Redis is configured with `REDIS_HOST`, `REDIS_PORT`, `REDIS_TLS` and an
optional `REDIS_PASSWORD`. Compose falls back to its own `redis` service when
`REDIS_HOST` is unset; outside Docker the default is `localhost:6379`. The app
pings Redis at startup and exits if it cannot reach it.

For ElastiCache, set `REDIS_HOST` to the primary endpoint and `REDIS_TLS=true`
when in-transit encryption is enabled; the server certificate is verified
against the system CA roots. Set `REDIS_PASSWORD` to the AUTH token if one is
configured. Only cluster-mode-disabled replication groups are supported: the
app uses a plain client, not a cluster client.

> `.env` holds a plaintext password. Add it to `.gitignore` and commit a
> `.env.example` with blank values instead. These credentials are for local
> development only; use a secret manager in production.

## Running

```bash
docker compose up --build
```

The API listens on <http://localhost:3001>. Postgres data lives in the `pgdata`
named volume and survives restarts.

```bash
docker compose down      # stop, keep the data
docker compose down -v   # stop and wipe the database
```

### Host port 5432

Compose publishes Postgres on host port `5432`. If you already run a native
Postgres locally, it binds `127.0.0.1:5432` while Docker binds `*:5432`, and the
loopback binding wins — `psql`/DBeaver on `localhost:5432` will silently reach
your **local** database, not the container. Change the mapping to `"5433:5432"`
if you need host tools to reach the container. The app itself is unaffected: it
connects over the Compose network as `postgres:5432`.

## API

Base URL: `http://localhost:3001`

Every request with a JSON body **must** send `Content-Type: application/json`.
Fiber's `BodyParser` picks its parser from that header; without it a JSON body is
parsed as form data and the request fails with `400 url is required`.

### `POST /shorten` — create

```bash
curl -X POST http://localhost:3001/shorten \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://example.com/some/long/path?x=1"}'
```

```json
{
  "id": 1,
  "url": "https://example.com/some/long/path?x=1",
  "shorten_code": "aIDWKZ7k",
  "count": 0,
  "created_at": "2026-08-26T17:11:37Z",
  "updated_at": "2026-08-26T17:11:37Z"
}
```

`201` on success. `400` if the body is not valid JSON, or the URL is empty, not
absolute, or not `http`/`https`.

The short code is 8 characters of base62 drawn from `crypto/rand`, so codes
cannot be guessed or enumerated. `shorten_code` carries a unique index, so a
repeat code can never be stored; there is no retry, a collision simply fails with
`500`. With 8 base62 characters that is vanishingly rare, and handling it is left
out to keep the project simple.

### `GET /redis-health` — Redis liveness

```bash
curl http://localhost:3001/redis-health
```

Responds `200` with the plain-text body `PONG` (Redis's reply to `PING`), or
`503` with `{"status":"error","redis":"unreachable"}` if Redis cannot be reached.
`GET /healthz` does the same for Postgres.

### `GET /shorten/:shorten_code` — resolve and redirect

```bash
curl -i http://localhost:3001/shorten/aIDWKZ7k
```

Responds `302 Found` with a `Location` header pointing at the original URL, and
increments `count`.

`302` rather than `301`/`308` is deliberate: a permanent redirect is cached by
the browser, which would stop later visits from ever being counted and would pin
the target for good.

The counter is incremented with a SQL expression (`count = count + 1`) rather
than a read-modify-write, so simultaneous hits cannot lose updates. It also
leaves `updated_at` alone — visiting a link is not a modification of the record.

The code → URL lookup is cached in Redis under `url:<code>` for one hour, so
repeat visits skip the Postgres read; the count is still incremented in
Postgres on every visit. `PUT` and `DELETE` evict the key. Redis is treated as
a cache only: if it errors mid-request the app logs it and falls back to
Postgres.

`404` if no URL has that code.

### `PUT /shorten/:shorten_code` — update

```bash
curl -X PUT http://localhost:3001/shorten/aIDWKZ7k \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://example.com/new/target"}'
```

Repoints an existing short code at a new URL. The short code itself is left
unchanged, so links already handed out keep working. The response carries the
real row, including the untouched `count` and a refreshed `updated_at`.

`400` if the body is invalid or the URL fails validation, `404` if no URL has
that short code.

### `GET /shorten/:shorten_code/stats` — access count

```bash
curl http://localhost:3001/shorten/aIDWKZ7k/stats
```

```json
{
  "id": 1,
  "url": "https://example.com/some/long/path?x=1",
  "shorten_code": "aIDWKZ7k",
  "count": 4,
  "created_at": "2026-08-26T17:39:52Z",
  "updated_at": "2026-08-26T17:39:52Z"
}
```

Returns the stored record without redirecting. Looking at the stats does not
itself count as a visit, so `count` is unchanged by this call.

`404` if no URL has that short code.

### `DELETE /shorten/:shorten_code` — delete

```bash
curl -i -X DELETE http://localhost:3001/shorten/aIDWKZ7k
```

`204` on success, `404` if the code is unknown.

### Status codes you may hit

| Code | Meaning |
|---|---|
| `400` | Invalid JSON body, or the URL failed validation |
| `404` | No route matches that path, **or** no URL has that short code |
| `405` | The path matches a registered route, but not for that HTTP method |

The `404` vs `405` distinction is Fiber's: `DELETE /shorten` returns `405` because only
`POST` is registered for that exact path, whereas `POST /shorten/abc/def` returns
`404` because no route pattern has that shape.

A full URL cannot be passed as a path segment — its `/` and `?` characters split
into further segments and a query string. The URL to shorten always goes in the
request body.

## Data model

```go
type URL struct {
	ID          uint      `gorm:"primaryKey"    json:"id"`
	URL         string    `gorm:"not null"      json:"url"`
	ShortenCode string    `gorm:"unique"        json:"shorten_code"`
	Count       int       `gorm:"default:0"     json:"count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
```

`AutoMigrate` runs at startup, so the `urls` table and its unique index are
created on first boot.

## Differences from the roadmap.sh brief

Minor divergences from the
[project specification](https://roadmap.sh/projects/url-shortening-service):

| Brief | This project |
|---|---|
| `GET /shorten/{code}` returns the URL object as JSON | issues a `302` redirect to the original URL instead |
| `accessCount` field on the stats response | named `count` |
| camelCase JSON (`shortCode`, `createdAt`) | snake_case (`shorten_code`, `created_at`) |
| MySQL or MongoDB suggested | PostgreSQL |

Paths and status codes match the brief: `POST /shorten`,
`GET`/`PUT`/`DELETE /shorten/{code}`, and `GET /shorten/{code}/stats`.

Required features that *are* covered: unique random short-code generation, URL
validation, per-URL access counting, creation/modification timestamps, and CRUD.

## Known issues

- No automated tests.
- No list-all endpoint.
- A short-code collision is not retried; it fails with `500`. See
  [`POST /shorten`](#post-shorten--create).
- A failed cache eviction, or an update racing a cache fill, can leave a stale
  redirect target in Redis for up to the one-hour TTL.

## Development

Run outside Docker against the Compose database — note the host port caveat
above:

```bash
go build ./...
go vet ./...
gofmt -l .
go run ./cmd
```
