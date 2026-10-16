# Insight

Insight — веб-интерфейс для просмотра, поиска и редактирования ресурсов [state-manager](https://github.com/reconcile-kit/state-manager).

- [Как это работает](#как-это-работает)
- [Установка](#установка)
  - [Что нужно](#что-нужно)
  - [1. Роль в PostgreSQL](#1-роль-в-postgresql)
  - [2. Образ](#2-образ)
  - [3. Запуск](#3-запуск)
  - [4. Проверка](#4-проверка)
- [Параметры](#параметры)
- [Доступ и авторизация](#доступ-и-авторизация)
- [Производительность на больших объёмах](#производительность-на-больших-объёмах)
- [Локальный стенд](#локальный-стенд)
- [Разработка](#разработка)
- [API insight](#api-insight)
- [Частые проблемы](#частые-проблемы)

## Как это работает

Insight — один Go-бинарь со встроенным фронтендом (React). Внешних зависимостей, кроме PostgreSQL и state-manager, нет.

```
браузер ────► insight ─┬─► PostgreSQL (реплика)   чтение: список, фильтры, счётчики
AI-агент (MCP) ┘       └─► state-manager          GET одного ресурса, создание, изменение, удаление
```

| Запрос | Куда идёт |
|---|---|
| `POST /insight/v1/resources/search`, `POST /insight/v1/values` | Напрямую в PostgreSQL, **только чтение**. |
| `GET/POST/PUT/DELETE /api/v1/groups/...` | Проксируется в state-manager без изменений. |
| Остальные `/api/*`, включая list state-manager | `404`. |
| `/login`, `/auth/*` | Вход через OIDC, если включён. Без сессии всё остальное, кроме `/mcp` и проб, недоступно. |
| `POST /mcp` | MCP для AI-агентов, своя авторизация по токенам. Внутри та же логика: чтение из PostgreSQL, GET и запись через state-manager. См. [MCP](#mcp-для-ai-агентов). |

- API state-manager не меняется и не получает тяжёлых запросов. Сложные фильтры выполняет insight на своей стороне.
- Писать в БД insight не может: каждый запрос выполняется в `READ ONLY` транзакции. Всё равно подключайте его к реплике под read-only ролью.
- Работает напрямую с PostgreSQL и через пулеры (PgBouncer в любом `pool_mode`), см. [PgBouncer и другие пулеры](#pgbouncer-и-другие-пулеры).

### Возможности

- Фильтры по group, kind, namespace и shard. Значения в выпадающих списках берутся из БД с учётом остальных фильтров и подсвечены теми же цветами, что в таблице.
- Поиск по name по подстроке.
- Labels-селектор: `app=nginx, tier!=db, env in (a,b), env notin (c), team, !legacy, app~ngi`.
- Условия по jsonb-полям (`spec`, `status`, `annotations`, `finalizers`):
  - операторы `= ≠ > ≥ < ≤ contains !contains regex in exists !exists`;
  - объединение условий через И / ИЛИ;
  - путь пишется вручную: `resources.cpu`, `conditions[0].type`, `["app.kubernetes.io/name"]`.
- Общее количество найденных ресурсов, серверная сортировка по клику на заголовок колонки, пагинация.
- Клик по значению в ячейке или по label добавляет фильтр. `Enter` в любом поле применяет фильтры.
- Ресурс открывается в боковой панели, тело каждый раз заново загружается из state-manager. Редактор JSON с подсветкой, сохранение по `⌘S`.
- Светлая и тёмная тема. Фильтры хранятся в URL, ссылкой можно поделиться.
- MCP-сервер `/mcp` для AI-агентов с отдельными токенами на чтение и на запись.
- Вход через Keycloak или другой OIDC-провайдер, уровни доступа «чтение / запись / удаление» по ролям.

## Установка

### Что нужно

- **PostgreSQL state-manager.** Лучше реплика. Insight читает только таблицы `resources` и `labels`. Схему создаёт и мигрирует сам state-manager, insight миграций не выполняет.
- **Доступ к HTTP API state-manager** из пода или контейнера insight.
- **Ресурсы:** порт `27182`, образ около 25 МБ. Запаса в 64 МБ памяти и 0,1 CPU обычно хватает.

### 1. Роль в PostgreSQL

Создайте роль на **primary**, она реплицируется вместе с данными. Если БД state-manager называется не `state_manager`, подставьте своё имя:

```sql
CREATE ROLE insight_ro LOGIN PASSWORD '<пароль>';
GRANT CONNECT ON DATABASE state_manager TO insight_ro;
GRANT USAGE ON SCHEMA public TO insight_ro;
GRANT SELECT ON public.resources, public.labels TO insight_ro;
-- необязательно: дополнительная защита от записи на стороне сервера
ALTER ROLE insight_ro SET default_transaction_read_only = on;
```

Других прав не нужно.

### 2. Образ

Образ собирается GitHub Actions при пуше тега `v*` и публикуется в Docker Hub для `linux/amd64` и `linux/arm64`:

```
docker.io/dhnikolas/insight:<версия>      # git-тег v0.3.1 → образ 0.3.1
```

Версия с Go-бэкендом и прямым чтением из БД начинается с **0.3.0**; через PgBouncer работает начиная с **0.3.1**. Образы до `0.2.1` включительно — прежняя сборка: nginx проксирует весь `/api` в state-manager, нужен только `STATE_MANAGER_URL`, БД не используется. Список версий: [теги репозитория](https://github.com/reconcile-kit/insight/tags).

Собрать самостоятельно:

```bash
docker build -t insight .
```

### 3. Запуск

Нужно задать два обязательных параметра: подключение к БД и адрес state-manager. Полный список — в разделе [Параметры](#параметры).

**Docker**

```bash
docker run -d --name insight -p 27182:27182 \
  -e STATE_MANAGER_URL=http://state-manager:8080 \
  -e DATABASE_HOST=pg-replica.internal \
  -e DATABASE_USERNAME=insight_ro \
  -e DATABASE_PASSWORD='<пароль>' \
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
  DATABASE_URL: postgres://insight_ro:<пароль>@pg-replica.internal:5432/state_manager?sslmode=require
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

Наружу insight публикуется через Ingress, но обязательно за аутентификацией. Подробнее — в разделе [Доступ и авторизация](#доступ-и-авторизация).

Под стейтлесс, поэтому можно запускать несколько реплик. Учтите, что каждая открывает до `DATABASE_MAX_CONNS` соединений к БД.

### 4. Проверка

```bash
curl -i http://<insight>/healthz     # 200 — процесс жив
curl -i http://<insight>/readyz      # 200 — БД доступна; 503 и текст ошибки — нет
curl -s -X POST http://<insight>/insight/v1/resources/search -d '{"limit":1}'
```

Если обязательный параметр не задан, insight сразу завершается и пишет причину в лог:

```
{"level":"ERROR","msg":"insight stopped","err":"STATE_MANAGER_URL is required"}
{"level":"ERROR","msg":"insight stopped","err":"set DATABASE_URL or DATABASE_USERNAME, DATABASE_PASSWORD and DATABASE_NAME"}
```

Недоступность БД при старте не роняет процесс. В этом случае `/readyz` отвечает `503`, пока соединение не появится.

### 5. Вход через Keycloak

Необязательный шаг. Без него вход выключен и UI открыт всем, у кого есть к нему сетевой доступ. Подробности и другие провайдеры описаны в разделе [Вход через Keycloak или другой OIDC-провайдер](#вход-через-keycloak-или-другой-oidc-провайдер).

**1. Клиент в Keycloak.** Clients → Create client:
- `Client ID`: `insight`;
- `Client authentication`: On, `Standard flow`: On;
- `Valid redirect URIs`: `https://insight.example.com/auth/callback`;
- `Valid post logout redirect URIs`: `https://insight.example.com/*`.

Секрет клиента находится на вкладке Credentials.

**2. Роли.** Можно оставить роли gatekeeper (`gatekeeper:insight-get`, `insight-edit`, `insight-admin`) или завести клиентские роли у клиента `insight`, например `insight:read`, `insight:write`, `insight:admin`, и выдать их пользователям или группам.

**3. Секрет.**

```bash
kubectl -n state-manager create secret generic insight-auth \
  --from-literal=AUTH_OIDC_CLIENT_SECRET='<секрет клиента из Keycloak>' \
  --from-literal=AUTH_SESSION_SECRET="$(openssl rand -hex 32)"
```

**4. Переменные в Deployment** (в дополнение к шагу [3](#3-запуск)):

```yaml
          env:
            - name: AUTH_ENABLED
              value: "true"
            - name: PUBLIC_URL                  # адрес insight в браузере
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
            - name: STATE_MANAGER_TOKEN         # только если в state-manager AUTH_ENABLED=true
              valueFrom:
                secretKeyRef: { name: insight-state-manager, key: token }
          envFrom:
            - secretRef:
                name: insight-auth
```

**5. Gatekeeper.** Если UI раньше закрывал gatekeeper, уберите его: insight теперь проверяет вход и роли сам. Если gatekeeper остаётся, пропустите через него без проверки `/login`, `/auth/*`, `/assets/*` и `/mcp`.

**6. Проверка.**
- В журнале при старте: `"msg":"login enabled"` с issuer и ролями.
- Открытие insight перенаправляет на `/login` с кнопкой «Продолжить через SSO».
- После входа в журнале появляется `"msg":"auth: login"` с полями `access` и `roles`. Если `access` равен `none`, сверьте `roles` с `AUTH_*_ROLES`.

### 6. Токены для MCP

Необязательный шаг. Он включает `/mcp` для AI-агентов. От входа через Keycloak он не зависит. Подробности — в разделе [MCP для AI-агентов](#mcp-для-ai-агентов).

**1. Токены.** Токен на чтение и токен на чтение и запись:

```bash
kubectl -n state-manager create secret generic insight-mcp \
  --from-literal=MCP_READ_TOKEN="$(openssl rand -hex 32)" \
  --from-literal=MCP_WRITE_TOKEN="$(openssl rand -hex 32)"
```

Достаточно одного из двух. Например, только `MCP_READ_TOKEN`, если агентам не нужно ничего менять.

**2. Переменные в Deployment:**

```yaml
          envFrom:
            - secretRef:
                name: insight-mcp
```

Если в state-manager `AUTH_ENABLED=true`, задайте также `STATE_MANAGER_TOKEN` (см. шаг 5): с ним MCP обращается к state-manager.

**3. Проверка.** В журнале при старте `mcp_read_tokens` и `mcp_write_tokens` больше нуля. Запрос со списком инструментов:

```bash
curl -s https://insight.example.com/mcp \
  -H "Authorization: Bearer $MCP_READ_TOKEN" \
  -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

С токеном на чтение в ответе три инструмента (`search_resources`, `list_values`, `get_resource`), с токеном на запись — шесть.

**4. Подключение агента** (Claude Code):

```bash
claude mcp add --transport http insight https://insight.example.com/mcp \
  --header "Authorization: Bearer $MCP_READ_TOKEN"
```

**Смена токена без простоя:** задайте старый и новый токен через запятую (`MCP_READ_TOKEN=old,new`), переключите агентов на новый и уберите старый.

## Параметры

Все параметры задаются переменными окружения.

**Обязательные**

| Переменная | Пример | Описание |
|---|---|---|
| `STATE_MANAGER_URL` | `http://state-manager:8080` | Адрес state-manager: схема, хост и порт. Путь `/api/v1/...` добавляется к нему. |
| `DATABASE_URL` | `postgres://insight_ro:pass@replica:5432/state_manager?sslmode=require` | Подключение к PostgreSQL. Вместо него можно задать переменные `DATABASE_*` ниже. |

**Подключение к БД по частям** (используются, только если `DATABASE_URL` не задан; это те же переменные, что у state-manager)

| Переменная | По умолчанию | Описание |
|---|---|---|
| `DATABASE_HOST` | `localhost` | Хост реплики. |
| `DATABASE_PORT` | `5432` | |
| `DATABASE_USERNAME` | — (обязательна) | Read-only роль. |
| `DATABASE_PASSWORD` | — | |
| `DATABASE_NAME` | — (обязательна) | |
| `DATABASE_SSLMODE` | `disable` | `disable`, `require`, `verify-ca` или `verify-full`. |

**Необязательные**

| Переменная | По умолчанию | Описание |
|---|---|---|
| `LISTEN_ADDR` | `:27182` | Адрес HTTP-сервера. |
| `DATABASE_MAX_CONNS` | `10` | Максимум соединений к БД на один под. |
| `DATABASE_STATEMENT_TIMEOUT` | `15s` | Предельное время одного SQL-запроса. Если подсчёт total не укладывается, список всё равно возвращается, но без total, а в UI показывается «не удалось посчитать». |
| `MCP_READ_TOKEN` | — | Токен MCP только на чтение. Можно несколько через запятую (для ротации). Минимум 16 символов. |
| `MCP_WRITE_TOKEN` | — | Токен MCP на чтение и запись. Можно несколько через запятую. Не должен совпадать с токеном на чтение. Если не задан ни один `MCP_*_TOKEN`, `/mcp` отвечает `404`. |
| `STATE_MANAGER_TOKEN` | — | Bearer-токен для state-manager (нужен, если там `AUTH_ENABLED=true`). Его используют MCP и, при включённом входе, UI. |

**Вход через OIDC** (см. [Доступ и авторизация](#доступ-и-авторизация))

| Переменная | По умолчанию | Описание |
|---|---|---|
| `AUTH_ENABLED` | `false` | `true` включает вход. Без сессии открывается только страница `/login`. |
| `PUBLIC_URL` | — (обязательна) | Адрес, по которому пользователи открывают insight, например `https://insight.example.com`. Redirect URI — `PUBLIC_URL/auth/callback`. |
| `AUTH_OIDC_ISSUER_URL` | — (обязательна) | Issuer провайдера, например `https://keycloak.example.com/realms/main`. |
| `AUTH_OIDC_CLIENT_ID` | — (обязательна) | |
| `AUTH_OIDC_CLIENT_SECRET` | — | Секрет клиента. Для public-клиента не нужен, PKCE используется всегда. |
| `AUTH_SESSION_SECRET` | — (обязательна) | Ключ подписи сессии, не короче 32 символов: `openssl rand -hex 32`. Одинаковый на всех подах. Смена ключа завершает все сессии. |
| `AUTH_SESSION_TTL` | `12h` | Время жизни сессии. Роли перечитываются при следующем входе. |
| `AUTH_OIDC_SCOPES` | `openid,email,profile` | Добавьте `groups` или свой scope, если провайдер отдаёт группы только по нему. |
| `AUTH_ROLES_CLAIM` | `groups` | Claim с ролями; путь через точку. Роли Keycloak (`realm_access`, `resource_access`) собираются всегда. |
| `AUTH_READ_ROLES`, `AUTH_WRITE_ROLES`, `AUTH_ADMIN_ROLES` | — | Роли для чтения, записи и удаления, через запятую. Пустой список уровень не ограничивает. |
| `AUTH_COOKIE_SECURE` | `true` для https | Флаг `Secure` у cookie. |

В `DATABASE_URL` можно передавать [параметры libpq/pgx](https://pkg.go.dev/github.com/jackc/pgx/v5/pgconn#ParseConfig), например `sslrootcert=/certs/ca.pem` или `target_session_attrs=read-only`.

### PgBouncer и другие пулеры

Insight работает через PgBouncer в режимах `session` и `transaction`, в том числе при `max_prepared_statements = 0`:
- при подключении передаётся только `application_name`; `statement_timeout` задаётся внутри каждой транзакции (`SET LOCAL`), поэтому `DATABASE_STATEMENT_TIMEOUT` действует и через пулер;
- именованные подготовленные запросы не используются (pgx `QueryExecModeExec`). Переопределить можно параметром `default_query_exec_mode` в `DATABASE_URL`.

Не добавляйте в `DATABASE_URL` серверные параметры вроде `search_path` или `options`: PgBouncer отклонит подключение с `unsupported startup parameter`.

## Доступ и авторизация

По умолчанию вход выключен: любой, кто открыл UI, может читать все ресурсы и менять их через state-manager. В таком режиме не публикуйте insight наружу, держите его во внутренней сети, за VPN или за auth-прокси.

### Вход через Keycloak или другой OIDC-провайдер

С `AUTH_ENABLED=true` insight сам выполняет вход через OpenID Connect: Keycloak, Dex, Okta, Entra ID, Google и любой другой провайдер с discovery (`/.well-known/openid-configuration`). Отдельный прокси вроде gatekeeper или oauth2-proxy не нужен.

- Неавторизованный пользователь попадает на страницу `/login` с кнопкой «Продолжить через SSO». После входа он возвращается туда, откуда пришёл, вместе с фильтрами из URL.
- Используется authorization code flow с PKCE, `state` и `nonce`. После входа insight выдаёт свою cookie `insight_session` (HttpOnly, SameSite=Lax, подпись HMAC-SHA256) со сроком `AUTH_SESSION_TTL`. Токены провайдера не хранятся. Исключение — ID token в cookie с `Path=/auth`: он нужен только как `id_token_hint` при выходе.
- «Выйти» завершает и сессию у провайдера (RP-initiated logout через `end_session_endpoint`), поэтому следующий вход снова спросит пароль.
- Запросы на изменение с чужих сайтов отклоняются (проверка `Origin` и `Sec-Fetch-Site`).
- `/mcp` не затрагивается: у него свои токены, см. [MCP](#mcp-для-ai-агентов).

**Уровни доступа** задаются ролями из токена. Подходит любая из перечисленных ролей:

| Уровень | Переменная | Что можно |
|---|---|---|
| чтение | `AUTH_READ_ROLES` | список, фильтры, открыть ресурс |
| запись | `AUTH_WRITE_ROLES` | плюс создание и изменение |
| удаление | `AUTH_ADMIN_ROLES` | плюс удаление |

- Пустой список уровень не ограничивает. Если не задан ни один список, всем вошедшим доступно всё. Если задан только `AUTH_WRITE_ROLES`, все вошедшие читают, а меняют только эти роли.
- Роль более высокого уровня включает нижние.
- Без подходящей роли пользователь увидит страницу «нет доступа».
- Кнопки, которые пользователю недоступны, в UI скрыты. Проверку всё равно делает сервер: запрос в обход UI получит `403`.

**Откуда берутся роли:**
- из claim `AUTH_ROLES_CLAIM` (по умолчанию `groups`): список или строка через запятую, можно указать путь через точку, например `realm_access.roles`;
- роли Keycloak собираются всегда: роли realm — как есть, клиентские роли — в виде `<клиент>:<роль>`. Поэтому роли из конфигурации gatekeeper подходят без изменений: `AUTH_READ_ROLES=gatekeeper:insight-get`;
- у групп Keycloak отбрасывается ведущий `/`.

Если раньше UI закрывал gatekeeper, его можно убрать. Те же роли задаются переменными:

```
AUTH_READ_ROLES=gatekeeper:insight-get
AUTH_WRITE_ROLES=gatekeeper:insight-edit
AUTH_ADMIN_ROLES=gatekeeper:insight-admin
```

**Настройка клиента в Keycloak:**
1. Clients → Create client: `Client ID` — `insight`, `Client authentication` — On, `Standard flow` — On.
2. `Valid redirect URIs`: `https://insight.example.com/auth/callback`.
3. `Valid post logout redirect URIs`: `https://insight.example.com/*`.
4. Скопируйте секрет из вкладки Credentials в `AUTH_OIDC_CLIENT_SECRET`.
5. Роли: используйте существующие клиентские роли (как `gatekeeper:insight-get`) или заведите свои. Чтобы клиентские роли другого клиента попали в токен insight, у клиента `insight` должен быть включён `Full scope allowed` (по умолчанию включён) или эти роли должны быть добавлены в его scope.

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

**Запросы в state-manager** при включённом входе идут от имени insight: заголовок `Authorization` из браузера отбрасывается, вместо него подставляется `STATE_MANAGER_TOKEN`, если он задан. Это нужно, когда в state-manager `AUTH_ENABLED=true`. Email пользователя передаётся в `X-Forwarded-User` и пишется в журнал insight у каждого запроса (`"user"`). Cookie insight в state-manager не передаются ни в каком режиме.

**Без встроенного входа**, за прокси: если перед insight стоит oauth2-proxy и в state-manager `AUTH_ENABLED=true`, прокси должен подставлять `Authorization: Bearer <JWT>` (oauth2-proxy с `--pass-authorization-header`). Insight передаст его в state-manager без изменений.

**Чтение идёт напрямую из БД**, и правила `auth_role_rules` state-manager к нему не применяются. Ограничить чтение можно только ролями insight.

## MCP для AI-агентов

`/mcp` — MCP-сервер (транспорт Streamable HTTP, без сессий) для Claude Code, Cursor и других агентов. Он построен на той же реализации, что и методы insight, и ходит по тем же маршрутам:

| Инструмент | Что делает | Куда идёт | Токен |
|---|---|---|---|
| `search_resources` | То же, что `POST /insight/v1/resources/search`: фильтры, labels, условия по jsonb, сортировка, total. Тело запроса такое же. | PostgreSQL, только чтение | read, write |
| `list_values` | То же, что `POST /insight/v1/values`. | PostgreSQL, только чтение | read, write |
| `get_resource` | Полное тело ресурса. | state-manager `GET` | read, write |
| `create_resource` | Создание. | state-manager `POST` | write |
| `update_resource` | Замена spec, labels, annotations, finalizers; `version` для защиты от одновременных правок. | state-manager `PUT` | write |
| `delete_resource` | Пометка на удаление. | state-manager `DELETE` | write |

**Авторизация своя**, не через прокси перед UI: заголовок `Authorization: Bearer <токен>`.
- `MCP_READ_TOKEN` — агент видит только три инструмента чтения. Вызвать инструмент записи по имени тоже нельзя: у сервера для этого токена их нет.
- `MCP_WRITE_TOKEN` — все шесть инструментов.
- Неверный или отсутствующий токен — `401`. Каждый вызов пишется в лог: инструмент, уровень доступа и ресурс (`"msg":"mcp tool"`). Сами токены в лог не попадают.

Отличия от UI: условие по jsonb без значения (кроме `exists`/`not_exists`) возвращает ошибку, а не игнорируется, чтобы агент не получил молча расширенную выборку; `update_resource` без `spec` отклоняется, чтобы не затереть spec.

**Токены** удобно хранить в Secret:

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

Для ротации задайте старый и новый токен через запятую, переключите клиентов и уберите старый.

**Если перед insight стоит auth-прокси** (gatekeeper, oauth2-proxy), пропустите `/mcp` без его аутентификации — у агента нет браузерной сессии, проверку делает сам insight. Для gatekeeper — правило выше `/*`:

```yaml
resources:
  - uri: /mcp
    white-listed: true
  - uri: /*
    ...
```

**Подключение агента.** Claude Code:

```bash
claude mcp add --transport http insight https://insight.example.com/mcp \
  --header "Authorization: Bearer $INSIGHT_MCP_TOKEN"
```

Другие клиенты (`mcp.json`):

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

Пример запроса агента «найди упавшие Job»: `search_resources` с `{"filters":{"kind":"Job"},"json":[{"root":"status","path":"exit_code","op":"ne","value":"0"}]}`.

## Производительность на больших объёмах

Замер на 10 млн ресурсов (5 ГБ) с индексами state-manager:

| Запрос | Время |
|---|---|
| Страница без фильтров, по kind, group или jsonb-условию | 0–30 мс |
| Подсчёт total (`count(*)`) | 1–2 с на каждый запрос |
| Открытие выпадающего списка group, kind или namespace (`DISTINCT`) | около 1,5 с |
| Фильтр по редкому `shard_id` | до 16 с, так как индекса по `shard_id` нет |
| Поиск name по подстроке | до 12 с, так как нет trigram-индекса |

Тяжёлые запросы ограничены `DATABASE_STATEMENT_TIMEOUT`. Чтобы долгие запросы на реплике не отменялись при репликации, настройте `hot_standby_feedback` или `max_standby_streaming_delay`.

## Локальный стенд

Стенд использует [reconcile-kit/e2e-tests](https://github.com/reconcile-kit/e2e-tests) без изменений:
- его `docker-compose.yml` (postgres, redis, state-manager) подключается через `include`;
- зависимости готовит его `scripts/prepare-deps.sh`.

Insight работает с базой стенда, отдельная база не создаётся.

```bash
make stand        # берёт ../e2e-tests, если он есть, иначе клонирует в build/e2e-tests
make seed         # 160 демо-ресурсов через API state-manager
open http://127.0.0.1:57182
make stand-logs
make stand-down   # остановить и удалить данные
```

| Сервис | Адрес |
|---|---|
| insight | http://127.0.0.1:57182 |
| state-manager | http://127.0.0.1:58080 |
| postgres | `postgres://e2e:e2e@127.0.0.1:55432/e2e` |

MCP на стенде: `http://127.0.0.1:57182/mcp`, токены `local-read-token-0000` (чтение) и `local-write-token-0000` (запись):

```bash
claude mcp add --transport http insight-local http://127.0.0.1:57182/mcp \
  --header "Authorization: Bearer local-read-token-0000"
```

**Стенд со входом через Keycloak:** `make stand-auth`. Дополнительно поднимается Keycloak (http://127.0.0.1:58180, администратор `admin`/`admin`) с realm `insight` из `deploy/keycloak/insight-realm.json`. Пользователи (пароль совпадает с именем):

| Пользователь | Роли | Доступ |
|---|---|---|
| `reader` | `gatekeeper:insight-get` | чтение |
| `editor` | `+ gatekeeper:insight-edit` | чтение и запись |
| `admin` | `+ gatekeeper:insight-admin` | всё, включая удаление |
| `nobody` | — | страница «нет доступа» |

Issuer — `http://host.docker.internal:58180/realms/insight`: этот адрес одинаково открывается из браузера и из контейнера insight. Docker Desktop прописывает `host.docker.internal` в `/etc/hosts`. Если записи нет, добавьте `127.0.0.1 host.docker.internal`.

- **Версии зависимостей** задаются так же, как в e2e-tests, например `STATE_MANAGER_REF=pr/42 make stand` или `STATE_MANAGER_REF=../state-manager make stand`.
- **Конфликт портов.** Стенд занимает те же порты, что `make e2e` в e2e-tests, поэтому одновременно их не запустить.

Нужно: Docker с Compose v2.20+ (из-за `include`), git, curl.

## Разработка

Нужно: Go 1.25+ и Node.js 22+.

```bash
make stand                 # бэкенд для фронтенда
npm ci
make dev                   # Vite на http://localhost:5173, /insight и /api проксируются в insight стенда
make build                 # фронтенд + bin/insight
make test                  # unit-тесты
make test-integration      # SQL-тесты на postgres стенда, во временной схеме insight_test
```

Структура:

```
cmd/insight         точка входа
internal/config     переменные окружения
internal/query      построение SQL: фильтры, labels-селектор, jsonb-условия (только параметры, без конкатенации значений)
internal/store      выполнение запросов в READ ONLY транзакциях
internal/server     HTTP: /insight/v1, прокси в state-manager, раздача SPA
web/                встроенный фронтенд (web/dist собирается Vite)
src/                фронтенд (React, Tailwind, CodeMirror)
```

## API insight

Все методы принимают JSON. Неизвестное поле в теле даёт `400`. Ошибка в фильтре (неверный путь, селектор или regex) тоже возвращается как `400 {"error": "..."}`.

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

- `name` ищется по подстроке. `pending` означает `version > current_version`, `deleting` — что задан `deletion_timestamp`.
- `json[].root` — одно из `spec`, `status`, `annotations`, `finalizers`.
- `json[].op` — одно из `eq`, `ne`, `gt`, `gte`, `lt`, `lte`, `contains`, `not_contains`, `matches`, `in`, `exists`, `not_exists`.
- `json_match` — `all` или `any`.
- `sort.field` — одно из `name`, `kind`, `resource_group`, `namespace`, `shard_id`, `created_at`, `updated_at`, `version`.
- `limit` по умолчанию 100, максимум 1000.

Ответ: `{"items": [...], "total": 123}`. В `items` нет `spec` и `status`, полное тело берётся через `GET` state-manager. `total` равен `null`, если подсчёт не уложился в таймаут; причина тогда в `count_error`.

### `POST /insight/v1/values`

```json
{ "field": "kind", "q": "job", "search": { "filters": { "namespace": "project-1" } }, "limit": 200 }
```

- `field` — одно из `resource_group`, `kind`, `namespace`, `shard_id`. Фильтр по самому `field` не применяется, остальные фильтры из `search` применяются.
- `q` — подстрока.

Ответ: `{"values": [{"value": "Job", "group": "jobs.example.io"}]}`. Поле `group` есть только у `kind`.

### Служебные

`GET /healthz` — проверка, что процесс жив. `GET /readyz` — проверка соединения с БД.

## Частые проблемы

| Симптом | Причина |
|---|---|
| `/readyz` — `503` | БД недоступна или неверные учётные данные. Текст ошибки — в теле ответа. |
| Список не грузится: `permission denied for table resources` | Роли не выданы права из шага [1](#1-роль-в-postgresql). |
| Создание или сохранение возвращает `401`/`403` | В state-manager включена авторизация, а токен не передаётся. См. [Доступ и авторизация](#доступ-и-авторизация). |
| Создание или сохранение возвращает `502 state-manager unavailable` | Неверный `STATE_MANAGER_URL` или нет сетевого доступа к state-manager. |
| Total показывает «не удалось посчитать» | `count(*)` не уложился в `DATABASE_STATEMENT_TIMEOUT`. Сузьте фильтры или увеличьте таймаут. |
| `/readyz` — `503`: `unsupported startup parameter: statement_timeout` (или `default_transaction_read_only`) | Insight `0.3.0` за PgBouncer. Обновитесь до `0.3.1` и не передавайте серверные параметры в `DATABASE_URL`. |
| `prepared statement "…" does not exist` | PgBouncer в режиме `transaction` без поддержки prepared statements, а в `DATABASE_URL` задан `default_query_exec_mode=cache_statement`. Уберите этот параметр. |
| После входа — снова страница входа | Cookie не сохраняется: `AUTH_COOKIE_SECURE=true`, а insight открыт по http. Или `PUBLIC_URL` не совпадает с адресом в браузере. |
| `/login?error=provider_unavailable` | Insight не может получить `AUTH_OIDC_ISSUER_URL/.well-known/openid-configuration`. Проверьте сетевой доступ из пода к провайдеру. |
| `/login?error=exchange_failed` | Неверный `AUTH_OIDC_CLIENT_SECRET` или redirect URI клиента не совпадает с `PUBLIC_URL/auth/callback`. Подробности — в журнале insight. |
| `/login?error=invalid_token` | Issuer в токене не совпадает с `AUTH_OIDC_ISSUER_URL`. Адрес провайдера должен быть одинаковым для браузера и для insight. |
| Keycloak: `Invalid parameter: redirect_uri` | В клиенте не указан `PUBLIC_URL/auth/callback` в `Valid redirect URIs`. |
| Вошёл, но «нет доступа» | В токене нет ни одной роли из `AUTH_*_ROLES`. Роли, которые пришли в токене, видны в журнале insight: `"msg":"auth: login"`, поле `roles`. |
| `/mcp` — `404` `MCP is disabled` | Не задан ни `MCP_READ_TOKEN`, ни `MCP_WRITE_TOKEN`. |
| `/mcp` — `401` | Неверный токен или нет заголовка `Authorization: Bearer …`. Если перед insight стоит auth-прокси и ответ — его страница входа, пропустите `/mcp` (см. [MCP](#mcp-для-ai-агентов)). |
| Инструменты записи MCP возвращают `state-manager 401` | В state-manager включена авторизация, задайте `STATE_MANAGER_TOKEN`. |
| `canceling statement due to conflict with recovery` | Запрос на реплике отменён репликацией. См. [Производительность](#производительность-на-больших-объёмах). |
