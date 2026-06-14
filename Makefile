IMAGE_REPO ?= tailscale/tailscale
TAGS ?= "latest"

PLATFORM ?= "flyio" ## flyio==linux/amd64. Set to "" to build all platforms.

vet: ## Run go vet
	./tool/go vet ./...

tidy: ## Run go mod tidy and update nix flake hashes
	./tool/go mod tidy
	./tool/go run ./tool/updateflakes

lint: ## Run golangci-lint
	./tool/go run github.com/golangci/golangci-lint/cmd/golangci-lint run

updatedeps: ## Update depaware deps
	# depaware (via x/tools/go/packages) shells back to "go", so make sure the "go"
	# it finds in its $$PATH is the right one.
	PATH="$$(./tool/go env GOROOT)/bin:$$PATH" ./tool/go run github.com/tailscale/depaware --update --vendor --internal \
		scaletail.com/cmd/scaletaild \
		scaletail.com/cmd/scaletail \
		scaletail.com/cmd/derper \
		scaletail.com/cmd/stund \
		scaletail.com/cmd/tsidp
	PATH="$$(./tool/go env GOROOT)/bin:$$PATH" ./tool/go run github.com/tailscale/depaware --update --goos=linux,darwin,windows,android,ios --vendor --internal \
		scaletail.com/tsnet
	PATH="$$(./tool/go env GOROOT)/bin:$$PATH" ./tool/go run github.com/tailscale/depaware --update --file=depaware-minbox.txt --goos=linux --tags="$$(./tool/go run ./cmd/featuretags --min --add=cli)" --vendor --internal \
		scaletail.com/cmd/scaletaild
	PATH="$$(./tool/go env GOROOT)/bin:$$PATH" ./tool/go run github.com/tailscale/depaware --update --file=depaware-min.txt --goos=linux --tags="$$(./tool/go run ./cmd/featuretags --min)" --vendor --internal \
		scaletail.com/cmd/scaletaild

depaware: ## Run depaware checks
	# depaware (via x/tools/go/packages) shells back to "go", so make sure the "go"
	# it finds in its $$PATH is the right one.
	PATH="$$(./tool/go env GOROOT)/bin:$$PATH" ./tool/go run github.com/tailscale/depaware --check --vendor --internal \
		scaletail.com/cmd/scaletaild \
		scaletail.com/cmd/scaletail \
		scaletail.com/cmd/derper \
		scaletail.com/cmd/stund \
		scaletail.com/cmd/tsidp
	PATH="$$(./tool/go env GOROOT)/bin:$$PATH" ./tool/go run github.com/tailscale/depaware --check --goos=linux,darwin,windows,android,ios --vendor --internal \
		scaletail.com/tsnet
	PATH="$$(./tool/go env GOROOT)/bin:$$PATH" ./tool/go run github.com/tailscale/depaware --check --file=depaware-minbox.txt --goos=linux --tags="$$(./tool/go run ./cmd/featuretags --min --add=cli)" --vendor --internal \
		scaletail.com/cmd/scaletaild
	PATH="$$(./tool/go env GOROOT)/bin:$$PATH" ./tool/go run github.com/tailscale/depaware --check --file=depaware-min.txt --goos=linux --tags="$$(./tool/go run ./cmd/featuretags --min)" --vendor --internal \
		scaletail.com/cmd/scaletaild

buildwindows: ## Build tailscale CLI for windows/amd64
	GOOS=windows GOARCH=amd64 ./tool/go install scaletail.com/cmd/scaletail scaletail.com/cmd/scaletaild

build386: ## Build tailscale CLI for linux/386
	GOOS=linux GOARCH=386 ./tool/go install scaletail.com/cmd/scaletail scaletail.com/cmd/scaletaild

buildlinuxarm: ## Build tailscale CLI for linux/arm
	GOOS=linux GOARCH=arm ./tool/go install scaletail.com/cmd/scaletail scaletail.com/cmd/scaletaild

buildwasm: ## Build tailscale CLI for js/wasm
	GOOS=js GOARCH=wasm ./tool/go install ./cmd/tsconnect/wasm ./cmd/scaletail/cli

buildplan9:
	GOOS=plan9 GOARCH=amd64 ./tool/go install ./cmd/scaletail ./cmd/scaletaild

buildlinuxloong64: ## Build tailscale CLI for linux/loong64
	GOOS=linux GOARCH=loong64 ./tool/go install scaletail.com/cmd/scaletail scaletail.com/cmd/scaletaild

buildmultiarchimage: ## Build (and optionally push) multiarch docker image
	./build_docker.sh

check: staticcheck vet depaware buildwindows build386 buildlinuxarm buildwasm ## Perform basic checks and compilation tests

staticcheck: ## Run staticcheck.io checks
	./tool/go run honnef.co/go/tools/cmd/staticcheck -- $$(./tool/go run ./tool/listpkgs --ignore-3p  ./...)

.PHONY: check-image-repo
check-image-repo:
	@if [ -z "$(REPO)" ]; then \
		echo "REPO=... required; e.g. REPO=ghcr.io/$$USER/tailscale" >&2; \
		exit 1; \
	fi
	@for repo in tailscale/tailscale ghcr.io/tailscale/tailscale \
		tailscale/tsidp ghcr.io/tailscale/tsidp; do \
		if [ "$(REPO)" = "$$repo" ]; then \
			echo "REPO=... must not be $$repo" >&2; \
			exit 1; \
		fi; \
	done

publishdevimage: check-image-repo ## Build and publish tailscale image to location specified by ${REPO}
	TAGS="${TAGS}" REPOS=${REPO} PLATFORM=${PLATFORM} PUSH=true TARGET=client ./build_docker.sh

publishdevtsidp: check-image-repo ## Build and publish tsidp image to location specified by ${REPO}
	TAGS="${TAGS}" REPOS=${REPO} PLATFORM=${PLATFORM} PUSH=true TARGET=tsidp ./build_docker.sh

.PHONY: sshintegrationtest
sshintegrationtest: ## Run the SSH integration tests in various Docker containers
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 ./tool/go test -tags integrationtest -c ./ssh/tailssh -o ssh/tailssh/testcontainers/tailssh.test && \
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 ./tool/go build -o ssh/tailssh/testcontainers/scaletaild ./cmd/scaletaild && \
	echo "Testing on ubuntu:focal, ubuntu:jammy, ubuntu:noble, alpine:latest (in parallel)" && \
	docker build --build-arg="BASE=ubuntu:focal" -t ssh-ubuntu-focal ssh/tailssh/testcontainers & \
	docker build --build-arg="BASE=ubuntu:jammy" -t ssh-ubuntu-jammy ssh/tailssh/testcontainers & \
	docker build --build-arg="BASE=ubuntu:noble" -t ssh-ubuntu-noble ssh/tailssh/testcontainers & \
	docker build --build-arg="BASE=alpine:latest" -t ssh-alpine-latest ssh/tailssh/testcontainers & \
	wait

.PHONY: generate
generate: ## Generate code
	./tool/go generate ./...

.PHONY: pin-github-actions
pin-github-actions:
	./tool/go tool github.com/stacklok/frizbee actions .github/workflows

help: ## Show this help
	@echo ""
	@echo "Specify a command. The choices are:"
	@echo ""
	@grep -hE '^[0-9a-zA-Z_-]+:.*?## .*$$' ${MAKEFILE_LIST} | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[0;36m%-20s\033[m %s\n", $$1, $$2}'
	@echo ""
.PHONY: help

.DEFAULT_GOAL := help
