# Insight

Insight is a web UI for browsing, searching and editing [state-manager](https://github.com/reconcile-kit/state-manager) resources.

- [How it works](#how-it-works)
- [Installation](#installation)
  - [Requirements](#requirements)
  - [1. PostgreSQL role](#1-postgresql-role)
  - [2. Image](#2-image)
  - [3. Run](#3-run)
  - [4. Verify](#4-verify)
  - [5. Sign-in with Keycloak](#5-sign-in-with-keycloak)
  - [6. MCP tokens](#6-mcp-tokens)
- [Configuration](#configuration)
- [Access and authorization](#access-and-authorization)
- [MCP for AI agents](#mcp-for-ai-agents)
- [Performance at scale](#performance-at-scale)
- [Local environment](#local-environment)
- [Development](#development)
- [insight API](#insight-api)
- [Troubleshooting](#troubleshooting)

## How it works

Insight is a single Go binary with an embedded React frontend. It has no external dependencies other than PostgreSQL and state-manager.

```
browser ─────► insight ─┬─► PostgreSQL (replica)  reads: list, filters, counts
AI agent (MCP) ┘        └─► state-manager         GET a single resource, create, update, delete
```

| Request | Where it goes |
|---|---|
| `POST /insight/v1/resources/search`, `POST /insight/v1/values` | Straight to PostgreSQL, **read only**. |
| `GET/POST/PUT/DELETE /api/v1/groups/...` | Proxied to state-manager unchanged. |
| Other `/api/*`, including the state-manager list | `404`. |
| `/login`, `/auth/*` | OIDC sign-in, if enabled. Without a session, everything else except `/mcp` and the probes is unavailable. |
| `POST /mcp` | MCP for AI agents, with its own token-based authorization. Same logic inside: reads from PostgreSQL, GET and writes through state-manager. See [MCP](#mcp-for-ai-agents). |

- The state-manager API is unchanged and receives no heavy queries. Complex filters run on the insight side.
- Insight cannot write to the database: every query runs in a `READ ONLY` transaction. Still, connect it to a replica with a read-only role.
- Works with PostgreSQL directly and through poolers (PgBouncer in any `pool_mode`), see [PgBouncer and other poolers](#pgbouncer-and-other-poolers).

### Features

- Filters by group, kind, namespace and shard. Dropdown values come from the database, take the other filters into account, and are highlighted with the same colors as in the table.
- Substring search by name.
- Label selector: `app=nginx, tier!=db, env in (a,b), env notin (c), team, !legacy, app~ngi`.
- Conditions on jsonb fields (`spec`, `status`, `annotations`, `finalizers`):
  - operators `= ≠ > ≥ < ≤ contains !contains regex in exists !exists`;
  - combine conditions with AND / OR;
  - the path is typed by hand: `resources.cpu`, `conditions[0].type`, `["app.kubernetes.io/name"]`.
- Total count of matching resources, server-side sorting by clicking a column header, pagination.
- Clicking a cell value or a label adds a filter. `Enter` in any field applies the filters.
- A resource opens in a side drawer; its body is reloaded from state-manager every time. JSON editor with syntax highlighting, save with `⌘S`.
- Light and dark themes. Filters are kept in the URL, so you can share a link.
- MCP server at `/mcp` for AI agents, with separate read and write tokens.
- Sign-in with Keycloak or another OIDC provider, with role-based access levels: read / write / delete.

## Installation

### Requirements

- **The state-manager PostgreSQL database.** Preferably a replica. Insight reads only the `resources` and `labels` tables. The schema is created and migrated by state-manager itself; insight runs no migrations.
- **Access to the state-manager HTTP API** from the insight pod or container.
- **Resources:** port `27182`, image of about 25 MB. 64 MB of memory and 0.1 CPU are usually enough.

### 1. PostgreSQL role

Create the role on the **primary**; it is replicated along with the data. If the state-manager database is not named `state_manager`, use your own name:

```sql
CREATE ROLE insight_ro LOGIN PASSWORD '<password>';
GRANT CONNECT ON DATABASE state_manager TO insight_ro;
GRANT USAGE ON SCHEMA public TO insight_ro;
GRANT SELECT ON public.resources, public.labels TO insight_ro;
-- optional: extra server-side protection against writes
ALTER ROLE insight_ro SET default_transaction_read_only = on;
```

No other privileges are needed.

### 2. Image

The image is built by GitHub Actions when a `v*` tag is pushed and published to Docker Hub for `linux/amd64` and `linux/arm64`:

```
docker.io/dhnikolas/insight:<version>      # git tag v0.3.1 → image 0.3.1
```

The Go backend with direct database reads starts at **0.3.0**; PgBouncer is supported from **0.3.1**. Images up to and including `0.2.1` are the old build: nginx proxies all of `/api` to state-manager, only `STATE_MANAGER_URL` is needed, and no database is used. Version list: [repository tags](https://github.com/reconcile-kit/insight/tags).

To build it yourself:

```bash
docker build -t insight .
```

### 3. Run

Two parameters are required: the database connection and the state-manager address. The full list is in [Configuration](#configuration).

**Docker**

```bash
docker run -d --name insight -p 27182:27182 \
  -e STATE_MANAGER_URL=http://state-manager:8080 \
  -e DATABASE_HOST=pg-replica.internal \
  -e DATABASE_USERNAME=insight_ro \
  -e DATABASE_PASSWORD='<password>' \
  -e DATABASE_NAME=state_manager \
  -e DATABASE_SSLMODE=require \
  dhnikolas/insight:0.3.1
```

**Kubernetes**

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: insight-db
stringData:
  DATABASE_URL: postgres://insight_ro:<password>@pg-replica.internal:5432/state_manager?sslmode=require
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: insight
spec:
  replicas: 1
  selector:
    matchLabels: { app: insight }
  template:
    metadata:
      labels: { app: insight }
    spec:
      containers:
        - name: insight
          image: dhnikolas/insight:0.3.1
          ports:
            - containerPort: 27182
          env:
            - name: STATE_MANAGER_URL
              value: http://state-manager:8080
          envFrom:
            - secretRef: { name: insight-db }
          livenessProbe:
            httpGet: { path: /healthz, port: 27182 }
          readinessProbe:
            httpGet: { path: /readyz, port: 27182 }
          resources:
            requests: { cpu: 50m, memory: 64Mi }
            limits: { memory: 256Mi }
          securityContext:
            runAsNonRoot: true
            readOnlyRootFilesystem: true
            allowPrivilegeEscalation: false
---
apiVersion: v1
kind: Service
metadata:
  name: insight
spec:
  selector: { app: insight }
  ports:
    - port: 80
      targetPort: 27182
```

Expose insight through an Ingress, but always behind authentication. See [Access and authorization](#access-and-authorization).

The pod is stateless, so you can run several replicas. Keep in mind that each one opens up to `DATABASE_MAX_CONNS` database connections.

### 4. Verify

```bash
curl -i http://<insight>/healthz     # 200 — the process is alive
curl -i http://<insight>/readyz      # 200 — the database is reachable; 503 and an error message — it is not
curl -s -X POST http://<insight>/insight/v1/resources/search -d '{"limit":1}'
```

If a required parameter is missing, insight exits immediately and logs the reason:

```
{"level":"ERROR","msg":"insight stopped","err":"STATE_MANAGER_URL is required"}
{"level":"ERROR","msg":"insight stopped","err":"set DATABASE_URL or DATABASE_USERNAME, DATABASE_PASSWORD and DATABASE_NAME"}
```

An unreachable database at startup does not crash the process. In that case `/readyz` returns `503` until the connection comes up.

### 5. Sign-in with Keycloak

Optional. Without it, sign-in is disabled and the UI is open to anyone with network access to it. Details and other providers are covered in [Sign-in with Keycloak or another OIDC provider](#sign-in-with-keycloak-or-another-oidc-provider).

**1. Keycloak client.** Clients → Create client:
- `Client ID`: `insight`;
- `Client authentication`: On, `Standard flow`: On;
- `Valid redirect URIs`: `https://insight.example.com/auth/callback`;
- `Valid post logout redirect URIs`: `https://insight.example.com/*`.

The client secret is on the Credentials tab.

**2. Roles.** You can keep the gatekeeper roles (`gatekeeper:insight-get`, `insight-edit`, `insight-admin`) or create client roles on the `insight` client, such as `insight:read`, `insight:write`, `insight:admin`, and assign them to users or groups.

**3. Secret.**

```bash
kubectl -n state-manager create secret generic insight-auth \
  --from-literal=AUTH_OIDC_CLIENT_SECRET='<client secret from Keycloak>' \
  --from-literal=AUTH_SESSION_SECRET="$(openssl rand -hex 32)"
```

**4. Deployment variables** (in addition to step [3](#3-run)):

```yaml
          env:
            - name: AUTH_ENABLED
              value: "true"
            - name: PUBLIC_URL                  # insight address in the browser
              value: https://insight.example.com
            - name: AUTH_OIDC_ISSUER_URL        # https://<keycloak>/realms/<realm>
              value: https://keycloak.example.com/realms/main
            - name: AUTH_OIDC_CLIENT_ID
              value: insight
            - name: AUTH_READ_ROLES
              value: gatekeeper:insight-get
            - name: AUTH_WRITE_ROLES
              value: gatekeeper:insight-edit
            - name: AUTH_ADMIN_ROLES
              value: gatekeeper:insight-admin
            - name: STATE_MANAGER_TOKEN         # only if state-manager has AUTH_ENABLED=true
              valueFrom:
                secretKeyRef: { name: insight-state-manager, key: token }
          envFrom:
            - secretRef:
                name: insight-auth
```

**5. Gatekeeper.** If gatekeeper used to protect the UI, remove it: insight now checks sign-in and roles itself. If gatekeeper stays, let `/login`, `/auth/*`, `/assets/*` and `/mcp` through without checks.

**6. Verify.**
- At startup the log shows `"msg":"login enabled"` with the issuer and roles.
- Opening insight redirects to `/login` with a "Continue with SSO" button.
- After signing in, the log shows `"msg":"auth: login"` with the `access` and `roles` fields. If `access` is `none`, compare `roles` with `AUTH_*_ROLES`.

### 6. MCP tokens

Optional. This enables `/mcp` for AI agents. It does not depend on Keycloak sign-in. Details are in [MCP for AI agents](#mcp-for-ai-agents).

**1. Tokens.** A read token and a read-write token:

```bash
kubectl -n state-manager create secret generic insight-mcp \
  --from-literal=MCP_READ_TOKEN="$(openssl rand -hex 32)" \
  --from-literal=MCP_WRITE_TOKEN="$(openssl rand -hex 32)"
```

One of the two is enough. For example, only `MCP_READ_TOKEN` if agents don't need to change anything.

**2. Deployment variables:**

```yaml
          envFrom:
            - secretRef:
                name: insight-mcp
```

If state-manager has `AUTH_ENABLED=true`, also set `STATE_MANAGER_TOKEN` (see step 5): MCP uses it to call state-manager.

**3. Verify.** At startup, `mcp_read_tokens` and `mcp_write_tokens` in the log are greater than zero. Request the tool list:

```bash
curl -s https://insight.example.com/mcp \
  -H "Authorization: Bearer $MCP_READ_TOKEN" \
  -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

With a read token the response lists three tools (`search_resources`, `list_values`, `get_resource`); with a write token, six.

**4. Connect an agent** (Claude Code):

```bash
claude mcp add --transport http insight https://insight.example.com/mcp \
  --header "Authorization: Bearer $MCP_READ_TOKEN"
```

**Rotating a token without downtime:** set the old and new tokens separated by a comma (`MCP_READ_TOKEN=old,new`), switch agents to the new one, then remove the old one.

## Configuration

All parameters are set through environment variables.

**Required**

| Variable | Example | Description |
|---|---|---|
| `STATE_MANAGER_URL` | `http://state-manager:8080` | state-manager address: scheme, host and port. The `/api/v1/...` path is appended to it. |
| `DATABASE_URL` | `postgres://insight_ro:pass@replica:5432/state_manager?sslmode=require` | PostgreSQL connection. You can set the `DATABASE_*` variables below instead. |

**Database connection by parts** (used only if `DATABASE_URL` is not set; these are the same variables state-manager uses)

| Variable | Default | Description |
|---|---|---|
| `DATABASE_HOST` | `localhost` | Replica host. |
| `DATABASE_PORT` | `5432` | |
| `DATABASE_USERNAME` | — (required) | Read-only role. |
| `DATABASE_PASSWORD` | — | |
| `DATABASE_NAME` | — (required) | |
| `DATABASE_SSLMODE` | `disable` | `disable`, `require`, `verify-ca` or `verify-full`. |

**Optional**

| Variable | Default | Description |
|---|---|---|
| `LISTEN_ADDR` | `:27182` | HTTP server address. |
| `DATABASE_MAX_CONNS` | `10` | Maximum database connections per pod. |
| `DATABASE_STATEMENT_TIMEOUT` | `15s` | Time limit for a single SQL query. If counting the total doesn't fit, the list is still returned without a total, and the UI shows "couldn't count". |
| `MCP_READ_TOKEN` | — | Read-only MCP token. Several can be given, comma-separated (for rotation). At least 16 characters. |
| `MCP_WRITE_TOKEN` | — | Read-write MCP token. Several can be given, comma-separated. Must differ from the read token. If no `MCP_*_TOKEN` is set, `/mcp` returns `404`. |
| `STATE_MANAGER_TOKEN` | — | Bearer token for state-manager (needed if it has `AUTH_ENABLED=true`). Used by MCP and, when sign-in is enabled, by the UI. |

**OIDC sign-in** (see [Access and authorization](#access-and-authorization))

| Variable | Default | Description |
|---|---|---|
| `AUTH_ENABLED` | `false` | `true` enables sign-in. Without a session, only the `/login` page is available. |
| `PUBLIC_URL` | — (required) | The address users open insight at, e.g. `https://insight.example.com`. The redirect URI is `PUBLIC_URL/auth/callback`. |
| `AUTH_OIDC_ISSUER_URL` | — (required) | Provider issuer, e.g. `https://keycloak.example.com/realms/main`. |
| `AUTH_OIDC_CLIENT_ID` | — (required) | |
| `AUTH_OIDC_CLIENT_SECRET` | — | Client secret. Not needed for a public client; PKCE is always used. |
| `AUTH_SESSION_SECRET` | — (required) | Session signing key, at least 32 characters: `openssl rand -hex 32`. Must be the same on all pods. Changing it ends all sessions. |
| `AUTH_SESSION_TTL` | `12h` | Session lifetime. Roles are re-read at the next sign-in. |
| `AUTH_OIDC_SCOPES` | `openid,email,profile` | Add `groups` or your own scope if the provider returns groups only with it. |
| `AUTH_ROLES_CLAIM` | `groups` | Claim containing roles; dot-separated path. Keycloak roles (`realm_access`, `resource_access`) are always collected. |
| `AUTH_READ_ROLES`, `AUTH_WRITE_ROLES`, `AUTH_ADMIN_ROLES` | — | Roles for read, write and delete, comma-separated. An empty list does not restrict that level. |
| `AUTH_COOKIE_SECURE` | `true` for https | The cookie `Secure` flag. |

`DATABASE_URL` accepts [libpq/pgx parameters](https://pkg.go.dev/github.com/jackc/pgx/v5/pgconn#ParseConfig), such as `sslrootcert=/certs/ca.pem` or `target_session_attrs=read-only`.

### PgBouncer and other poolers

Insight works through PgBouncer in `session` and `transaction` modes, including with `max_prepared_statements = 0`:
- only `application_name` is sent on connect; `statement_timeout` is set inside each transaction (`SET LOCAL`), so `DATABASE_STATEMENT_TIMEOUT` also works through a pooler;
- named prepared statements are not used (pgx `QueryExecModeExec`). You can override this with the `default_query_exec_mode` parameter in `DATABASE_URL`.

Don't add server parameters such as `search_path` or `options` to `DATABASE_URL`: PgBouncer will reject the connection with `unsupported startup parameter`.

## Access and authorization

Sign-in is disabled by default: anyone who opens the UI can read all resources and change them through state-manager. In this mode, don't expose insight publicly; keep it on an internal network, behind a VPN or behind an auth proxy.

### Sign-in with Keycloak or another OIDC provider

With `AUTH_ENABLED=true`, insight handles sign-in itself over OpenID Connect: Keycloak, Dex, Okta, Entra ID, Google and any other provider with discovery (`/.well-known/openid-configuration`). No separate proxy such as gatekeeper or oauth2-proxy is needed.

- An unauthenticated user lands on the `/login` page with a "Continue with SSO" button. After signing in, they return to where they came from, URL filters included.
- The authorization code flow with PKCE, `state` and `nonce` is used. After sign-in, insight issues its own `insight_session` cookie (HttpOnly, SameSite=Lax, HMAC-SHA256 signature) valid for `AUTH_SESSION_TTL`. Provider tokens are not stored. The one exception is the ID token in a cookie with `Path=/auth`: it is needed only as `id_token_hint` on sign-out.
- "Sign out" also ends the session at the provider (RP-initiated logout via `end_session_endpoint`), so the next sign-in asks for the password again.
- Modifying requests from other sites are rejected (`Origin` and `Sec-Fetch-Site` checks).
- `/mcp` is not affected: it has its own tokens, see [MCP](#mcp-for-ai-agents).

**Access levels** are set by roles from the token. Any of the listed roles is enough:

| Level | Variable | Allowed |
|---|---|---|
| read only | `AUTH_READ_ROLES` | list, filters, open a resource |
| read & write | `AUTH_WRITE_ROLES` | plus create and update |
| full access | `AUTH_ADMIN_ROLES` | plus delete |

- An empty list does not restrict that level. If no list is set, every signed-in user can do everything. If only `AUTH_WRITE_ROLES` is set, every signed-in user can read, but only those roles can make changes.
- A higher level includes the lower ones.
- A user without a matching role sees a "No access" page.
- Buttons the user cannot use are hidden in the UI. The server still enforces the checks: a request that bypasses the UI gets `403`.

**Where roles come from:**
- from the `AUTH_ROLES_CLAIM` claim (`groups` by default): a list or a comma-separated string; a dot-separated path such as `realm_access.roles` is allowed;
- Keycloak roles are always collected: realm roles as is, client roles as `<client>:<role>`. So roles from a gatekeeper config work unchanged: `AUTH_READ_ROLES=gatekeeper:insight-get`;
- the leading `/` is stripped from Keycloak groups.

If gatekeeper used to protect the UI, you can remove it. The same roles are set with variables:

```
AUTH_READ_ROLES=gatekeeper:insight-get
AUTH_WRITE_ROLES=gatekeeper:insight-edit
AUTH_ADMIN_ROLES=gatekeeper:insight-admin
```

**Keycloak client setup:**
1. Clients → Create client: `Client ID` — `insight`, `Client authentication` — On, `Standard flow` — On.
2. `Valid redirect URIs`: `https://insight.example.com/auth/callback`.
3. `Valid post logout redirect URIs`: `https://insight.example.com/*`.
4. Copy the secret from the Credentials tab into `AUTH_OIDC_CLIENT_SECRET`.
5. Roles: use existing client roles (like `gatekeeper:insight-get`) or create your own. For another client's roles to appear in the insight token, the `insight` client must have `Full scope allowed` enabled (the default) or those roles must be added to its scope.

```yaml
          env:
            - name: AUTH_ENABLED
              value: "true"
            - name: PUBLIC_URL
              value: https://insight.example.com
            - name: AUTH_OIDC_ISSUER_URL
              value: https://keycloak.example.com/realms/main
            - name: AUTH_OIDC_CLIENT_ID
              value: insight
            - name: AUTH_READ_ROLES
              value: gatekeeper:insight-get
            - name: AUTH_WRITE_ROLES
              value: gatekeeper:insight-edit
            - name: AUTH_ADMIN_ROLES
              value: gatekeeper:insight-admin
          envFrom:
            - secretRef:
                name: insight-auth   # AUTH_OIDC_CLIENT_SECRET, AUTH_SESSION_SECRET (openssl rand -hex 32)
```

**Requests to state-manager** are made on behalf of insight when sign-in is enabled: the browser's `Authorization` header is dropped and `STATE_MANAGER_TOKEN` is used instead, if set. This is needed when state-manager has `AUTH_ENABLED=true`. The user's email is passed in `X-Forwarded-User` and written to the insight log for every request (`"user"`). Insight cookies are never forwarded to state-manager, in any mode.

**Without built-in sign-in**, behind a proxy: if oauth2-proxy sits in front of insight and state-manager has `AUTH_ENABLED=true`, the proxy must set `Authorization: Bearer <JWT>` (oauth2-proxy with `--pass-authorization-header`). Insight forwards it to state-manager unchanged.

**Reads go straight to the database**, so state-manager's `auth_role_rules` do not apply to them. Reads can be restricted only with insight roles.

## MCP for AI agents

`/mcp` is an MCP server (Streamable HTTP transport, sessionless) for Claude Code, Cursor and other agents. It is built on the same implementation as the insight methods and uses the same routes:

| Tool | What it does | Where it goes | Token |
|---|---|---|---|
| `search_resources` | Same as `POST /insight/v1/resources/search`: filters, labels, jsonb conditions, sorting, total. Same request body. | PostgreSQL, read only | read, write |
| `list_values` | Same as `POST /insight/v1/values`. | PostgreSQL, read only | read, write |
| `get_resource` | Full resource body. | state-manager `GET` | read, write |
| `create_resource` | Create. | state-manager `POST` | write |
| `update_resource` | Replace spec, labels, annotations, finalizers; `version` guards against concurrent edits. | state-manager `PUT` | write |
| `delete_resource` | Mark for deletion. | state-manager `DELETE` | write |

**Authorization is separate**, not through the proxy in front of the UI: the `Authorization: Bearer <token>` header.
- `MCP_READ_TOKEN`: the agent sees only the three read tools. It cannot call a write tool by name either: the server has none for this token.
- `MCP_WRITE_TOKEN`: all six tools.
- An invalid or missing token gets `401`. Every call is logged with the tool, access level and resource (`"msg":"mcp tool"`). Tokens themselves are never logged.

Differences from the UI: a jsonb condition without a value (other than `exists`/`not_exists`) returns an error instead of being ignored, so an agent never silently gets a broader result; `update_resource` without `spec` is rejected so the spec isn't wiped.

**Tokens** are best kept in a Secret:

```bash
kubectl -n state-manager create secret generic insight-mcp \
  --from-literal=MCP_READ_TOKEN="$(openssl rand -hex 32)" \
  --from-literal=MCP_WRITE_TOKEN="$(openssl rand -hex 32)"
```

```yaml
          envFrom:
            - secretRef:
                name: insight-mcp
```

To rotate, set the old and new tokens separated by a comma, switch clients over, then remove the old one.

**If an auth proxy sits in front of insight** (gatekeeper, oauth2-proxy), let `/mcp` through without its authentication: the agent has no browser session, and insight does the check itself. For gatekeeper, add a rule above `/*`:

```yaml
resources:
  - uri: /mcp
    white-listed: true
  - uri: /*
    ...
```

**Connecting an agent.** Claude Code:

```bash
claude mcp add --transport http insight https://insight.example.com/mcp \
  --header "Authorization: Bearer $INSIGHT_MCP_TOKEN"
```

Other clients (`mcp.json`):

```json
{
  "mcpServers": {
    "insight": {
      "type": "http",
      "url": "https://insight.example.com/mcp",
      "headers": { "Authorization": "Bearer ${INSIGHT_MCP_TOKEN}" }
    }
  }
}
```

Example agent request "find failed Jobs": `search_resources` with `{"filters":{"kind":"Job"},"json":[{"root":"status","path":"exit_code","op":"ne","value":"0"}]}`.

## Performance at scale

Measured on 10 million resources (5 GB) with the state-manager indexes:

| Query | Time |
|---|---|
| Page with no filters, by kind, group or a jsonb condition | 0–30 ms |
| Total count (`count(*)`) | 1–2 s per request |
| Opening the group, kind or namespace dropdown (`DISTINCT`) | about 1.5 s |
| Filter by a rare `shard_id` | up to 16 s, since there is no index on `shard_id` |
| Substring search by name | up to 12 s, since there is no trigram index |

Heavy queries are capped by `DATABASE_STATEMENT_TIMEOUT`. To keep long queries on a replica from being canceled by replication, tune `hot_standby_feedback` or `max_standby_streaming_delay`.

## Local environment

The environment uses [reconcile-kit/e2e-tests](https://github.com/reconcile-kit/e2e-tests) unchanged:
- its `docker-compose.yml` (postgres, redis, state-manager) is pulled in via `include`;
- dependencies are prepared by its `scripts/prepare-deps.sh`.

Insight uses the environment's database; no separate database is created.

```bash
make stand        # uses ../e2e-tests if present, otherwise clones it into build/e2e-tests
make seed         # 160 demo resources via the state-manager API
open http://127.0.0.1:57182
make stand-logs
make stand-down   # stop and delete data
```

| Service | Address |
|---|---|
| insight | http://127.0.0.1:57182 |
| state-manager | http://127.0.0.1:58080 |
| postgres | `postgres://e2e:e2e@127.0.0.1:55432/e2e` |

MCP in the local environment: `http://127.0.0.1:57182/mcp`, tokens `local-read-token-0000` (read) and `local-write-token-0000` (write):

```bash
claude mcp add --transport http insight-local http://127.0.0.1:57182/mcp \
  --header "Authorization: Bearer local-read-token-0000"
```

**Local environment with Keycloak sign-in:** `make stand-auth`. It also starts Keycloak (http://127.0.0.1:58180, admin `admin`/`admin`) with the `insight` realm from `deploy/keycloak/insight-realm.json`. Users (the password is the same as the user name):

| User | Roles | Access |
|---|---|---|
| `reader` | `gatekeeper:insight-get` | read only |
| `editor` | `+ gatekeeper:insight-edit` | read & write |
| `admin` | `+ gatekeeper:insight-admin` | full access, including delete |
| `nobody` | — | "No access" page |

The issuer is `http://host.docker.internal:58180/realms/insight`: this address resolves the same way from the browser and from the insight container. Docker Desktop adds `host.docker.internal` to `/etc/hosts`. If the entry is missing, add `127.0.0.1 host.docker.internal`.

- **Dependency versions** are set the same way as in e2e-tests, e.g. `STATE_MANAGER_REF=pr/42 make stand` or `STATE_MANAGER_REF=../state-manager make stand`.
- **Port conflicts.** The environment uses the same ports as `make e2e` in e2e-tests, so the two can't run at the same time.

Requires Docker with Compose v2.20+ (for `include`), git, curl.

## Development

Requires Go 1.25+ and Node.js 22+.

```bash
make stand                 # backend for the frontend
npm ci
make dev                   # Vite at http://localhost:5173, /insight and /api are proxied to the local insight
make build                 # frontend + bin/insight
make test                  # unit tests
make test-integration      # SQL tests against the local postgres, in a temporary insight_test schema
```

Layout:

```
cmd/insight         entry point
internal/config     environment variables
internal/query      SQL building: filters, label selector, jsonb conditions (parameters only, no value concatenation)
internal/store      query execution in READ ONLY transactions
internal/server     HTTP: /insight/v1, state-manager proxy, SPA serving
web/                embedded frontend (web/dist is built by Vite)
src/                frontend (React, Tailwind, CodeMirror)
```

## insight API

All methods accept JSON. An unknown field in the body gives `400`. A filter error (invalid path, selector or regex) is also returned as `400 {"error": "..."}`.

### `POST /insight/v1/resources/search`

```json
{
  "filters": {
    "name": "job-",
    "resource_group": "jobs.example.io",
    "kind": "Job",
    "namespace": "project-1",
    "shard_id": "shard-1",
    "pending": false,
    "deleting": false
  },
  "labels": "app=nginx, env in (prod,stage)",
  "json": [{ "root": "status", "path": "exit_code", "op": "ne", "value": "0" }],
  "json_match": "all",
  "sort": { "field": "created_at", "desc": true },
  "limit": 100,
  "offset": 0
}
```

- `name` is matched as a substring. `pending` means `version > current_version`; `deleting` means `deletion_timestamp` is set.
- `json[].root` is one of `spec`, `status`, `annotations`, `finalizers`.
- `json[].op` is one of `eq`, `ne`, `gt`, `gte`, `lt`, `lte`, `contains`, `not_contains`, `matches`, `in`, `exists`, `not_exists`.
- `json_match` is `all` or `any`.
- `sort.field` is one of `name`, `kind`, `resource_group`, `namespace`, `shard_id`, `created_at`, `updated_at`, `version`.
- `limit` defaults to 100, maximum 1000.

Response: `{"items": [...], "total": 123}`. `items` contain no `spec` or `status`; the full body comes from the state-manager `GET`. `total` is `null` if counting did not finish within the timeout; the reason is then in `count_error`.

### `POST /insight/v1/values`

```json
{ "field": "kind", "q": "job", "search": { "filters": { "namespace": "project-1" } }, "limit": 200 }
```

- `field` is one of `resource_group`, `kind`, `namespace`, `shard_id`. The filter on `field` itself is not applied; the other filters from `search` are.
- `q` is a substring.

Response: `{"values": [{"value": "Job", "group": "jobs.example.io"}]}`. The `group` field is present only for `kind`.

### Service endpoints

`GET /healthz` checks that the process is alive. `GET /readyz` checks the database connection.

## Troubleshooting

| Symptom | Cause |
|---|---|
| `/readyz` returns `503` | The database is unreachable or the credentials are wrong. The error message is in the response body. |
| The list doesn't load: `permission denied for table resources` | The role lacks the privileges from step [1](#1-postgresql-role). |
| Create or save returns `401`/`403` | state-manager has authorization enabled, but no token is passed. See [Access and authorization](#access-and-authorization). |
| Create or save returns `502 state-manager unavailable` | Wrong `STATE_MANAGER_URL` or no network access to state-manager. |
| Total shows "couldn't count" | `count(*)` did not finish within `DATABASE_STATEMENT_TIMEOUT`. Narrow the filters or increase the timeout. |
| `/readyz` returns `503`: `unsupported startup parameter: statement_timeout` (or `default_transaction_read_only`) | Insight `0.3.0` behind PgBouncer. Upgrade to `0.3.1` and don't pass server parameters in `DATABASE_URL`. |
| `prepared statement "…" does not exist` | PgBouncer in `transaction` mode without prepared statement support, and `DATABASE_URL` sets `default_query_exec_mode=cache_statement`. Remove that parameter. |
| After signing in, the sign-in page shows again | The cookie isn't saved: `AUTH_COOKIE_SECURE=true` while insight is opened over http. Or `PUBLIC_URL` doesn't match the address in the browser. |
| `/login?error=provider_unavailable` | Insight cannot fetch `AUTH_OIDC_ISSUER_URL/.well-known/openid-configuration`. Check network access from the pod to the provider. |
| `/login?error=exchange_failed` | Wrong `AUTH_OIDC_CLIENT_SECRET`, or the client redirect URI doesn't match `PUBLIC_URL/auth/callback`. Details are in the insight log. |
| `/login?error=invalid_token` | The issuer in the token doesn't match `AUTH_OIDC_ISSUER_URL`. The provider address must be the same for the browser and for insight. |
| Keycloak: `Invalid parameter: redirect_uri` | `PUBLIC_URL/auth/callback` is missing from the client's `Valid redirect URIs`. |
| Signed in, but "No access" | The token has none of the roles from `AUTH_*_ROLES`. The roles received in the token are in the insight log: `"msg":"auth: login"`, field `roles`. |
| `/mcp` returns `404` `MCP is disabled` | Neither `MCP_READ_TOKEN` nor `MCP_WRITE_TOKEN` is set. |
| `/mcp` returns `401` | Wrong token or no `Authorization: Bearer …` header. If an auth proxy sits in front of insight and the response is its sign-in page, let `/mcp` through (see [MCP](#mcp-for-ai-agents)). |
| MCP write tools return `state-manager 401` | state-manager has authorization enabled; set `STATE_MANAGER_TOKEN`. |
| `canceling statement due to conflict with recovery` | The query on the replica was canceled by replication. See [Performance](#performance-at-scale). |
