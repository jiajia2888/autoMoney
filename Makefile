GO_DIR := HzTechGoBackService
.PHONY: run build test check migrate seed collector-setup collector-doctor
run:
	cd $(GO_DIR) && go run .
migrate:
	cd $(GO_DIR) && go run . -migrate
seed:
	cd $(GO_DIR) && go run . -seed-samples
build:
	cd $(GO_DIR) && go build -o bin/autoMoney .
test:
	cd $(GO_DIR) && go test ./...
check:
	cd $(GO_DIR) && go test ./... && go vet ./...
	.venv/bin/python -m unittest discover -s scripts -p 'test_*.py'
	node --check HxTechPages/assets/app.js
	node --test scripts/test_frontend.cjs
collector-setup:
	test -x .venv/bin/python || uv venv --python 3.12 .venv
	uv pip install --python .venv/bin/python -r scripts/collector-requirements.txt
collector-doctor:
	.venv/bin/python scripts/douyin_collector.py --mode doctor

.PHONY: service-install service-stop service-status service-uninstall
service-install: build
	python3 scripts/local_service.py install
service-stop:
	python3 scripts/local_service.py stop
service-status:
	python3 scripts/local_service.py status
service-uninstall:
	python3 scripts/local_service.py uninstall

.PHONY: yuanbao-browser-setup
yuanbao-browser-setup:
	.venv/bin/python -m playwright install chromium
