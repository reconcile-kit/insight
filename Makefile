.PHONY: stand stand-auth stand-deps stand-down stand-logs seed web build test test-integration dev

# The stand reuses reconcile-kit/e2e-tests: a sibling checkout (../e2e-tests) if
# present, otherwise a clone in build/e2e-tests. Its *_REF variables
# (STATE_MANAGER_REF=pr/42, ...) work as usual.
E2E_TESTS_DIR ?= $(if $(wildcard ../e2e-tests/docker-compose.yml),../e2e-tests,build/e2e-tests)
E2E_TESTS_REF ?= main
export E2E_TESTS_DIR

STATE_MANAGER_URL ?= http://127.0.0.1:58080
TEST_DATABASE_URL ?= postgres://e2e:e2e@127.0.0.1:55432/e2e?sslmode=disable

stand: stand-deps ## Start postgres + redis + state-manager (from e2e-tests) and insight
	docker compose up -d --build
	@for i in $$(seq 1 60); do curl -sf $(STATE_MANAGER_URL)/health/live >/dev/null && break; sleep 1; done
	@for i in $$(seq 1 30); do curl -sf http://127.0.0.1:57182/readyz >/dev/null && break; sleep 1; done
	@echo "insight:       http://127.0.0.1:57182"
	@echo "state-manager: $(STATE_MANAGER_URL)"

stand-auth: stand-deps ## Stand with Keycloak login (users reader/editor/admin/nobody, password = user name)
	docker compose -f docker-compose.yml -f docker-compose.auth.yml up -d --build
	@for i in $$(seq 1 90); do curl -sf http://127.0.0.1:58180/realms/insight/.well-known/openid-configuration >/dev/null && break; sleep 1; done
	@for i in $$(seq 1 30); do curl -sf http://127.0.0.1:57182/readyz >/dev/null && break; sleep 1; done
	@echo "insight:  http://127.0.0.1:57182 (sign in: reader/reader, editor/editor, admin/admin, nobody/nobody)"
	@echo "keycloak: http://127.0.0.1:58180 (admin/admin)"

stand-deps:
	@if [ ! -f "$(E2E_TESTS_DIR)/docker-compose.yml" ]; then \
		git clone --depth 1 --branch $(E2E_TESTS_REF) https://github.com/reconcile-kit/e2e-tests.git $(E2E_TESTS_DIR); \
	fi
	$(E2E_TESTS_DIR)/scripts/prepare-deps.sh

stand-down: ## Stop the stand and delete its data
	docker compose down -v --remove-orphans

stand-logs:
	docker compose logs -f insight state-manager

seed: ## Fill the stand with demo resources via the state-manager API
	STATE_MANAGER_URL=$(STATE_MANAGER_URL) ./scripts/seed.sh

web:
	npm ci && npm run build

build: web
	go build -o bin/insight ./cmd/insight

dev: ## Vite dev server, backend is insight from the stand
	npm run dev

test:
	go test ./...

test-integration: ## Query tests on the stand postgres (isolated schema insight_test)
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -count=1 ./...
