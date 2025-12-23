MODULE   = $(shell env GO111MODULE=on $(GO) list -m)
DATE    ?= $(shell date +%FT%T%z)
VERSION ?= $(shell git describe --tags --always --dirty --match=v* 2> /dev/null || \
			cat $(CURDIR)/.version 2> /dev/null || echo v0)
PKGS     = $(or $(PKG),$(shell env GO111MODULE=on $(GO) list ./...))
TESTPKGS = $(shell env GO111MODULE=on $(GO) list -f \
			'{{ if or .TestGoFiles .XTestGoFiles }}{{ .ImportPath }}{{ end }}' \
			$(PKGS))
BIN      = $(CURDIR)/.bin

GOLANGCI_VERSION = v1.47.2

GO           = go
TIMEOUT_UNIT = 5m
TIMEOUT_E2E  = 20m
V = 0
Q = $(if $(filter 1,$V),,@)
M = $(shell printf "\033[34;1m🐱\033[0m")

export GO111MODULE=on

COMMANDS=$(patsubst cmd/%,%,$(wildcard cmd/*))
BINARIES=$(addprefix bin/,$(COMMANDS))

.PHONY: all
all: fmt $(BINARIES) | $(BIN) ; $(info $(M) building executable…) @ ## Build program binary

.PHONY: fmt
fmt: ; $(info $(M) formatting Go code…) @ ## Format Go code
	$Q $(GO) fmt ./...

.PHONY: test
test: test-unit ; $(info $(M) running all tests…) @ ## Run all tests

.PHONY: test-unit
test-unit: ; $(info $(M) running unit tests…) @ ## Run unit tests
	$Q $(GO) test -timeout $(TIMEOUT_UNIT) -race -cover $(TESTPKGS)

.PHONY: test-unit-verbose
test-unit-verbose: ; $(info $(M) running unit tests (verbose)…) @ ## Run unit tests (verbose)
	$Q $(GO) test -timeout $(TIMEOUT_UNIT) -race -cover -v $(TESTPKGS)

.PHONY: yamllint
yamllint: ; $(info $(M) running yamllint…) @ ## Run yamllint on config and workflow files
	$Q yamllint -c .yamllint config/ .github/workflows/ || echo "yamllint not installed or errors found"

$(BIN):
	@mkdir -p $@
$(BIN)/%: | $(BIN) ; $(info $(M) building $(PACKAGE)…)
	$Q tmp=$$(mktemp -d); cd $$tmp; \
		env GO111MODULE=on GOPATH=$$tmp GOBIN=$(BIN) $(GO) install $(PACKAGE) \
		|| ret=$$?; \
		env GO111MODULE=on GOPATH=$$tmp GOBIN=$(BIN) $(GO) clean -modcache \
        || ret=$$?; \
		cd - ; \
	  	rm -rf $$tmp ; exit $$ret

FORCE:

bin/%: cmd/% FORCE
	$Q $(GO) build -mod=vendor $(LDFLAGS) -v -o $@ ./$<

KO = $(or ${KO_BIN},${KO_BIN},$(BIN)/ko)
$(BIN)/ko: PACKAGE=github.com/google/ko@latest

.PHONY: apply
apply: | $(KO) ; $(info $(M) ko apply -R -f config/) @ ## Apply config to the current cluster
	$Q $(KO) apply -R -f config

.PHONY: resolve
resolve: | $(KO) ; $(info $(M) ko resolve -R -f config/) @ ## Resolve config to the current cluster
	$Q $(KO) resolve --push=false --oci-layout-path=$(BIN)/oci -R -f config

.PHONY: generated
generated: | vendor ; $(info $(M) update generated files) ## Update generated files
	$Q ./hack/update-codegen.sh

.PHONY: vendor
vendor:
	$Q ./hack/update-deps.sh

# Misc

.PHONY: clean
clean: ; $(info $(M) cleaning…)	@ ## Cleanup everything
	@rm -rf $(BIN)
	@rm -rf bin
	@rm -rf test/tests.* test/coverage.*

.PHONY: help
help:
	@grep -hE '^[ a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-17s\033[0m %s\n", $$1, $$2}'

.PHONY: version
version:

	@echo $(VERSION)

.PHONY: deploy_tekton
deploy_tekton: clean_tekton | ; $(info $(M) deploying tekton on local cluster …) @ ## Deploying tekton on local clustert
	-kubectl apply --filename https://infra.tekton.dev/tekton-releases/pipeline/latest/release.yaml
	-ko apply -f config;

.PHONY:  clean_tekton 
clean_tekton: | ; $(info $(M) deleteing tekton from local cluster …) @ ## Deleteing tekton on local clustert
	-ko delete -f config;

# Prerequisite: docker [or] podman and kind
# this will deploy a local registry using docker and create a kind cluster
# configuring with the registry
# then does make apply to deploy the operator
# and show the location of kubeconfig at last
.PHONY: dev-setup
dev-setup: # setup kind with local registry for local development
	@cd ./hack/dev/kind/;./install.sh

#Release
RELEASE_VERSION=v0.0.0
RELEASE_DIR ?= /tmp/tektoncd-pruner-${RELEASE_VERSION}

.PHONY: github-release
github-release:
	./hack/release.sh ${RELEASE_VERSION}

