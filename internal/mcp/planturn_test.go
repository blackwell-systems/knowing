package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func makePlanTurnRequest(task string) mcp.CallToolRequest {
	return makeCallToolRequest("plan_turn", map[string]any{"task": task})
}

type planTurnResponse struct {
	Suggestions []suggestion `json:"suggestions"`
}

func parsePlanTurnResponse(t *testing.T, result *mcp.CallToolResult) planTurnResponse {
	t.Helper()
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}
	text := result.Content[0].(mcp.TextContent).Text
	var resp planTurnResponse
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("failed to parse response: %v\nraw: %s", err, text)
	}
	return resp
}

func TestPlanTurn_TestScope(t *testing.T) {
	s := &Server{}
	req := makePlanTurnRequest("which tests are affected by this change in scope")
	result, err := s.handlePlanTurn(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	resp := parsePlanTurnResponse(t, result)
	if len(resp.Suggestions) == 0 {
		t.Fatal("expected at least one suggestion")
	}
	if resp.Suggestions[0].Tool != "test_scope" {
		t.Errorf("expected first suggestion to be test_scope, got %s", resp.Suggestions[0].Tool)
	}
}

func TestPlanTurn_BlastRadius(t *testing.T) {
	s := &Server{}
	req := makePlanTurnRequest("what is the blast radius of FuncX")
	result, err := s.handlePlanTurn(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	resp := parsePlanTurnResponse(t, result)
	if len(resp.Suggestions) == 0 {
		t.Fatal("expected at least one suggestion")
	}
	found := false
	for _, sg := range resp.Suggestions {
		if sg.Tool == "blast_radius" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected blast_radius in suggestions, got %v", resp.Suggestions)
	}
}

func TestPlanTurn_FlowBetween(t *testing.T) {
	s := &Server{}
	req := makePlanTurnRequest("find path from A to B")
	result, err := s.handlePlanTurn(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	resp := parsePlanTurnResponse(t, result)
	if len(resp.Suggestions) == 0 {
		t.Fatal("expected at least one suggestion")
	}
	found := false
	for _, sg := range resp.Suggestions {
		if sg.Tool == "flow_between" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected flow_between in suggestions, got %v", resp.Suggestions)
	}
}

func TestPlanTurn_Communities(t *testing.T) {
	s := &Server{}
	req := makePlanTurnRequest("which community does this belong to")
	result, err := s.handlePlanTurn(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	resp := parsePlanTurnResponse(t, result)
	if len(resp.Suggestions) == 0 {
		t.Fatal("expected at least one suggestion")
	}
	found := false
	for _, sg := range resp.Suggestions {
		if sg.Tool == "communities" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected communities in suggestions, got %v", resp.Suggestions)
	}
}

func TestPlanTurn_Fallback(t *testing.T) {
	s := &Server{}
	req := makePlanTurnRequest("xyzzy frobnicator quintux")
	result, err := s.handlePlanTurn(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	resp := parsePlanTurnResponse(t, result)
	if len(resp.Suggestions) != 1 {
		t.Fatalf("expected 1 fallback suggestion, got %d", len(resp.Suggestions))
	}
	if resp.Suggestions[0].Tool != "context_for_task" {
		t.Errorf("expected fallback to be context_for_task, got %s", resp.Suggestions[0].Tool)
	}
}

func TestPlanTurn_MaxFourSuggestions(t *testing.T) {
	s := &Server{}
	// Task that matches many rules
	req := makePlanTurnRequest("test blast radius diff flow community dataflow stale index feedback")
	result, err := s.handlePlanTurn(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	resp := parsePlanTurnResponse(t, result)
	if len(resp.Suggestions) > 4 {
		t.Errorf("expected at most 4 suggestions, got %d", len(resp.Suggestions))
	}
}

func TestPlanTurn_MissingTask(t *testing.T) {
	s := &Server{}
	req := makeCallToolRequest("plan_turn", map[string]any{})
	result, err := s.handlePlanTurn(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error result for missing task argument")
	}
}
