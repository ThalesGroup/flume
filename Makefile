# Expands to list this project's go packages, excluding the vendor folder
SHELL = bash
PACKAGES = $$(go list ./... | grep -v /vendor/)

# Setting this to 1 enables the new Docker BuildKit code when building images,
# if available.  If buildkit is available in the local version of Docker, it
# will speed up image builds a little.  If it's not there, or if you need to disable
# it for some reason, everying will still work.  It's not required.
DOCKER_BUILDKIT ?= 0

BUILD_FLAGS =

all: fmt build vet lint test

build:
	go build $(BUILD_FLAGS) $(PACKAGES)

builddir:
	@if [ ! -d build ]; then mkdir build; fi

vet:
	go vet $(PACKAGES)

lint:
	golangci-lint run

clean:
	rm -rf build/*

fmt:
	go fmt $(PACKAGES)

test:
	go test -race $(BUILD_FLAGS) $(PACKAGES)

cover: builddir
	# runs go test in each package one at a time, generating coverage profiling
    # finally generates a combined junit test report and a test coverage report
    # note: running coverage messes up line numbers in error stacktraces
	go test $(BUILD_FLAGS) -v -covermode=count -coverprofile=build/coverage.out $(PACKAGES) | tee build/test.out
	go tool cover -html=build/coverage.out -o build/coverage.html
	go2xunit -input build/test.out -output build/test.xml
	! grep -e "--- FAIL" -e "^FAIL" build/test.out

builder:
	DOCKER_BUILDKIT=${DOCKER_BUILDKIT} docker build --pull -t flume_builder .

docker: builder
	docker-compose run --rm builder make all

fish: builder
	docker-compose run --rm builder fish

update:
	go get -u
	go mod tidy

### TOOLS

tools:
# installs tools used during build
	go get -u github.com/tebeka/go2xunit
	go get -u golang.org/x/tools/cmd/cover
	wget -O - -q https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b $(shell go env GOPATH)/bin $(GOLANGCI_LINT_VERSION)

.PHONY: all build builddir run artifacts vet lint clean fmt test testall testreport up down pull builder runc ci bash fish image prep vendor.update vendor.ensure tools buildtools migratetool db.migrate

