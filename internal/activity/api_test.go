package activity

import (
	"net/http/httptest"
	"testing"
)

func TestFilterFromRequest_ClampsLimit(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/activity?limit=500000", nil)

	filter, err := filterFromRequest(req)
	if err != nil {
		t.Fatalf("filterFromRequest: %v", err)
	}
	if filter.Limit != maxQueryLimit {
		t.Fatalf("Limit = %d, want %d", filter.Limit, maxQueryLimit)
	}
}

func TestFilterFromRequest_ParsesAgentID(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/activity?agentId=cli", nil)

	filter, err := filterFromRequest(req)
	if err != nil {
		t.Fatalf("filterFromRequest: %v", err)
	}
	if filter.AgentID != "cli" {
		t.Fatalf("AgentID = %q, want cli", filter.AgentID)
	}
}

func TestFilterFromRequest_RejectsInvalidLimit(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/activity?limit=0", nil)

	_, err := filterFromRequest(req)
	if err == nil {
		t.Fatal("expected error for invalid limit")
	}
}
