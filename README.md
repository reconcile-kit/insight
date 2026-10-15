# Insight

Insight — веб-интерфейс для ресурсов [state-manager](https://github.com/reconcile-kit/state-manager).

## Архитектура

Insight собирается в один Go-бинарь, в который встроен SPA (React + Vite).

| Запрос | Куда идёт |
|---|---|
| `POST /insight/v1/resources/search`, `/values`, `/paths` | Напрямую в PostgreSQL, **только чтение**. |
| `GET /api/v1/groups/.../resources/{name}` и все `POST/PUT/DELETE` | Проксируются в state-manager без изменений. |
| Остальные `/api/*`, включая list state-manager | `404`. |

Сложные фильтры живут только в insight: API state-manager не меняется и не получает тяжёлых запросов.

Запись в БД исключена на двух уровнях:
- сессии открываются с `default_transaction_read_only=on`;
- каждый запрос выполняется в `READ ONLY` транзакции.

Всё равно рекомендуется указывать реплику и read-only роль.

### Возможности

- Фильтры по group, kind, namespace и shard. Выпадающие списки строятся по `DISTINCT` из БД с учётом остальных фильтров, работает автодополнение при вводе.
- Поиск по name по подстроке.
- Labels-селектор: `app=nginx, tier!=db, env in (a,b), env notin (c), team, !legacy, app~ngi`.
- Конструктор условий по jsonb (`spec`, `status`, `annotations`, `finalizers`):
  - операторы `= ≠ > ≥ < ≤ contains !contains regex in exists !exists`;
  - объединение условий через И / ИЛИ;
  - пути вида `a.b[0].c` и `["app.kubernetes.io/name"]`;
  - подсказки путей по выборке документов.
- Общее количество найденных ресурсов. Серверная сортировка по клику на заголовок колонки. Пагинация.
- Клик по иконке-фильтру в ячейке kind, group, namespace или shard и по чипу label добавляет фильтр. `Enter` в любом поле применяет фильтры.
- Ресурс открывается в боковой панели. Тело при каждом открытии заново загружается из state-manager.
- Редактор JSON (CodeMirror) с подсветкой и проверкой синтаксиса, сохранение по `⌘S`.
- Цвет kind и group вычисляется из их имени. Светлая и тёмная тема. Состояние фильтров хранится в URL, есть автообновление.

## Переменные окружения

| Переменная | По умолчанию | |
|---|---|---|
| `STATE_MANAGER_URL` | — (обязательна) | Адрес state-manager, например `http://state-manager:8080`. |
| `DATABASE_URL` | — | DSN Postgres (реплика). |
| `DATABASE_HOST` / `PORT` / `USERNAME` / `PASSWORD` / `NAME` / `SSLMODE` | `localhost` / `5432` / — / — / — / `disable` | Используются, если `DATABASE_URL` не задан. Это те же переменные, что у state-manager. |
| `DATABASE_MAX_CONNS` | `10` | |
| `DATABASE_STATEMENT_TIMEOUT` | `15s` | Если подсчёт total не уложится в таймаут, страница всё равно вернётся, но без total. |
| `LISTEN_ADDR` | `:27182` | |

## Локальный стенд

Стенд использует [reconcile-kit/e2e-tests](https://github.com/reconcile-kit/e2e-tests) без изменений:
- его `docker-compose.yml` (postgres, redis, state-manager) подключается через `include`;
- зависимости готовит его же `scripts/prepare-deps.sh`.

Insight работает с той же базой стенда, отдельная база не создаётся.

```bash
make stand        # ../e2e-tests, если он есть, иначе клон в build/e2e-tests
make seed         # демо-ресурсы через API state-manager
open http://127.0.0.1:57182
make stand-down
```

Версии зависимостей задаются так же, как в e2e-tests, например `STATE_MANAGER_REF=pr/42 make stand`.

Разработка фронтенда: `make dev`. Vite проксирует `/insight` и `/api` в insight стенда.

## Тесты

```bash
make test               # unit-тесты
make test-integration   # запросы на postgres стенда, в отдельной временной схеме insight_test
```
