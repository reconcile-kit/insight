.PHONY: stand stand-deps stand-down stand-logs seed web build test test-integration dev

# The stand reuses reconcile-kit/e2e-tests: a sibling checkout (../e2e-tests) if
# present, otherwise a clone in build/e2e-tests. Its *_REF variables
# (STATE_MANAGER_REF=pr/42, ...) work as usual.
E2E_TESTS_DIR ?= $(if $(wildcard ../e2e-tests/docker-compose.yml),../e2e-tests,build/e2e-tests)
E2E_TESTS_REF ?= main
export E2E_TESTS_DIR

STATE_MANAGER_URL ?= http://127.0.0.1:58080
TEST_DATABASE_URL ?= postgres://e2e:e2e@127.0.0.1:55432/e2e?sslmode=disable

stand: stand-deps ## Поднять postgres + redis + state-manager (из e2e-tests) и insight
	docker compose up -d --build
	@for i in $$(seq 1 60); do curl -sf $(STATE_MANAGER_URL)/health/live >/dev/null && break; sleep 1; done
	@for i in $$(seq 1 30); do curl -sf http://127.0.0.1:57182/readyz >/dev/null && break; sleep 1; done
	@echo "insight:       http://127.0.0.1:57182"
	@echo "state-manager: $(STATE_MANAGER_URL)"

stand-deps:
	@if [ ! -f "$(E2E_TESTS_DIR)/docker-compose.yml" ]; then \
		git clone --depth 1 --branch $(E2E_TESTS_REF) https://github.com/reconcile-kit/e2e-tests.git $(E2E_TESTS_DIR); \
	fi
	$(E2E_TESTS_DIR)/scripts/prepare-deps.sh

stand-down: ## Остановить стенд и удалить данные
	docker compose down -v --remove-orphans

stand-logs:
	docker compose logs -f insight state-manager

seed: ## Наполнить стенд демо-ресурсами через API state-manager
	STATE_MANAGER_URL=$(STATE_MANAGER_URL) ./scripts/seed.sh

web:
	npm ci && npm run build

build: web
	go build -o bin/insight ./cmd/insight

dev: ## Vite dev server, бэкенд — insight со стенда
	npm run dev

test:
	go test ./...

test-integration: ## Тесты запросов на postgres стенда (изолированная схема insight_test)
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -count=1 ./...
