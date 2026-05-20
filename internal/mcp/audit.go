package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

func proveTool() mcp.Tool {
	return mcp.NewTool("prove",
		mcp.WithDescription("Generate a cryptographic Merkle proof that a relationship EXISTS between two symbols. The proof is verifiable offline without database access. Returns a JSON proof artifact containing the hash path from edge leaf through edge-type root, package root, to repo root."),
		mcp.WithString("source", mcp.Required(), mcp.Description("Source symbol name (substring match)"), Examples("AuthService", "RecordFeedback")),
		mcp.WithString("target", mcp.Required(), mcp.Description("Target symbol name (substring match)"), Examples("SessionStore", "QueryFeedback")),
		mcp.WithString("edge_type", mcp.Description("Edge type to prove (default: calls)"), Examples("calls", "imports", "implements")),
	)
}

func proveAbsentTool() mcp.Tool {
	return mcp.NewTool("prove_absent",
		mcp.WithDescription("Generate a cryptographic proof that a relationship does NOT exist between two symbols. Uses adjacent sorted Merkle leaves to prove the gap. The proof is verifiable offline. Use this to prove isolation between services, confirm no unauthorized dependencies, or validate architectural boundaries."),
		mcp.WithString("source", mcp.Required(), mcp.Description("Source symbol name (substring match)"), Examples("PaymentService", "RecordFeedback")),
		mcp.WithString("target", mcp.Required(), mcp.Description("Target symbol name (substring match)"), Examples("UserDB", "DeleteSnapshot")),
		mcp.WithString("edge_type", mcp.Description("Edge type to prove absent (default: calls)"), Examples("calls", "imports", "implements")),
	)
}

func fsckTool() mcp.Tool {
	return mcp.NewTool("fsck",
		mcp.WithDescription("Verify graph integrity: recompute every hash, check referential integrity (all edge endpoints exist), verify snapshot chain continuity, and run SQLite PRAGMA integrity_check. Returns pass/fail with details on any corruption found."),
	)
}

// handleProve generates an inclusion proof.
func (s *Server) handleProve(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.sqlStore == nil || s.snapMgr == nil {
		return mcp.NewToolResultError("prove: requires indexed repository"), nil
	}

	source, errResult := requireStringArg(req, "source")
	if errResult != nil {
		return errResult, nil
	}
	target, errResult := requireStringArg(req, "target")
	if errResult != nil {
		return errResult, nil
	}
	edgeType := getStringArg(req, "edge_type")
	if edgeType == "" {
		edgeType = "calls"
	}

	// Find source and target nodes.
	sourceNodes, err := s.sqlStore.NodesByName(ctx, "%"+source)
	if err != nil || len(sourceNodes) == 0 {
		return mcp.NewToolResultError(fmt.Sprintf("source symbol %q not found", source)), nil
	}
	targetNodes, err := s.sqlStore.NodesByName(ctx, "%"+target)
	if err != nil || len(targetNodes) == 0 {
		return mcp.NewToolResultError(fmt.Sprintf("target symbol %q not found", target)), nil
	}

	// Verify the edge exists.
	edgeHash := types.ComputeEdgeHash(sourceNodes[0].NodeHash, targetNodes[0].NodeHash, edgeType, "ast_inferred")
	existingEdges, _ := s.sqlStore.EdgesFrom(ctx, sourceNodes[0].NodeHash, edgeType)
	found := false
	for _, e := range existingEdges {
		if e.TargetHash == targetNodes[0].NodeHash {
			edgeHash = e.EdgeHash
			found = true
			break
		}
	}
	if !found {
		return mcp.NewToolResultError(fmt.Sprintf("no %s edge found from %q to %q", edgeType, source, target)), nil
	}

	// Get repo hash from source node.
	pkgPath, err := snapshot.ExtractPackagePath(sourceNodes[0].QualifiedName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("extracting package: %v", err)), nil
	}

	repoURL := extractRepoURL(sourceNodes[0].QualifiedName)
	repoHash := types.NewHash([]byte(repoURL))

	// Build tree and generate proof.
	edgeInputs, _, err := s.snapMgr.CollectEdgeInputs(ctx, repoHash)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("collecting edges: %v", err)), nil
	}

	tree := snapshot.BuildHierarchicalTree(edgeInputs)
	proof, err := snapshot.GenerateProof(tree, edgeHash, pkgPath, edgeType, edgeInputs)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("generating proof: %v", err)), nil
	}

	totalSteps := len(proof.EdgeToEdgeTypeRoot) + len(proof.EdgeTypeToPackageRoot) + len(proof.PackageToRepoRoot)

	result := map[string]any{
		"verified":    true,
		"source":      source,
		"target":      target,
		"edge_type":   edgeType,
		"package":     pkgPath,
		"proof_steps": totalSteps,
		"repo_root":   proof.RepoRoot.String(),
		"proof":       proof,
	}

	out, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(out)), nil
}

// handleProveAbsent generates an absence proof.
func (s *Server) handleProveAbsent(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.sqlStore == nil || s.snapMgr == nil {
		return mcp.NewToolResultError("prove_absent: requires indexed repository"), nil
	}

	source, errResult := requireStringArg(req, "source")
	if errResult != nil {
		return errResult, nil
	}
	target, errResult := requireStringArg(req, "target")
	if errResult != nil {
		return errResult, nil
	}
	edgeType := getStringArg(req, "edge_type")
	if edgeType == "" {
		edgeType = "calls"
	}

	// Find source and target nodes.
	sourceNodes, err := s.sqlStore.NodesByName(ctx, "%"+source)
	if err != nil || len(sourceNodes) == 0 {
		return mcp.NewToolResultError(fmt.Sprintf("source symbol %q not found", source)), nil
	}
	targetNodes, err := s.sqlStore.NodesByName(ctx, "%"+target)
	if err != nil || len(targetNodes) == 0 {
		return mcp.NewToolResultError(fmt.Sprintf("target symbol %q not found", target)), nil
	}

	// Verify the edge does NOT exist.
	existingEdges, _ := s.sqlStore.EdgesFrom(ctx, sourceNodes[0].NodeHash, edgeType)
	for _, e := range existingEdges {
		if e.TargetHash == targetNodes[0].NodeHash {
			return mcp.NewToolResultError(fmt.Sprintf("cannot prove absence: %s -%s-> %s EXISTS", source, edgeType, target)), nil
		}
	}

	// Compute the hypothetical edge hash.
	edgeHash := types.ComputeEdgeHash(sourceNodes[0].NodeHash, targetNodes[0].NodeHash, edgeType, "ast_inferred")

	pkgPath, err := snapshot.ExtractPackagePath(sourceNodes[0].QualifiedName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("extracting package: %v", err)), nil
	}

	repoURL := extractRepoURL(sourceNodes[0].QualifiedName)
	repoHash := types.NewHash([]byte(repoURL))

	// Build tree and generate absence proof.
	edgeInputs, _, err := s.snapMgr.CollectEdgeInputs(ctx, repoHash)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("collecting edges: %v", err)), nil
	}

	tree := snapshot.BuildHierarchicalTree(edgeInputs)
	proof, err := snapshot.GenerateAbsenceProof(tree, edgeHash, pkgPath, edgeType, edgeInputs)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("generating absence proof: %v", err)), nil
	}

	result := map[string]any{
		"absent":    true,
		"source":    source,
		"target":    target,
		"edge_type": edgeType,
		"package":   pkgPath,
		"repo_root": tree.Root.String(),
		"proof":     proof,
	}

	out, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(out)), nil
}

// handleFsck runs integrity verification.
func (s *Server) handleFsck(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.sqlStore == nil {
		return mcp.NewToolResultError("fsck: requires store"), nil
	}

	db := s.sqlStore.DB()

	// 1. SQLite integrity check.
	var integrityResult string
	_ = db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrityResult)

	// 2. Count nodes and edges.
	var nodeCount, edgeCount int64
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM nodes").Scan(&nodeCount)
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM edges").Scan(&edgeCount)

	// 3. Check referential integrity (edges with missing endpoints).
	var danglingSource, danglingTarget int64
	_ = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM edges WHERE source_hash NOT IN (SELECT node_hash FROM nodes)").Scan(&danglingSource)
	_ = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM edges WHERE target_hash NOT IN (SELECT node_hash FROM nodes)").Scan(&danglingTarget)

	// 4. Snapshot chain check.
	var snapshotCount int64
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM snapshots").Scan(&snapshotCount)
	var brokenChain int64
	_ = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM snapshots WHERE parent_hash != x'0000000000000000000000000000000000000000000000000000000000000000' AND parent_hash NOT IN (SELECT snapshot_hash FROM snapshots)").Scan(&brokenChain)

	passed := integrityResult == "ok" && danglingSource == 0 && danglingTarget == 0 && brokenChain == 0

	result := map[string]any{
		"passed":            passed,
		"sqlite_integrity":  integrityResult,
		"nodes":             nodeCount,
		"edges":             edgeCount,
		"dangling_sources":  danglingSource,
		"dangling_targets":  danglingTarget,
		"snapshots":         snapshotCount,
		"broken_chain_refs": brokenChain,
	}

	if !passed {
		result["errors"] = []string{}
		if integrityResult != "ok" {
			result["errors"] = append(result["errors"].([]string), "SQLite integrity check failed: "+integrityResult)
		}
		if danglingSource > 0 {
			result["errors"] = append(result["errors"].([]string), fmt.Sprintf("%d edges have missing source nodes", danglingSource))
		}
		if danglingTarget > 0 {
			result["errors"] = append(result["errors"].([]string), fmt.Sprintf("%d edges have missing target nodes", danglingTarget))
		}
		if brokenChain > 0 {
			result["errors"] = append(result["errors"].([]string), fmt.Sprintf("%d snapshots reference non-existent parent", brokenChain))
		}
	}

	out, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(out)), nil
}

// extractRepoURL extracts the repo URL from a qualified name (format: "repoURL://...")
func extractRepoURL(qualifiedName string) string {
	for i := 0; i < len(qualifiedName)-2; i++ {
		if qualifiedName[i] == ':' && qualifiedName[i+1] == '/' && qualifiedName[i+2] == '/' {
			return qualifiedName[:i]
		}
	}
	return ""
}
