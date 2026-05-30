#!/usr/bin/env bash
#
# corpus-setup.sh: Reproducible benchmark corpus setup
#
# This script enables external parties to reproduce the cross-system benchmark
# results exactly. It clones repositories at pinned commits, builds graph
# databases, runs LSP enrichment, and pre-embeds vectors.
#
# Usage:
#   ./corpus-setup.sh clone      # Clone all repos at pinned commits (~5 min)
#   ./corpus-setup.sh index      # Build graph databases, tree-sitter only (~5 min)
#   ./corpus-setup.sh enrich     # LSP enrichment (requires language servers, ~2 hours)
#   ./corpus-setup.sh embed      # Pre-embed vectors (requires ONNX model, ~30 min)
#   ./corpus-setup.sh all        # Clone + index + enrich + embed
#   ./corpus-setup.sh verify     # Verify local corpus matches MANIFEST.yaml
#   ./corpus-setup.sh status     # Show current state of each repo
#
# Prerequisites:
#   - knowing binary on PATH (or set KNOWING_BIN)
#   - git 2.20+
#   - For enrichment: gopls, pyright, rust-analyzer, jdtls, tsserver, csharp-ls, ruby-lsp
#   - For embeddings: ONNX model auto-downloads on first run (~30MB)
#
# The MANIFEST.yaml file in this directory records the exact commit hash for each
# repository. Results are only reproducible when repos match these commits.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPOS_DIR="$SCRIPT_DIR/repos"
MANIFEST="$SCRIPT_DIR/MANIFEST.yaml"
KNOWING_BIN="${KNOWING_BIN:-knowing}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log()  { echo -e "${BLUE}[corpus]${NC} $*"; }
ok()   { echo -e "${GREEN}[  OK  ]${NC} $*"; }
warn() { echo -e "${YELLOW}[ WARN ]${NC} $*"; }
fail() { echo -e "${RED}[ FAIL ]${NC} $*"; }

# Parse MANIFEST.yaml (portable, no yq dependency)
# Returns: name|url|commit for each repo
parse_manifest() {
    awk '
        /^  - name:/ { name = $3 }
        /^    url:/ { url = $2 }
        /^    commit:/ { commit = $2; print name "|" url "|" commit }
    ' "$MANIFEST"
}

# Parse enrichment info from manifest
parse_enrichment() {
    awk '
        /^  - name:/ { name = $3 }
        /^    enrichment:/ { enrich = $2; print name "|" enrich }
    ' "$MANIFEST"
}

cmd_clone() {
    log "Cloning repositories at pinned commits..."
    mkdir -p "$REPOS_DIR"

    local total=0 cloned=0 skipped=0 failed=0

    while IFS='|' read -r name url commit; do
        total=$((total + 1))
        local repo_dir="$REPOS_DIR/$name"

        if [ -d "$repo_dir/.git" ]; then
            # Repo exists: verify commit
            local current
            current=$(git -C "$repo_dir" rev-parse HEAD 2>/dev/null || echo "unknown")
            if [ "$current" = "$commit" ]; then
                ok "$name: already at $commit"
                skipped=$((skipped + 1))
                continue
            else
                warn "$name: at $current, expected $commit. Fetching..."
                git -C "$repo_dir" fetch origin 2>/dev/null
                if git -C "$repo_dir" checkout "$commit" 2>/dev/null; then
                    ok "$name: checked out $commit"
                    cloned=$((cloned + 1))
                else
                    fail "$name: could not checkout $commit"
                    failed=$((failed + 1))
                fi
            fi
        else
            log "Cloning $name from $url..."
            if git clone --depth 100 "$url" "$repo_dir" 2>/dev/null; then
                git -C "$repo_dir" checkout "$commit" 2>/dev/null || {
                    # Shallow clone may not have the commit; deepen
                    git -C "$repo_dir" fetch --unshallow 2>/dev/null || true
                    git -C "$repo_dir" checkout "$commit" 2>/dev/null || {
                        fail "$name: commit $commit not found"
                        failed=$((failed + 1))
                        continue
                    }
                }
                ok "$name: cloned at $commit"
                cloned=$((cloned + 1))
            else
                fail "$name: clone failed"
                failed=$((failed + 1))
            fi
        fi
    done < <(parse_manifest)

    log "Clone complete: $cloned cloned, $skipped already present, $failed failed (of $total)"
    [ "$failed" -eq 0 ] || return 1
}

cmd_index() {
    log "Building graph databases (tree-sitter extraction only)..."

    if ! command -v "$KNOWING_BIN" &>/dev/null; then
        fail "knowing binary not found. Set KNOWING_BIN or add knowing to PATH."
        fail "Build with: GOWORK=off go build -o /usr/local/bin/knowing ./cmd/knowing/"
        return 1
    fi

    local total=0 indexed=0 failed=0

    while IFS='|' read -r name url commit; do
        total=$((total + 1))
        local repo_dir="$REPOS_DIR/$name"
        local db_path="$repo_dir/.knowing/graph.db"

        if [ ! -d "$repo_dir" ]; then
            warn "$name: repo not cloned, skipping"
            continue
        fi

        if [ -f "$db_path" ]; then
            local nodes
            nodes=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM nodes" 2>/dev/null || echo "0")
            if [ "$nodes" -gt 0 ]; then
                ok "$name: already indexed ($nodes nodes)"
                indexed=$((indexed + 1))
                continue
            fi
        fi

        log "Indexing $name..."
        mkdir -p "$repo_dir/.knowing"
        if "$KNOWING_BIN" index -no-enrich -db "$db_path" "$repo_dir" 2>/dev/null; then
            local nodes
            nodes=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM nodes" 2>/dev/null || echo "?")
            ok "$name: indexed ($nodes nodes)"
            indexed=$((indexed + 1))
        else
            fail "$name: indexing failed"
            failed=$((failed + 1))
        fi
    done < <(parse_manifest)

    log "Index complete: $indexed indexed, $failed failed (of $total)"
    [ "$failed" -eq 0 ] || return 1
}

cmd_enrich() {
    log "Running LSP enrichment (this requires language servers installed)..."
    log "Expected language servers: gopls, pyright, rust-analyzer, jdtls, tsserver, csharp-ls, ruby-lsp"

    local total=0 enriched=0 skipped=0 failed=0

    while IFS='|' read -r name enrich_type; do
        total=$((total + 1))
        local repo_dir="$REPOS_DIR/$name"
        local db_path="$repo_dir/.knowing/graph.db"

        if [ "$enrich_type" = "none" ]; then
            ok "$name: no enrichment configured"
            skipped=$((skipped + 1))
            continue
        fi

        if [ ! -f "$db_path" ]; then
            warn "$name: not indexed yet, skipping enrichment"
            skipped=$((skipped + 1))
            continue
        fi

        # Check if already enriched
        local lsp_count
        lsp_count=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM edges WHERE provenance LIKE '%lsp%'" 2>/dev/null || echo "0")
        if [ "$lsp_count" -gt 100 ]; then
            ok "$name: already enriched ($lsp_count LSP edges)"
            enriched=$((enriched + 1))
            continue
        fi

        log "Enriching $name with $enrich_type..."
        if "$KNOWING_BIN" index -db "$db_path" "$repo_dir" 2>/dev/null; then
            lsp_count=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM edges WHERE provenance LIKE '%lsp%'" 2>/dev/null || echo "?")
            ok "$name: enriched ($lsp_count LSP edges)"
            enriched=$((enriched + 1))
        else
            fail "$name: enrichment failed (is $enrich_type installed?)"
            failed=$((failed + 1))
        fi
    done < <(parse_enrichment)

    log "Enrichment complete: $enriched enriched, $skipped skipped, $failed failed (of $total)"
    [ "$failed" -eq 0 ] || return 1
}

cmd_embed() {
    log "Pre-embedding vectors (nomic-embed-text-v1.5, ~30MB ONNX model)..."

    local total=0 embedded=0 skipped=0 failed=0

    while IFS='|' read -r name url commit; do
        total=$((total + 1))
        local repo_dir="$REPOS_DIR/$name"
        local db_path="$repo_dir/.knowing/graph.db"

        if [ ! -f "$db_path" ]; then
            warn "$name: not indexed, skipping"
            skipped=$((skipped + 1))
            continue
        fi

        local embed_count
        embed_count=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM embeddings" 2>/dev/null || echo "0")
        if [ "$embed_count" -gt 100 ]; then
            ok "$name: already embedded ($embed_count vectors)"
            embedded=$((embedded + 1))
            continue
        fi

        log "Embedding $name..."
        if "$KNOWING_BIN" enrich embeddings -db "$db_path" 2>/dev/null; then
            embed_count=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM embeddings" 2>/dev/null || echo "?")
            ok "$name: embedded ($embed_count vectors)"
            embedded=$((embedded + 1))
        else
            fail "$name: embedding failed"
            failed=$((failed + 1))
        fi
    done < <(parse_manifest)

    log "Embedding complete: $embedded embedded, $skipped skipped, $failed failed (of $total)"
}

cmd_verify() {
    log "Verifying local corpus against MANIFEST.yaml..."

    local total=0 ok_count=0 mismatch=0 missing=0

    while IFS='|' read -r name url commit; do
        total=$((total + 1))
        local repo_dir="$REPOS_DIR/$name"

        if [ ! -d "$repo_dir/.git" ]; then
            fail "$name: NOT CLONED"
            missing=$((missing + 1))
            continue
        fi

        local current
        current=$(git -C "$repo_dir" rev-parse HEAD 2>/dev/null || echo "unknown")
        if [ "$current" != "$commit" ]; then
            fail "$name: COMMIT MISMATCH (have ${current:0:12}, want ${commit:0:12})"
            mismatch=$((mismatch + 1))
            continue
        fi

        local db_path="$repo_dir/.knowing/graph.db"
        if [ ! -f "$db_path" ]; then
            warn "$name: commit OK but NO GRAPH DB"
            mismatch=$((mismatch + 1))
            continue
        fi

        local nodes
        nodes=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM nodes" 2>/dev/null || echo "0")
        ok "$name: commit ${commit:0:12}, $nodes nodes"
        ok_count=$((ok_count + 1))
    done < <(parse_manifest)

    echo ""
    log "Verification: $ok_count OK, $mismatch mismatched, $missing missing (of $total)"

    if [ "$mismatch" -gt 0 ] || [ "$missing" -gt 0 ]; then
        echo ""
        warn "Corpus does not match manifest. Results may not be reproducible."
        warn "Run './corpus-setup.sh clone' to fix commit mismatches."
        warn "Run './corpus-setup.sh index' to rebuild missing graph databases."
        return 1
    else
        echo ""
        ok "Corpus matches manifest. Results are reproducible."
    fi
}

cmd_status() {
    log "Corpus status:"
    echo ""
    printf "%-15s %-10s %-8s %-10s %-10s %-12s\n" "REPO" "LANGUAGE" "TASKS" "NODES" "EDGES" "ENRICHMENT"
    printf "%-15s %-10s %-8s %-10s %-10s %-12s\n" "----" "--------" "-----" "-----" "-----" "----------"

    while IFS='|' read -r name url commit; do
        local repo_dir="$REPOS_DIR/$name"
        local db_path="$repo_dir/.knowing/graph.db"

        local lang tasks nodes edges enrich
        lang=$(awk -v n="$name" '/^  - name:/{found=0} /^  - name: '"$name"'/{found=1} found && /language:/{print $2; exit}' "$MANIFEST")
        tasks=$(awk -v n="$name" '/^  - name:/{found=0} /^  - name: '"$name"'/{found=1} found && /tasks:/{print $2; exit}' "$MANIFEST")
        enrich=$(awk -v n="$name" '/^  - name:/{found=0} /^  - name: '"$name"'/{found=1} found && /enrichment:/{print $2; exit}' "$MANIFEST")

        if [ -f "$db_path" ]; then
            nodes=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM nodes" 2>/dev/null || echo "?")
            edges=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM edges" 2>/dev/null || echo "?")
        else
            nodes="-"
            edges="-"
        fi

        printf "%-15s %-10s %-8s %-10s %-10s %-12s\n" "$name" "$lang" "$tasks" "$nodes" "$edges" "$enrich"
    done < <(parse_manifest)

    echo ""
    local total_tasks
    total_tasks=$(find "$SCRIPT_DIR/tasks" -name "*.yaml" 2>/dev/null | wc -l | tr -d ' ')
    log "Total task fixtures on disk: $total_tasks"
}

# Main dispatch
case "${1:-help}" in
    clone)   cmd_clone ;;
    index)   cmd_index ;;
    enrich)  cmd_enrich ;;
    embed)   cmd_embed ;;
    all)     cmd_clone && cmd_index && cmd_enrich && cmd_embed ;;
    verify)  cmd_verify ;;
    status)  cmd_status ;;
    help|*)
        echo "Usage: $0 {clone|index|enrich|embed|all|verify|status}"
        echo ""
        echo "Reproducible benchmark corpus setup for the knowing cross-system benchmark."
        echo "See MANIFEST.yaml for pinned repository versions and expected graph statistics."
        echo ""
        echo "Commands:"
        echo "  clone    Clone all repos at pinned commits"
        echo "  index    Build graph databases (tree-sitter only, ~5 min)"
        echo "  enrich   LSP enrichment (requires language servers, ~2 hours)"
        echo "  embed    Pre-embed vectors (ONNX model, ~30 min)"
        echo "  all      Run clone + index + enrich + embed"
        echo "  verify   Verify local corpus matches MANIFEST.yaml"
        echo "  status   Show current state of each repo"
        ;;
esac
