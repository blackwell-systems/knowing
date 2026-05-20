package cloudextractor

import (
	"regexp"
	"strings"

	"github.com/blackwell-systems/knowing/internal/types"
)

// jobDef is a convenience type for parsed job YAML used by deployment and test
// edge extraction helpers.
type jobDef struct {
	Name  string
	Hash  types.Hash
	Steps []stepDef
}

// stepDef holds a single workflow step's relevant fields.
type stepDef struct {
	Run  string
	Uses string
	Name string
	With map[string]string
}

// deployActionPatterns are known deployment action prefixes/substrings.
var deployActionPatterns = []string{
	"aws-actions/amazon-ecs-deploy-task-definition",
	"azure/webapps-deploy",
	"google-github-actions/deploy-cloudrun",
	"docker/build-push-action",
}

// deployRunPatterns detect deployment commands in run steps.
var deployRunPatterns = []*regexp.Regexp{
	regexp.MustCompile(`docker\s+push\s+(\S+)`),
	regexp.MustCompile(`kubectl\s+apply\s+-f\s+(\S+)`),
	regexp.MustCompile(`kubectl\s+rollout`),
	regexp.MustCompile(`helm\s+upgrade\s+(\S+)`),
	regexp.MustCompile(`terraform\s+apply`),
	regexp.MustCompile(`serverless\s+deploy`),
	regexp.MustCompile(`flyctl\s+deploy`),
	regexp.MustCompile(`gcloud.*deploy`),
}

// testRunPatterns detect test commands in run steps.
var testRunPatterns = struct {
	goTest *regexp.Regexp
	npm    *regexp.Regexp
	pytest *regexp.Regexp
	cargo  *regexp.Regexp
	make   *regexp.Regexp
}{
	goTest: regexp.MustCompile(`go\s+test\s+(.+)`),
	npm:    regexp.MustCompile(`(?:npm\s+test|npx\s+jest)`),
	pytest: regexp.MustCompile(`(?:pytest|python\s+-m\s+pytest)`),
	cargo:  regexp.MustCompile(`cargo\s+test`),
	make:   regexp.MustCompile(`make\s+test`),
}

// extractDeployedByEdges parses job definitions for deployment patterns and
// creates 'deployed_by' edges from synthetic deployment target nodes to the
// workflow node.
func extractDeployedByEdges(opts types.ExtractOptions, workflowHash types.Hash, jobDefs map[string]jobDef) ([]types.Node, []types.Edge) {
	var nodes []types.Node
	var edges []types.Edge

	for jobID, job := range jobDefs {
		for _, step := range job.Steps {
			target := detectDeployTarget(step, jobID)
			if target == "" {
				continue
			}

			targetHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, target, "deployment_target")
			targetQN := buildQN(opts.RepoURL, opts.FilePath, "deployment_target", target)
			targetNode := types.Node{
				NodeHash:      targetHash,
				FileHash:      opts.FileHash,
				QualifiedName: targetQN,
				Kind:          "deployment_target",
			}
			nodes = append(nodes, targetNode)

			edge := makeEdge(targetHash, workflowHash, "deployed_by")
			edge.Confidence = 0.9
			edges = append(edges, edge)
		}
	}

	return nodes, edges
}

// extractTestedByEdges parses job definitions for test command patterns and
// creates 'tested_by' edges from synthetic test target nodes to the job node.
func extractTestedByEdges(opts types.ExtractOptions, jobDefs map[string]jobDef) ([]types.Node, []types.Edge) {
	var nodes []types.Node
	var edges []types.Edge

	for _, job := range jobDefs {
		for _, step := range job.Steps {
			if step.Run == "" {
				continue
			}

			targets := detectTestTargets(step.Run)
			for _, target := range targets {
				testTargetHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, target, "test_target")
				testTargetQN := buildQN(opts.RepoURL, opts.FilePath, "test_target", target)
				testTargetNode := types.Node{
					NodeHash:      testTargetHash,
					FileHash:      opts.FileHash,
					QualifiedName: testTargetQN,
					Kind:          "test_target",
				}
				nodes = append(nodes, testTargetNode)

				edge := makeEdge(testTargetHash, job.Hash, "tested_by")
				edge.Confidence = 0.8
				edges = append(edges, edge)
			}
		}
	}

	return nodes, edges
}

// detectDeployTarget examines a step and returns a deployment target name,
// or empty string if no deployment is detected.
func detectDeployTarget(step stepDef, jobID string) string {
	// Check uses field for deployment actions.
	if step.Uses != "" {
		if isDeployAction(step.Uses) {
			// Try to extract target from with inputs.
			if target := extractDeployTargetFromWith(step.With); target != "" {
				return target
			}
			return jobID
		}
	}

	// Check run field for deployment commands.
	if step.Run != "" {
		if target := extractDeployTargetFromRun(step.Run, jobID); target != "" {
			return target
		}
	}

	return ""
}

// isDeployAction checks if a uses value matches a known deployment action or
// contains "deploy" as a heuristic.
func isDeployAction(uses string) bool {
	usesLower := strings.ToLower(uses)
	for _, pattern := range deployActionPatterns {
		if strings.HasPrefix(usesLower, strings.ToLower(pattern)) {
			return true
		}
	}
	return strings.Contains(usesLower, "deploy")
}

// extractDeployTargetFromWith extracts deployment target names from step with inputs.
func extractDeployTargetFromWith(with map[string]string) string {
	// Check common input keys for deployment targets.
	targetKeys := []string{"service", "app-name", "app_name", "image", "task-definition"}
	for _, key := range targetKeys {
		if val, ok := with[key]; ok && val != "" {
			return val
		}
	}
	return ""
}

// extractDeployTargetFromRun extracts deployment targets from run commands.
func extractDeployTargetFromRun(run string, jobID string) string {
	for _, pattern := range deployRunPatterns {
		matches := pattern.FindStringSubmatch(run)
		if matches != nil {
			// If there's a capture group with a specific target, use it.
			if len(matches) > 1 && matches[1] != "" {
				return matches[1]
			}
			// Otherwise fall back to jobID.
			return jobID
		}
	}
	return ""
}

// detectTestTargets examines a run command and returns test target names.
func detectTestTargets(run string) []string {
	var targets []string

	// Check for Go test commands.
	if matches := testRunPatterns.goTest.FindStringSubmatch(run); matches != nil {
		args := matches[1]
		targets = append(targets, extractGoTestPackages(args)...)
		return targets
	}

	// Check for npm test / npx jest.
	if testRunPatterns.npm.MatchString(run) {
		return []string{"npm:test"}
	}

	// Check for pytest.
	if testRunPatterns.pytest.MatchString(run) {
		return []string{"pytest"}
	}

	// Check for cargo test.
	if testRunPatterns.cargo.MatchString(run) {
		return []string{"cargo:test"}
	}

	// Check for make test.
	if testRunPatterns.make.MatchString(run) {
		return []string{"make:test"}
	}

	return targets
}

// extractGoTestPackages parses the arguments of a `go test` command and returns
// normalized package paths.
func extractGoTestPackages(args string) []string {
	var packages []string
	parts := strings.Fields(args)

	for _, part := range parts {
		// Skip flags (e.g., -v, -count=1, -timeout).
		if strings.HasPrefix(part, "-") {
			continue
		}
		// Must look like a package path (starts with ./ or contains /).
		if !strings.HasPrefix(part, "./") && !strings.Contains(part, "/") {
			continue
		}

		pkg := normalizeGoPackage(part)
		if pkg != "" {
			packages = append(packages, pkg)
		}
	}

	return packages
}

// normalizeGoPackage normalizes a Go test package argument to a clean package path.
// ./internal/store/... -> internal/store
// ./... -> .
// ./pkg/auth/ -> pkg/auth
func normalizeGoPackage(pkg string) string {
	// Remove leading ./
	pkg = strings.TrimPrefix(pkg, "./")

	// Remove trailing /...
	pkg = strings.TrimSuffix(pkg, "/...")
	// Remove trailing ...
	pkg = strings.TrimSuffix(pkg, "...")

	// Remove trailing /
	pkg = strings.TrimSuffix(pkg, "/")

	if pkg == "" {
		return "."
	}

	return pkg
}
