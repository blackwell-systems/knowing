package diff

import (
	"context"
	"math"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// benignProcessTargets lists process names that are expected/safe: compilers,
// package managers, runtimes, version control, and shells. These should not
// boost the isolation score because legitimate packages spawn them routinely
// (esbuild for compilation, pino for worker threads, etc.).
var benignProcessTargets = map[string]struct{}{
	// JavaScript / Node.js
	"node": {}, "node.exe": {},
	"npm": {}, "npx": {}, "yarn": {}, "pnpm": {},
	// Python
	"python": {}, "python3": {}, "pip": {}, "pip3": {},
	// Go / Rust / Java / TypeScript compilers
	"go": {}, "cargo": {}, "rustc": {}, "javac": {}, "tsc": {},
	// Version control
	"git": {},
	// Shells (benign when running known scripts)
	"sh": {}, "bash": {}, "zsh": {},
	// Node.js parallelism patterns
	"worker_threads": {}, "cluster.fork": {},
}

// isBenignProcessTarget returns true if the process target name refers to a
// known-safe executable (runtime, compiler, package manager, shell, or worker).
func isBenignProcessTarget(target string) bool {
	// Strip directory prefix: "/usr/bin/node" -> "node"
	base := filepath.Base(target)
	// Normalize to lowercase for case-insensitive matching.
	base = strings.ToLower(base)
	_, ok := benignProcessTargets[base]
	return ok
}

// IsolationResult represents the isolation score for a single changed file.
// Higher scores indicate more isolated files with dangerous outbound edges,
// which are more suspicious in a supply-chain context.
type IsolationResult struct {
	File          string   `json:"file"`
	Score         float64  `json:"score"`
	InboundEdges  int      `json:"inbound_edges"`
	OutboundEdges int      `json:"outbound_edges"`
	HookExecuted  bool     `json:"hook_executed"`
	ReadsEnv      []string `json:"reads_env,omitempty"`
	ExecutesProc  []string `json:"executes_process,omitempty"`
}

// ComputeIsolation computes the isolation score for each changed file.
// The score captures how "isolated" a file is (few inbound edges from unchanged
// files) combined with how "dangerous" its outbound edges are (reads_env,
// executes_process). Files that are isolated AND have dangerous outbound edges
// score higher, indicating potential supply-chain risk.
func ComputeIsolation(ctx context.Context, store types.GraphStore, changedFiles []types.Hash) ([]IsolationResult, error) {
	if len(changedFiles) == 0 {
		return nil, nil
	}

	// Build a set of all node hashes belonging to changed files.
	changedNodeSet := make(map[types.Hash]struct{})
	// Map file hash -> file path (from first node we find).
	fileToPath := make(map[types.Hash]string)
	// Map file hash -> nodes in that file.
	fileToNodes := make(map[types.Hash][]types.Node)

	for _, fh := range changedFiles {
		nodes, err := store.NodesByFileHash(ctx, fh)
		if err != nil {
			return nil, err
		}
		fileToNodes[fh] = nodes
		for _, n := range nodes {
			changedNodeSet[n.NodeHash] = struct{}{}
			if _, ok := fileToPath[fh]; !ok {
				fileToPath[fh] = n.QualifiedName
			}
		}
	}

	results := make([]IsolationResult, 0, len(changedFiles))

	for _, fh := range changedFiles {
		nodes := fileToNodes[fh]
		result := IsolationResult{
			File: fileToPath[fh],
		}

		var envVars []string
		var procs []string
		var suspiciousProcs []string
		var inbound int
		var outboundDangerous int
		var hookExecuted bool

		for _, n := range nodes {
			// Count inbound edges from nodes NOT in changedFiles.
			edgesIn, err := store.EdgesTo(ctx, n.NodeHash, "")
			if err != nil {
				return nil, err
			}
			for _, e := range edgesIn {
				if _, inChanged := changedNodeSet[e.SourceHash]; !inChanged {
					inbound++
				}
			}

			// Count outbound dangerous edges and collect targets.
			edgesOut, err := store.EdgesFrom(ctx, n.NodeHash, "")
			if err != nil {
				return nil, err
			}
			for _, e := range edgesOut {
				switch e.EdgeType {
				case edgetype.ReadsEnv:
					outboundDangerous++
					target, err := store.GetNode(ctx, e.TargetHash)
					if err == nil && target != nil {
						envVars = append(envVars, target.QualifiedName)
					}
				case edgetype.ExecutesProcess:
					target, err := store.GetNode(ctx, e.TargetHash)
					if err == nil && target != nil {
						procs = append(procs, target.QualifiedName)
						if isBenignProcessTarget(target.QualifiedName) {
							// Benign targets (compilers, runtimes, package managers)
							// are collected but do not boost the danger score
							// and do not trigger hook detection.
						} else {
							suspiciousProcs = append(suspiciousProcs, target.QualifiedName)
							outboundDangerous++
							hookExecuted = true
						}
					} else {
						// Unknown target (can't resolve): treat as suspicious.
						outboundDangerous++
						hookExecuted = true
					}
				}
			}
		}

		// Compute isolation score.
		// inbound_factor: well-connected files (many inbound) get low isolation.
		// Use a gentler curve: even 1-2 inbound edges shouldn't zero out the score.
		inboundCapped := math.Min(float64(inbound), 10.0)
		inboundFactor := 1.0 - (inboundCapped / 10.0 * 0.7) // max 70% reduction from inbound

		// outbound_factor: more dangerous outbound edges increase the score.
		// Key insight: reads_env alone is normal (config, feature flags, logging).
		// The supply chain signal is the COMBINATION: reads credentials AND
		// spawns processes or makes network calls (exfiltration pattern).
		// reads_env-only gets 0.2x weight; with executes_process it gets full weight.
		hasSuspiciousProc := len(suspiciousProcs) > 0
		envOnly := len(envVars) > 0 && !hasSuspiciousProc
		outboundCapped := math.Min(float64(outboundDangerous), 5.0)
		outboundFactor := outboundCapped / 5.0
		if envOnly {
			outboundFactor *= 0.2 // reads_env alone is not suspicious
		}

		// hook_factor: hook execution amplifies the score.
		hookFactor := 1.0
		if hookExecuted {
			hookFactor = 1.5
		}

		score := inboundFactor * outboundFactor * hookFactor
		// Clamp to [0.0, 1.0].
		score = math.Max(0.0, math.Min(1.0, score))

		result.Score = score
		result.InboundEdges = inbound
		result.OutboundEdges = outboundDangerous
		result.HookExecuted = hookExecuted
		result.ReadsEnv = envVars
		result.ExecutesProc = procs

		results = append(results, result)
	}

	return results, nil
}

