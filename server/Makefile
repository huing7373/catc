.PHONY: build test lint docker-up docker-down ci

REPO_ROOT := $(shell cd .. && pwd)
SCRIPTS := $(REPO_ROOT)/scripts

build:
	bash $(SCRIPTS)/build.sh

test:
	bash $(SCRIPTS)/build.sh --test

lint:
	cd . && golangci-lint run ./...

docker-up:
	docker compose -f deploy/docker-compose.yml up -d

docker-down:
	docker compose -f deploy/docker-compose.yml down

ci: lint build test
	bash $(SCRIPTS)/build.sh --race --test
