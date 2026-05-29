# knowing Makefile
#
# Primary targets:
#   make build           Build the knowing binary
#   make test            Run unit tests
#   make bench           Run benchmark suite (no embeddings)
#   make bench-embed     Run benchmark suite (with embeddings, ~80 min)
#   make corpus-rebuild  Rebuild all benchmark corpus DBs from scratch
#   make corpus-backup   Tarball all corpus DBs for safekeeping
#   make corpus-restore  Restore corpus DBs from tarball

GOWORK := off
BINARY := knowing
BUILD_DIR := /tmp
CORPUS := bench/cross-system/corpus/repos
DOTNET_ROOT := /opt/homebrew/opt/dotnet/libexec

# Repos and their enrichment config.
# Format: repo:lang (lang determines which LSP to use; "none" = skip enrichment)
CORPUS_REPOS := \
	caddy:go \
	cargo:rust \
	django:python \
	fastapi:python \
	flask:python \
	kafka:java \
	kubernetes:go \
	ocelot:csharp \
	spark-java:java \
	terraform:go \
	jekyll:ruby \
	vscode:typescript

.PHONY: build test bench bench-embed corpus-rebuild corpus-enrich corpus-backup corpus-restore corpus-upload corpus-download

build:
	GOWORK=$(GOWORK) go build -o $(BUILD_DIR)/$(BINARY) ./cmd/knowing/

test:
	GOWORK=$(GOWORK) go test ./internal/... ./cmd/...

bench:
	BENCH_ADAPTIVE_SEEDS=1 BENCH_ADAPTERS=knowing GOWORK=$(GOWORK) \
		go test ./bench/cross-system/ -run "TestCrossSystem$$" -v -timeout 0

bench-embed:
	BENCH_EMBEDDINGS=1 BENCH_ADAPTIVE_SEEDS=1 BENCH_ADAPTERS=knowing GOWORK=$(GOWORK) \
		go test ./bench/cross-system/ -run "TestCrossSystem$$" -v -timeout 0

# Rebuild all corpus DBs from scratch (index only, no enrichment).
# Run corpus-enrich after to add LSP enrichment.
corpus-index: build
	@for entry in $(CORPUS_REPOS); do \
		repo=$${entry%%:*}; \
		echo "=== Indexing $$repo ==="; \
		rm -f $(CORPUS)/$$repo/.knowing/graph.db*; \
		mkdir -p $(CORPUS)/$$repo/.knowing; \
		$(BUILD_DIR)/$(BINARY) index -no-enrich \
			-url "$(CURDIR)/$(CORPUS)/$$repo" \
			-db "$(CURDIR)/$(CORPUS)/$$repo/.knowing/graph.db" \
			"$(CURDIR)/$(CORPUS)/$$repo" 2>&1 | tail -5; \
	done
	@echo "=== All repos indexed ==="

# Run LSP enrichment on repos that support it.
# Requires: pyright (Python), typescript-language-server (TS),
#           jdtls (Java), rust-analyzer (Rust), csharp-ls (C#)
corpus-enrich: build
	@for entry in $(CORPUS_REPOS); do \
		repo=$${entry%%:*}; \
		lang=$${entry##*:}; \
		if [ "$$lang" = "none" ]; then \
			echo "=== Skipping $$repo (no LSP) ==="; \
			continue; \
		fi; \
		echo "=== Enriching $$repo ($$lang) ==="; \
		DOTNET_ROOT=$(DOTNET_ROOT) PATH="$$PATH:$$HOME/.dotnet/tools" \
			$(BUILD_DIR)/$(BINARY) enrich lsp \
			-url "$(CURDIR)/$(CORPUS)/$$repo" \
			-db "$(CURDIR)/$(CORPUS)/$$repo/.knowing/graph.db" \
			"$(CURDIR)/$(CORPUS)/$$repo" 2>&1 | tail -5; \
	done
	@echo "=== All enrichment complete ==="

# Full rebuild: index + enrich.
corpus-rebuild: corpus-index corpus-enrich
	@echo "=== Corpus fully rebuilt ==="

# Backup all corpus DBs to tarballs. Split into two parts to stay under
# GitHub's 2GB release asset limit. Checkpoints WAL files before archiving.
# Part 1: small repos (<200MB each). Part 2: large repos (>=200MB each).
CORPUS_SMALL := caddy cargo fastapi flask ocelot spark-java
CORPUS_LARGE := django kafka kubernetes terraform vscode

corpus-backup:
	@echo "Checkpointing WAL files..."
	@for entry in $(CORPUS_REPOS); do \
		repo=$${entry%%:*}; \
		db="$(CORPUS)/$$repo/.knowing/graph.db"; \
		if [ -f "$$db" ]; then sqlite3 "$$db" "PRAGMA wal_checkpoint(TRUNCATE);" >/dev/null 2>&1; fi; \
	done
	@VERSION=$$(git describe --tags --abbrev=0 2>/dev/null || echo "dev"); \
	DATE=$(shell date +%Y%m%d); \
	echo "Creating part 1 (small repos)..."; \
	tar czf "corpus-dbs-$${VERSION}-$${DATE}-part1.tar.gz" \
		$(foreach repo,$(CORPUS_SMALL),$(CORPUS)/$(repo)/.knowing/graph.db); \
	echo "  Part 1: $$(du -h corpus-dbs-$${VERSION}-$${DATE}-part1.tar.gz | cut -f1)"; \
	echo "Creating part 2 (large repos)..."; \
	tar czf "corpus-dbs-$${VERSION}-$${DATE}-part2.tar.gz" \
		$(foreach repo,$(CORPUS_LARGE),$(CORPUS)/$(repo)/.knowing/graph.db); \
	echo "  Part 2: $$(du -h corpus-dbs-$${VERSION}-$${DATE}-part2.tar.gz | cut -f1)"; \
	echo "Done. Upload with: make corpus-upload"

# Restore corpus DBs from a tarball.
corpus-restore:
	@if [ -z "$(TARBALL)" ]; then \
		echo "Usage: make corpus-restore TARBALL=corpus-dbs-v0.11.0-20260528.tar.gz"; \
		exit 1; \
	fi
	tar xzf $(TARBALL)
	@echo "Restored from $(TARBALL)"

# Upload corpus tarballs to the latest GitHub release.
# Usage: make corpus-upload TAG=v0.11.0
#   or:  make corpus-upload (uses latest release)
corpus-upload:
	@TAG="$(TAG)"; \
	if [ -z "$$TAG" ]; then \
		TAG=$$(gh release list --limit 1 --json tagName -q '.[0].tagName'); \
	fi; \
	PARTS=$$(ls -t corpus-dbs-*-part*.tar.gz 2>/dev/null); \
	if [ -z "$$PARTS" ]; then \
		echo "No tarballs found. Run 'make corpus-backup' first."; \
		exit 1; \
	fi; \
	for f in $$PARTS; do \
		echo "Uploading $$f to release $$TAG..."; \
		gh release upload "$$TAG" "$$f" --clobber; \
	done; \
	echo "Done. Download: make corpus-download TAG=$$TAG"

# Download corpus tarballs from a GitHub release and restore.
# Usage: make corpus-download TAG=v0.11.0
#   or:  make corpus-download (uses latest release)
corpus-download:
	@TAG="$(TAG)"; \
	if [ -z "$$TAG" ]; then \
		TAG=$$(gh release list --limit 1 --json tagName -q '.[0].tagName'); \
	fi; \
	echo "Downloading corpus DBs from release $$TAG..."; \
	gh release download "$$TAG" -p 'corpus-dbs-*' || { echo "No corpus tarball in release $$TAG"; exit 1; }; \
	for f in corpus-dbs-*-part*.tar.gz; do \
		echo "Extracting $$f..."; \
		tar xzf "$$f"; \
	done; \
	echo "Restored all corpus DBs."

# Clear stale embedding caches (needed after reindexing since node hashes change).
corpus-clear-embeddings:
	@for entry in $(CORPUS_REPOS); do \
		repo=$${entry%%:*}; \
		db="$(CORPUS)/$$repo/.knowing/graph.db"; \
		if [ -f "$$db" ]; then \
			sqlite3 "$$db" "DELETE FROM embeddings" 2>/dev/null; \
			echo "Cleared embeddings: $$repo"; \
		fi; \
	done
