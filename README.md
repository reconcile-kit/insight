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
браузер ──► insight ─┬─► PostgreSQL (реплика)   чтение: список, фильтры, счётчики
                     └─► state-manager          GET одного ресурса, создание, изменение, удаление
```

| Запрос | Куда идёт |
|---|---|
| `POST /insight/v1/resources/search`, `POST /insight/v1/values` | Напрямую в PostgreSQL, **только чтение**. |
| `GET/POST/PUT/DELETE /api/v1/groups/...` | Проксируется в state-manager без изменений. |
| Остальные `/api/*`, включая list state-manager | `404`. |

- API state-manager не меняется и не получает тяжёлых запросов. Сложные фильтры выполняет insight на своей стороне.
- Писать в БД insight не может:
  - сессии открываются с `default_transaction_read_only=on`;
  - каждый запрос идёт в `READ ONLY` транзакции.

  Всё равно подключайте его к реплике под read-only ролью.

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
```

Других прав не нужно.

### 2. Образ

Образ собирается GitHub Actions при пуше тега `v*` и публикуется в Docker Hub для `linux/amd64` и `linux/arm64`:

```
docker.io/dhnikolas/insight:<версия>      # git-тег v0.3.0 → образ 0.3.0
```

Версия с Go-бэкендом и прямым чтением из БД начинается с **0.3.0**. Этот тег ставится после мержа в `main`. Образы до `0.2.1` включительно — прежняя сборка: nginx проксирует весь `/api` в state-manager, нужен только `STATE_MANAGER_URL`, БД не используется. Список версий: [теги репозитория](https://github.com/reconcile-kit/insight/tags).

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
  dhnikolas/insight:0.3.0
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
          image: dhnikolas/insight:0.3.0
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

В `DATABASE_URL` можно передавать любые [параметры libpq/pgx](https://pkg.go.dev/github.com/jackc/pgx/v5/pgconn#ParseConfig), например `sslrootcert=/certs/ca.pem` или `target_session_attrs=read-only`.

## Доступ и авторизация

Своей аутентификации у insight нет. Пользователь, который открыл UI, может читать все ресурсы и через прокси менять их в state-manager. Поэтому:

- **Не публикуйте insight наружу без аутентификации.** Ставьте его за Ingress с oauth2-proxy или OIDC, за VPN или во внутреннюю сеть.
- **Если в state-manager включён `AUTH_ENABLED=true`.** Insight передаёт заголовок `Authorization` в state-manager без изменений, но сам браузер токен не добавляет. Чтобы запись работала, заголовок `Authorization: Bearer <JWT>` должен подставлять прокси перед insight, например oauth2-proxy с `--pass-authorization-header`. Тогда state-manager сам проверит права пользователя на запись. Без этого чтение работает, а создание, изменение и удаление вернут `401`.
- **Чтение идёт напрямую из БД, и правила `auth_role_rules` state-manager к нему не применяются.** Любой, кто имеет доступ к insight, видит все ресурсы.

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
| `canceling statement due to conflict with recovery` | Запрос на реплике отменён репликацией. См. [Производительность](#производительность-на-больших-объёмах). |
