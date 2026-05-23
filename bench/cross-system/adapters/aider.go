package adapters

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/blackwell-systems/knowing/bench/cross-system/benchtype"
	"github.com/blackwell-systems/knowing/bench/cross-system/normalize"
)

// Aider implements benchtype.Adapter for Aider's repo-map (PageRank on reference graph).
// Requires: Python with aider-chat installed (`pip install aider-chat`)
//
// Aider's repo-map is internal to its codebase. We invoke it via a Python bridge script
// that imports aider's RepoMap class and returns the map as JSON.
type Aider struct{}

func NewAider() *Aider { return &Aider{} }

func (a *Aider) Name() string { return "aider" }

func (a *Aider) Index(_ string) (int64, error) { return 0, nil } // Aider indexes on-the-fly

func (a *Aider) Retrieve(repoPath string, task benchtype.Task, tokenBudget int) (benchtype.RetrievalResult, error) {
	start := time.Now()

	// Python bridge script that invokes Aider's RepoMap
	script := fmt.Sprintf(`
import json, sys, os
sys.path.insert(0, os.path.expanduser("~/.local/lib/python3.12/site-packages"))
try:
    from aider.repomap import RepoMap
    from aider.io import InputOutput
    from aider.models import Model

    io = InputOutput(yes=True)
    model = Model("gpt-4o")  # needed for token counting only

    rm = RepoMap(
        root=%q,
        main_model=model,
        io=io,
    )

    # Get all files in repo
    import glob
    all_files = glob.glob(os.path.join(%q, "**/*.*"), recursive=True)
    all_files = [f for f in all_files if not any(d in f for d in ['.git/', 'node_modules/', '__pycache__/', 'vendor/'])]

    # Extract identifiers from task description as mentioned_idents
    words = set(%q.split())

    repo_map = rm.get_repo_map(
        chat_files=[],
        other_files=all_files[:2000],  # cap for performance
        mentioned_fnames=set(),
        mentioned_idents=words,
    )

    # Parse the tree-context output to extract symbols
    symbols = []
    for line in (repo_map or "").split("\n"):
        line = line.strip()
        if not line or line.startswith("│") or line.startswith("├") or line.startswith("└"):
            # tree-sitter tree-context format: extract symbol names
            parts = line.lstrip("│├└─ ").strip()
            if parts and not parts.endswith("/") and "." in parts:
                symbols.append(parts)
        elif "def " in line or "class " in line or "func " in line:
            # Fallback: declaration lines
            for kw in ["def ", "class ", "func "]:
                if kw in line:
                    name = line.split(kw, 1)[1].split("(")[0].strip()
                    if name:
                        symbols.append(name)

    # Token count of the full repo map
    import tiktoken
    try:
        enc = tiktoken.encoding_for_model("gpt-4o")
        token_count = len(enc.encode(repo_map or ""))
    except:
        token_count = len((repo_map or "")) // 4

    print(json.dumps({"symbols": symbols[:20], "tokens": token_count, "raw_length": len(repo_map or "")}))

except Exception as e:
    print(json.dumps({"error": str(e), "symbols": [], "tokens": 0}))
`, repoPath, repoPath, task.Description)

	// Use the aider-bench venv if available, fallback to system python3.
	pythonPath := "python3"
	if _, err := exec.LookPath("/tmp/aider-bench/bin/python3"); err == nil {
		pythonPath = "/tmp/aider-bench/bin/python3"
	}
	cmd := exec.Command(pythonPath, "-c", script)
	output, err := cmd.Output()
	if err != nil {
		return benchtype.RetrievalResult{
			System: "aider",
			TaskID: task.ID,
			Error:  fmt.Sprintf("aider bridge failed: %v", err),
		}, nil
	}

	latency := time.Since(start).Milliseconds()

	var result struct {
		Symbols []string `json:"symbols"`
		Tokens  int      `json:"tokens"`
		Error   string   `json:"error"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return benchtype.RetrievalResult{
			System: "aider",
			TaskID: task.ID,
			Error:  fmt.Sprintf("parse error: %v", err),
		}, nil
	}
	if result.Error != "" {
		return benchtype.RetrievalResult{
			System: "aider",
			TaskID: task.ID,
			Error:  result.Error,
		}, nil
	}

	symbols := make([]benchtype.RetrievedSymbol, len(result.Symbols))
	for i, s := range result.Symbols {
		symbols[i] = benchtype.RetrievedSymbol{
			QualifiedName: s,
			Normalized:    normalize.Symbol(s),
			Rank:          i + 1,
		}
	}

	return benchtype.RetrievalResult{
		System:     "aider",
		TaskID:     task.ID,
		Symbols:    symbols,
		TokensUsed: result.Tokens,
		LatencyMs:  latency,
	}, nil
}

func (a *Aider) SupportsLearning() bool { return false }

func (a *Aider) RecordFeedback(_ string, _ benchtype.Task, _ []string) error { return nil }

func (a *Aider) Reset(_ string) error { return nil }

// IsAvailable checks if aider is installed.
func (a *Aider) IsAvailable() bool {
	// Check venv first, then system python.
	for _, py := range []string{"/tmp/aider-bench/bin/python3", "python3"} {
		cmd := exec.Command(py, "-c", "import aider.repomap")
		if cmd.Run() == nil {
			return true
		}
	}
	return false
}

// aiderVersion returns the installed version or empty string.
func aiderVersion() string {
	output, err := exec.Command("python3", "-c", "import aider; print(aider.__version__)").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
