package agent_efficiency

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SessionMetrics holds all extracted metrics for a single benchmark session.
type SessionMetrics struct {
	SessionID   string
	TaskID      string
	TotalTokens int
	InputTokens int
	OutputTokens int
	ToolCalls       int
	ToolCallsByType map[string]int
	Turns           int
	WallClockMs     int64
	FilesRead       []string

	// Correctness fields: populated by ScoreCorrectness.
	FoundRelevantFiles int
	FoundKeySymbols    int
	AnswerCorrectness  float64
}

// transcriptLine is the minimal structure of a Claude Code JSONL line.
type transcriptLine struct {
	Type    string          `json:"type"`
	Message *transcriptMsg  `json:"message,omitempty"`
	// Some line formats embed the role at the top level.
	Role    string          `json:"role,omitempty"`
}

type transcriptMsg struct {
	Role    string             `json:"role"`
	Content json.RawMessage    `json:"content"`
	Usage   *transcriptUsage   `json:"usage,omitempty"`
}

type transcriptUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// contentBlock is one element inside a message's content array.
type contentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// toolInput holds the fields we care about from common tool calls.
type toolInput struct {
	FilePath string `json:"file_path"`
	Path     string `json:"path"`
}

// ParseTranscript reads a Claude Code JSONL file and returns a SessionMetrics.
// Unknown line types and missing fields are silently skipped.
func ParseTranscript(path string) (SessionMetrics, error) {
	f, err := os.Open(path)
	if err != nil {
		return SessionMetrics{}, err
	}
	defer f.Close()

	// Derive task ID and session ID from filename convention:
	//   <task-id>-<mode>.jsonl  or  <task-id>-<mode>-<sessionid>.jsonl
	base := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	m := SessionMetrics{
		SessionID:       base,
		ToolCallsByType: make(map[string]int),
	}

	// Extract TaskID from filename: everything before the last dash segment.
	parts := strings.Split(base, "-")
	if len(parts) >= 2 {
		// Convention: last segment is mode (control/treatment) or session suffix.
		// We keep everything before the last recognised mode token as TaskID.
		for i := len(parts) - 1; i >= 1; i-- {
			if parts[i] == "control" || parts[i] == "treatment" {
				m.TaskID = strings.Join(parts[:i], "-")
				break
			}
		}
		if m.TaskID == "" {
			m.TaskID = strings.Join(parts[:len(parts)-1], "-")
		}
	}

	var allAssistantText strings.Builder
	filesSeen := make(map[string]bool)
	var firstTimestamp, lastTimestamp time.Time

	scanner := bufio.NewScanner(f)
	const maxLineSize = 10 * 1024 * 1024 // 10 MB per line
	buf := make([]byte, maxLineSize)
	scanner.Buffer(buf, maxLineSize)

	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}

		var line transcriptLine
		if err := json.Unmarshal(raw, &line); err != nil {
			// Skip lines we cannot parse.
			continue
		}

		// Handle timestamp extraction from any line that carries one.
		var ts struct {
			Timestamp string `json:"timestamp"`
		}
		_ = json.Unmarshal(raw, &ts)
		if ts.Timestamp != "" {
			t, err := time.Parse(time.RFC3339Nano, ts.Timestamp)
			if err == nil {
				if firstTimestamp.IsZero() {
					firstTimestamp = t
				}
				lastTimestamp = t
			}
		}

		msg := line.Message
		if msg == nil {
			// Some transcript formats have the message fields at the top level.
			// Attempt to reparse as a message directly.
			var direct transcriptMsg
			if err := json.Unmarshal(raw, &direct); err == nil && direct.Role != "" {
				msg = &direct
			} else {
				continue
			}
		}

		role := msg.Role
		if role == "" {
			role = line.Role
		}

		switch role {
		case "assistant":
			m.Turns++

			if msg.Usage != nil {
				m.InputTokens += msg.Usage.InputTokens
				m.OutputTokens += msg.Usage.OutputTokens
				m.TotalTokens += msg.Usage.InputTokens + msg.Usage.OutputTokens
			}

			var blocks []contentBlock
			// Content can be a string or an array of blocks.
			if len(msg.Content) > 0 && msg.Content[0] == '[' {
				_ = json.Unmarshal(msg.Content, &blocks)
			} else if len(msg.Content) > 0 && msg.Content[0] == '"' {
				var s string
				if err := json.Unmarshal(msg.Content, &s); err == nil {
					allAssistantText.WriteString(s)
				}
			}

			for _, block := range blocks {
				switch block.Type {
				case "text":
					allAssistantText.WriteString(block.Text)
				case "tool_use":
					m.ToolCalls++
					toolName := block.Name
					m.ToolCallsByType[toolName]++

					// Extract file path for Read tool calls.
					if toolName == "Read" && len(block.Input) > 0 {
						var inp toolInput
						if err := json.Unmarshal(block.Input, &inp); err == nil {
							fp := inp.FilePath
							if fp == "" {
								fp = inp.Path
							}
							if fp != "" && !filesSeen[fp] {
								filesSeen[fp] = true
								m.FilesRead = append(m.FilesRead, fp)
							}
						}
					}
				}
			}

		case "user":
			// User turns don't contribute to metrics directly but carry timestamps.
		}
	}

	if err := scanner.Err(); err != nil {
		return m, err
	}

	if !firstTimestamp.IsZero() && !lastTimestamp.IsZero() {
		m.WallClockMs = lastTimestamp.Sub(firstTimestamp).Milliseconds()
	}

	return m, nil
}

// ScoreCorrectness computes FoundRelevantFiles, FoundKeySymbols, and
// AnswerCorrectness against a GroundTruth. It updates m in place.
func ScoreCorrectness(m *SessionMetrics, gt GroundTruth, allAssistantText string) {
	// Relevant files: check how many ground-truth files appear in FilesRead.
	readSet := make(map[string]bool, len(m.FilesRead))
	for _, f := range m.FilesRead {
		// Normalise: strip leading path separators and use forward slashes.
		readSet[filepath.ToSlash(strings.TrimLeft(f, "/"))] = true
	}
	for _, rel := range gt.RelevantFiles {
		norm := filepath.ToSlash(strings.TrimLeft(rel, "/"))
		for rf := range readSet {
			if strings.Contains(rf, norm) || strings.Contains(norm, rf) {
				m.FoundRelevantFiles++
				break
			}
		}
	}

	// Key symbols: count how many appear anywhere in assistant output.
	lower := strings.ToLower(allAssistantText)
	for _, sym := range gt.KeySymbols {
		if strings.Contains(lower, strings.ToLower(sym)) {
			m.FoundKeySymbols++
		}
	}

	// Answer correctness: fraction of AnswerKeywords found in assistant output.
	if len(gt.AnswerKeywords) > 0 {
		found := 0
		for _, kw := range gt.AnswerKeywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				found++
			}
		}
		m.AnswerCorrectness = float64(found) / float64(len(gt.AnswerKeywords))
	}
}

// ParseTranscriptWithScoring parses a transcript and scores correctness against
// the provided ground truth in a single call.
func ParseTranscriptWithScoring(path string, gt GroundTruth) (SessionMetrics, error) {
	f, err := os.Open(path)
	if err != nil {
		return SessionMetrics{}, err
	}
	defer f.Close()

	base := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	m := SessionMetrics{
		SessionID:       base,
		ToolCallsByType: make(map[string]int),
	}

	parts := strings.Split(base, "-")
	if len(parts) >= 2 {
		for i := len(parts) - 1; i >= 1; i-- {
			if parts[i] == "control" || parts[i] == "treatment" {
				m.TaskID = strings.Join(parts[:i], "-")
				break
			}
		}
		if m.TaskID == "" {
			m.TaskID = strings.Join(parts[:len(parts)-1], "-")
		}
	}

	var allText strings.Builder
	filesSeen := make(map[string]bool)
	var firstTimestamp, lastTimestamp time.Time

	scanner := bufio.NewScanner(f)
	const maxLineSize = 10 * 1024 * 1024
	buf := make([]byte, maxLineSize)
	scanner.Buffer(buf, maxLineSize)

	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}

		var line transcriptLine
		if err := json.Unmarshal(raw, &line); err != nil {
			continue
		}

		var ts struct {
			Timestamp string `json:"timestamp"`
		}
		_ = json.Unmarshal(raw, &ts)
		if ts.Timestamp != "" {
			t, err := time.Parse(time.RFC3339Nano, ts.Timestamp)
			if err == nil {
				if firstTimestamp.IsZero() {
					firstTimestamp = t
				}
				lastTimestamp = t
			}
		}

		msg := line.Message
		if msg == nil {
			var direct transcriptMsg
			if err := json.Unmarshal(raw, &direct); err == nil && direct.Role != "" {
				msg = &direct
			} else {
				continue
			}
		}

		role := msg.Role
		if role == "" {
			role = line.Role
		}

		if role != "assistant" {
			continue
		}

		m.Turns++
		if msg.Usage != nil {
			m.InputTokens += msg.Usage.InputTokens
			m.OutputTokens += msg.Usage.OutputTokens
			m.TotalTokens += msg.Usage.InputTokens + msg.Usage.OutputTokens
		}

		var blocks []contentBlock
		if len(msg.Content) > 0 && msg.Content[0] == '[' {
			_ = json.Unmarshal(msg.Content, &blocks)
		} else if len(msg.Content) > 0 && msg.Content[0] == '"' {
			var s string
			if err := json.Unmarshal(msg.Content, &s); err == nil {
				allText.WriteString(s)
			}
		}

		for _, block := range blocks {
			switch block.Type {
			case "text":
				allText.WriteString(block.Text)
			case "tool_use":
				m.ToolCalls++
				m.ToolCallsByType[block.Name]++
				if block.Name == "Read" && len(block.Input) > 0 {
					var inp toolInput
					if err := json.Unmarshal(block.Input, &inp); err == nil {
						fp := inp.FilePath
						if fp == "" {
							fp = inp.Path
						}
						if fp != "" && !filesSeen[fp] {
							filesSeen[fp] = true
							m.FilesRead = append(m.FilesRead, fp)
						}
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return m, err
	}

	if !firstTimestamp.IsZero() && !lastTimestamp.IsZero() {
		m.WallClockMs = lastTimestamp.Sub(firstTimestamp).Milliseconds()
	}

	ScoreCorrectness(&m, gt, allText.String())
	return m, nil
}
