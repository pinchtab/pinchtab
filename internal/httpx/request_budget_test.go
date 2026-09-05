package httpx

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

// The budget is decided from the operation, once: cheap reads get the read
// budget, every mutation and every rendering read keeps the navigation ceiling.
func TestRequestBudgetIsDecidedFromTheOperation(t *testing.T) {
	cases := []struct {
		method, path string
		long         bool
	}{
		{http.MethodGet, "/tabs", false},
		{http.MethodGet, "/instances/inst_1/tabs", false},
		{http.MethodGet, "/health", false},
		{http.MethodGet, "/tabs/abc/title", false},
		{http.MethodGet, "/network", false},
		{http.MethodGet, "/network/stream", true},
		{http.MethodGet, "/tabs/abc/network/export", true},
		{http.MethodGet, "/screenshot", true},
		{http.MethodGet, "/tabs/abc/snapshot", true},
		{http.MethodGet, "/pdf", true},
		{http.MethodPost, "/navigate", true},
		{http.MethodPost, "/tabs/abc/action", true},
		{http.MethodPost, "/close", true},
		{http.MethodDelete, "/network/route", true},
	}
	for _, tc := range cases {
		got := RequestBudget(tc.method, tc.path)
		want := ReadHTTPDuration
		if tc.long {
			want = LongOperationHTTPBudget
		}
		if got != want {
			t.Errorf("%s %s budget = %v, want %v", tc.method, tc.path, got, want)
		}
	}
	if ReadHTTPDuration >= LongOperationHTTPBudget {
		t.Fatalf("read budget %v is not shorter than the long budget %v", ReadHTTPDuration, LongOperationHTTPBudget)
	}
	if LongOperationHTTPBudget != MaxNavigationHTTPDuration {
		t.Fatalf("navigation keeps its ceiling: long budget = %v, want %v", LongOperationHTTPBudget, MaxNavigationHTTPDuration)
	}
}

func TestUnreachableErrorNamesTheInstanceAndTheBudget(t *testing.T) {
	timedOut := UnreachableError("inst_9", "127.0.0.1:9868", 30*time.Second, context.DeadlineExceeded)
	for _, want := range []string{"instance inst_9", "127.0.0.1:9868", "did not answer within its 30s budget"} {
		if !strings.Contains(timedOut.Error(), want) {
			t.Errorf("%q lacks %q", timedOut, want)
		}
	}
	if !errors.Is(timedOut, context.DeadlineExceeded) {
		t.Error("the deadline cause was not wrapped")
	}
	refused := UnreachableError("", "127.0.0.1:9868", 30*time.Second, errors.New("connection refused"))
	if strings.Contains(refused.Error(), "budget") || !strings.Contains(refused.Error(), "127.0.0.1:9868 unreachable") {
		t.Errorf("a connection failure must not claim a budget ran out: %q", refused)
	}
}
