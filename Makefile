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
	cargo:rust \
	django:python \
	flask:python \
	kafka:java \
	kubernetes:none \
	ocelot:csharp \
	spark-java:java \
	terraform:none \
	vscode:typescript

.PHONY: build test bench bench-embed corpus-rebuild corpus-enrich corpus-backup corpus-restore

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

# Backup all corpus DBs to a tarball.
corpus-backup:
	@echo "Backing up corpus DBs..."
	tar czf corpus-dbs-$(shell date +%Y%m%d).tar.gz \
		$(foreach entry,$(CORPUS_REPOS),$(CORPUS)/$(firstword $(subst :, ,$(entry)))/.knowing/graph.db)
	@echo "Saved to corpus-dbs-$(shell date +%Y%m%d).tar.gz"

# Restore corpus DBs from a tarball.
corpus-restore:
	@if [ -z "$(TARBALL)" ]; then \
		echo "Usage: make corpus-restore TARBALL=corpus-dbs-20260527.tar.gz"; \
		exit 1; \
	fi
	tar xzf $(TARBALL)
	@echo "Restored from $(TARBALL)"

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
