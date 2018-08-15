# Expands to list this project's go packages, excluding the vendor folder
SHELL = bash
PACKAGES = $$(go list ./... | grep -v /vendor/)
BUILD_FLAGS =

all: prep

build:
	go build $(BUILD_FLAGS) $(PACKAGES)

builddir:
	@if [ ! -d build ]; then mkdir build; fi

vet:
	go vet $(PACKAGES)

lint:
	golint -set_exit_status $(PACKAGES)

clean:
	rm -rf build/*

fmt:
	go fmt $(PACKAGES)

test:
	go test $(BUILD_FLAGS) $(PACKAGES)

testreport: builddir
	# runs go test in each package one at a time, generating coverage profiling
    # finally generates a combined junit test report and a test coverage report
    # note: running coverage messes up line numbers in error stacktraces
	go test $(BUILD_FLAGS) -v -covermode=count -coverprofile=build/coverage.out $(PACKAGES) | tee build/test.out
	go tool cover -html=build/coverage.out -o build/coverage.html
	go2xunit -input build/test.out -output build/test.xml
	! grep -e "--- FAIL" -e "^FAIL" build/test.out

### DOCKER

COMPOSE_FILE_DEPS = docker-compose.yml
COMPOSE_FILE_BUILD = docker-compose.build.yml

DOCKER_COMPOSE := docker-compose $(if $(wildcard ${COMPOSE_FILE_DEPS}),-f ${COMPOSE_FILE_DEPS},) -f ${COMPOSE_FILE_BUILD}

up:
# brings up the projects dependencies in a compose stack
	@if [ -f ${COMPOSE_FILE_DEPS} ]; then \
		docker-compose -f ${COMPOSE_FILE_DEPS} up -d; \
		if docker-compose -f ${COMPOSE_FILE_DEPS} config --services | grep wait; then \
			docker-compose -f ${COMPOSE_FILE_DEPS} logs -f wait; \
		fi \
	fi

down:
# brings down the projects dependencies
	${DOCKER_COMPOSE} down -v --remove-orphans

pull:
# pulls latest versions of dependency images
	@if [ -f ${COMPOSE_FILE_DEPS} ]; then \
		docker-compose -f ${COMPOSE_FILE_DEPS} pull; \
	fi

builder:
# creates a development environment as a docker image.
# The source of the project is copied into this image.
# All the make targets should work inside this image.
	${DOCKER_COMPOSE} build --pull builder

ci: pull builder up
# Full, end-to-end build.
	${DOCKER_COMPOSE} run --rm builder make clean build vet lint testreport

bash: builder
	${DOCKER_COMPOSE} run --rm builder bash

fish: builder
	${DOCKER_COMPOSE} run --rm builder fish

prep: fmt pull builder up
# Run this before committing.
# Formats code, and runs "make ci" in docker
	${DOCKER_COMPOSE} run --rm builder make build vet lint test

vendor.update:
	dep ensure --update

vendor.ensure:
	dep ensure

### TOOLS

tools: buildtools
	go get -u github.com/golang/dep/cmd/dep

buildtools:
# installs tools used during build
	go get -u github.com/tebeka/go2xunit
	go get -u golang.org/x/tools/cmd/cover
	go get -u github.com/golang/lint/golint

.PHONY: all build builddir run artifacts vet lint clean fmt test testall testreport up down pull builder runc ci bash fish image prep vendor.update vendor.ensure tools buildtools migratetool db.migrate

