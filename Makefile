

.PHONY: test
test: test-go test-web

.PHONY: test-go
test-go:
	go fmt ./... && go vet ./... && go test -v -p=1 -cover ./...

# npm ci writes web/node_modules/.package-lock.json; use it as a stamp so
# deps are reinstalled when the lockfile changes, not only when node_modules
# is missing. `find -newer` keeps the check POSIX sh.
.PHONY: test-web
test-web:
	@if [ ! -f web/node_modules/.package-lock.json ] || \
		[ -n "$$(find web/package-lock.json -newer web/node_modules/.package-lock.json 2>/dev/null)" ]; then \
		npm --prefix web ci; \
	fi
	npm --prefix web run test:run

.PHONY: run
run:
	go run ./cmd/ledgerly/main.go

.PHONY: load-events
load-events:
	@echo "Loading events to server..."
	@TENANT_ID=$$(uuidgen | tr '[:upper:]' '[:lower:]'); \
	JWT_HEADER=$$(printf '{"alg":"RS256","typ":"JWT"}' | base64 | tr -d '=' | tr '/+' '_-'); \
	JWT_PAYLOAD=$$(printf '{"tenant_id":"%s","sub":"test-user","iat":%d,"exp":%d}' "$$TENANT_ID" $$(date +%s) $$((($$(date +%s)+3600))) | base64 | tr -d '=' | tr '/+' '_-'); \
	JWT_SIGNATURE=$$(printf 'dummy-signature' | base64 | tr -d '=' | tr '/+' '_-'); \
	JWT_TOKEN="$$JWT_HEADER.$$JWT_PAYLOAD.$$JWT_SIGNATURE"; \
	echo "Using Tenant ID: $$TENANT_ID"; \
	echo "JWT Token: $$JWT_TOKEN"; \
	echo "Payload: $$(printf '{"tenant_id":"%s","sub":"test-user","iat":%d,"exp":%d}' "$$TENANT_ID" $$(date +%s) $$((($$(date +%s)+3600))))"; \
	for i in 1 2 3 4 5; do \
		echo "Creating event $$i..."; \
		curl -vvvv -X POST http://localhost:8080/v1/events \
			-H "Authorization: Bearer $$JWT_TOKEN" \
			-H "Content-Type: application/json" \
			-d "{\"occurred-at\":\"$$(date -u +%Y-%m-%dT%H:%M:%SZ)\",\"action\":\"project.create\",\"actor\":{\"id\":\"user$$i\",\"type\":\"user\"},\"resource\":{\"type\":\"project\",\"id\":\"proj$$i\"}}"; \
		echo ""; \
	done; \
	echo "$$TENANT_ID" > .tenant_id

.PHONY: export-events
export-events:
	@echo "Exporting events from server..."
	@if [ ! -f .tenant_id ]; then \
		echo "Error: No tenant ID found. Run 'make load-events' first."; \
		exit 1; \
	fi; \
	TENANT_ID=$$(cat .tenant_id); \
	JWT_HEADER=$$(printf '{"alg":"RS256","typ":"JWT"}' | base64 | tr -d '=' | tr '/+' '_-'); \
	JWT_PAYLOAD=$$(printf '{"tenant_id":"%s","sub":"test-user","iat":%d,"exp":%d}' "$$TENANT_ID" $$(date +%s) $$((($$(date +%s)+3600))) | base64 | tr -d '=' | tr '/+' '_-'); \
	JWT_SIGNATURE=$$(printf 'dummy-signature' | base64 | tr -d '=' | tr '/+' '_-'); \
	JWT_TOKEN="$$JWT_HEADER.$$JWT_PAYLOAD.$$JWT_SIGNATURE"; \
	echo "Exporting events for Tenant ID: $$TENANT_ID"; \
	curl -vvvv -X GET http://localhost:8080/v1/export \
		-H "Authorization: Bearer $$JWT_TOKEN" \
		-s | jq . > export.json; \
	echo ""; \
	echo "Events exported to export.json"