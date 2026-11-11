.DEFAULT_GOAL := help

.PHONY: build build-go build-dist test test-go test-dart test-unit test-race test-e2e \
	lint lint-go lint-dart lint-e2e format format-go format-dart format-e2e review review-dart coverage coverage-go coverage-dart \
	doc-design doc-decision dup complexity release sec \
	thoth-meta thoth-meta-go thoth-meta-go-test thoth-meta-ts-e2e \
	check-tools install-tools-help help e2e e2e-deps test-dart-e2e test-python-e2e test-rust-e2e test-swift-e2e test-typescript-e2e test-kotlin-e2e test-integrations

GO := go
BUN := bun
ROOT_DIR := $(CURDIR)
TMP_DIR := $(ROOT_DIR)/tmp
GO_CACHE_DIR := $(ROOT_DIR)/.gocache
GO_MOD_CACHE_DIR := $(ROOT_DIR)/.gomodcache
E2E_BIN_DIR := $(ROOT_DIR)/.e2e-bin
CLI_BIN := $(E2E_BIN_DIR)/gh-flarebyte
GO_PACKAGES := ./...
GO_ENV := GOTOOLCHAIN=local GOCACHE="$(GO_CACHE_DIR)" GOMODCACHE="$(GO_MOD_CACHE_DIR)"
BUN_ENV := TMPDIR="$(TMP_DIR)"
BIOME := $(BUN_ENV) "$(ROOT_DIR)/node_modules/.bin/biome"
THOTH := thoth
FLYB := flyb
VERSION ?= dev
COMMIT ?= unknown
BUILD_DATE ?= unknown
GO_VERSION ?= $(shell $(GO) version | awk '{print $$3}')
# Query Go build metadata only for build recipes, so help/tool checks work without Go.
GO_LDFLAGS = -X github.com/flarebyte/gh-flarebyte/internal/buildinfo.Version=$(VERSION) -X github.com/flarebyte/gh-flarebyte/internal/buildinfo.CommitID=$(COMMIT) -X github.com/flarebyte/gh-flarebyte/internal/buildinfo.Date=$(BUILD_DATE) -X github.com/flarebyte/gh-flarebyte/internal/buildinfo.GoVersion=$(GO_VERSION)

# Build and distribution
build: build-dist ## Build the CLI and configured distribution artifacts.

build-go: ## Build the Go CLI into .e2e-bin/.
	mkdir -p "$(TMP_DIR)"
	mkdir -p "$(E2E_BIN_DIR)"
	$(GO_ENV) $(GO) build -ldflags "$(GO_LDFLAGS)" -o "$(CLI_BIN)" ./cmd/gh-flarebyte

build-dist: build-go ## Build artifacts using the root .gh-flarebyte.cue.
	mkdir -p "$(TMP_DIR)"
	"$(CLI_BIN)" build

# Tests and optional integrations
test: test-go test-e2e ## Run Go tests and default E2E tests; real integrations are opt-in.

test-go: test-unit ## Alias for configured unit tests (Go in this repository).

# Compatibility aliases follow build.language in the root config; they do not select Dart.
test-dart: test-unit ## Compatibility alias for configured tests; does not select Dart.

test-unit: build-go ## Run configured tests with per-test output.
	mkdir -p "$(TMP_DIR)"
	"$(CLI_BIN)" test --style per_test

test-race: ## Run Go tests with the race detector.
	mkdir -p "$(TMP_DIR)"
	$(GO_ENV) $(GO) test -race $(GO_PACKAGES)

coverage: coverage-go ## Run configured coverage checks.

coverage-go: build-go ## Run coverage using the root configuration (Go here).
	mkdir -p "$(TMP_DIR)"
	"$(CLI_BIN)" cov

coverage-dart: coverage-go ## Compatibility alias for configured coverage; does not select Dart.

# Default E2E uses stub tools and pure Bun tests; real language integrations remain opt-in.
e2e: test-e2e ## Alias for the default E2E suite.

test-dart-e2e: build-go ## Run the Dart command fixture using a stub Dart executable.
	mkdir -p "$(TMP_DIR)"
	$(BUN_ENV) $(BUN) test ./e2e/dart.test.ts

test-integrations: test-python-e2e test-rust-e2e test-swift-e2e test-typescript-e2e test-kotlin-e2e ## Run all real language fixtures.

e2e-deps: ## Install locked dependencies for Biome and TypeScript tooling.
	mkdir -p "$(TMP_DIR)"
	$(BUN_ENV) $(BUN) install --frozen-lockfile

test-e2e: build-go ## Run smoke and stub-tool E2E tests.
	mkdir -p "$(TMP_DIR)"
	$(BUN_ENV) $(BUN) test ./e2e

test-python-e2e: build-go ## Run the real Python/uv integration fixture.
	mkdir -p "$(TMP_DIR)"
	GH_FLAREBYTE_PYTHON_E2E=1 $(BUN_ENV) $(BUN) test ./e2e/python.test.ts

test-rust-e2e: build-go ## Run the real Rust/Cargo integration fixture.
	mkdir -p "$(TMP_DIR)"
	GH_FLAREBYTE_RUST_E2E=1 $(BUN_ENV) $(BUN) test ./e2e/rust.test.ts

test-swift-e2e: build-go ## Run the real SwiftPM integration fixture.
	mkdir -p "$(TMP_DIR)"
	GH_FLAREBYTE_SWIFT_E2E=1 $(BUN_ENV) $(BUN) test ./e2e/swift.test.ts

test-typescript-e2e: build-go ## Run the real Bun TypeScript library and CLI fixture.
	mkdir -p "$(TMP_DIR)"
	GH_FLAREBYTE_TYPESCRIPT_E2E=1 $(BUN_ENV) $(BUN) test ./e2e/typescript.test.ts

test-kotlin-e2e: build-go ## Run the real Kotlin/JVM Gradle library and CLI fixture (JDK 21).
	mkdir -p "$(TMP_DIR)"
	GH_FLAREBYTE_KOTLIN_E2E=1 $(BUN_ENV) $(BUN) test ./e2e/kotlin.test.ts

# Formatting, lint, and review
lint: lint-go lint-e2e ## Run configured Go lint and Biome checks.

lint-go: build-go ## Run lint using the root configuration (Go here).
	mkdir -p "$(TMP_DIR)"
	"$(CLI_BIN)" lint

lint-dart: lint-go ## Compatibility alias for configured lint; does not select Dart.

lint-e2e: e2e-deps ## Run locked local Biome checks.
	$(BIOME) check .

format: format-go format-e2e ## Format configured Go sources and Biome-managed files.

review: ## Finish formatting before running tests and lint, including with -j.
	$(MAKE) format
	$(MAKE) test lint

format-go: build-go ## Format sources using the root configuration (Go here).
	mkdir -p "$(TMP_DIR)"
	"$(CLI_BIN)" format

format-dart: format-go ## Compatibility alias for configured formatting; does not select Dart.

review-dart: ## Compatibility review for the root config, including coverage.
	$(MAKE) format-dart
	$(MAKE) test-dart lint-dart coverage-dart

format-e2e: e2e-deps ## Format Biome-managed files using locked local tooling.
	$(BIOME) format --write .

# Documentation and repository maintenance
doc-design: ## Validate design inputs and regenerate Markdown with flyb.
	mkdir -p doc/design
	$(FLYB) validate --config doc/design-meta/app.cue
	$(FLYB) generate markdown --config doc/design-meta/app.cue

doc-decision: build-go ## Legacy decision-doc workflow; requires the missing generation script.
	@test -f ./script/generate-decision-docs.sh || { printf "%s\n" "doc-decision unavailable: script/generate-decision-docs.sh is missing." >&2; exit 1; }
	mkdir -p doc/decision-meta
	mkdir -p doc/decision
	SEER_BIN="$(CLI_BIN)" sh ./script/generate-decision-docs.sh

dup: ## Run Go and TypeScript duplication checks through npx jscpd.
	npx jscpd --format go --min-lines 10 --ignore "**/.gomodcache/**,**/.gocache/**,**/.e2e-bin/**,**/node_modules/**,**/dist/**" --gitignore .
	npx jscpd --format typescript --min-lines 10 --gitignore .

complexity: ## Show top Go and TypeScript files by complexity.
	scc --sort complexity --by-file -i go . | head -n 15
	scc --sort complexity --by-file -i ts . | head -n 15

release: build-go ## Build the CLI and run the configured release workflow.
	mkdir -p "$(TMP_DIR)"
	GH_FLAREBYTE_FAKE_RELEASE=$${GH_FLAREBYTE_FAKE_RELEASE:-0} "$(CLI_BIN)" release

sec: ## Run Semgrep security scanning.
	semgrep scan --config auto

thoth-meta: thoth-meta-go thoth-meta-go-test thoth-meta-ts-e2e ## Refresh Go, Go test, and E2E TypeScript metadata.

thoth-meta-go: ## Refresh Go source metadata.
	$(THOTH) run --config ./pipeline-go-maat.thoth.cue

thoth-meta-go-test: ## Refresh Go test metadata.
	$(THOTH) run --config ./pipeline-go-test-maat.thoth.cue

thoth-meta-ts-e2e: ## Refresh E2E TypeScript metadata.
	$(THOTH) run --config ./pipeline-ts-e2e-maat.thoth.cue

# Tool discovery and help
check-tools: ## Report command/local-tool availability as key=value pairs.
	@printf "biome=%s\n" "$$(test -x "$(ROOT_DIR)/node_modules/.bin/biome" && printf true || printf false)"
	@printf "java=%s\n" "$$(command -v java >/dev/null 2>&1 && printf true || printf false)"
	@printf "swift=%s\n" "$$(command -v swift >/dev/null 2>&1 && printf true || printf false)"
	@printf "cargo=%s\n" "$$(command -v cargo >/dev/null 2>&1 && printf true || printf false)"
	@printf "cargo-llvm-cov=%s\n" "$$(command -v cargo-llvm-cov >/dev/null 2>&1 && printf true || printf false)"
	@printf "uv=%s\n" "$$(command -v uv >/dev/null 2>&1 && printf true || printf false)"
	@printf "go=%s\n" "$$(command -v $(GO) >/dev/null 2>&1 && printf true || printf false)"
	@printf "bun=%s\n" "$$(command -v $(BUN) >/dev/null 2>&1 && printf true || printf false)"
	@printf "flyb=%s\n" "$$(command -v $(FLYB) >/dev/null 2>&1 && printf true || printf false)"
	@printf "thoth=%s\n" "$$(command -v $(THOTH) >/dev/null 2>&1 && printf true || printf false)"
	@printf "semgrep=%s\n" "$$(command -v semgrep >/dev/null 2>&1 && printf true || printf false)"
	@printf "scc=%s\n" "$$(command -v scc >/dev/null 2>&1 && printf true || printf false)"
	@printf "npx=%s\n" "$$(command -v npx >/dev/null 2>&1 && printf true || printf false)"

install-tools-help: ## Show core and optional tool installation guidance.
	@printf "Core tools and optional workflow tools:\n"
	@printf "  Kotlin (optional integration): JDK 21; the committed Gradle wrapper downloads pinned tools and locked dependencies.\n"
	@printf "  Swift (optional integration): Swift 6+ via Xcode or https://www.swift.org/install/\n"
	@printf "  TypeScript (optional integration): Bun 1.3.6+; the fixture installs locked TypeScript and Biome tools.\n"
	@printf "  Rust (optional integration): https://rustup.rs; components rustfmt, clippy, llvm-tools-preview; cargo install cargo-llvm-cov --locked\n"
	@printf "  uv (optional Python integration): https://docs.astral.sh/uv/getting-started/installation/\n"
	@printf "  go: https://go.dev/doc/install\n"
	@printf "  bun: https://bun.sh/docs/installation\n"
	@printf "  biome: make e2e-deps installs the version pinned by bun.lock.\n"
	@printf "  flyb: install the flyb CLI used for doc generation.\n"
	@printf "  thoth: install the thoth CLI used for metadata pipelines.\n"
	@printf "  semgrep: https://semgrep.dev/docs/getting-started/cli\n"
	@printf "  scc: https://github.com/boyter/scc\n"
	@printf "  jscpd: available via npx jscpd once Node/npm tooling is installed.\n"

help: ## Show available targets.
	@printf "Targets:\n"
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_.-]+:.*## / {printf "  %-22s %s\n", $$1, $$2}' "$(firstword $(MAKEFILE_LIST))"
