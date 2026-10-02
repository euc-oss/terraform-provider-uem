package profile

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
)

// TestDefaultProfileServiceFactory_NoDiscoveryCall pins internal-task: building the
// default profile service must not call UEM. The eager Discover it replaced
// listed every profile via /api/mdm/profiles/search on each service build.
func TestDefaultProfileServiceFactory_NoDiscoveryCall(t *testing.T) {
	var calls atomic.Int32
	c, server := createTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ProfileList":[],"Page":0,"PageSize":500,"Total":0}`))
	})
	defer server.Close()

	svc, err := defaultProfileServiceFactory(context.Background(), c)
	if err != nil {
		t.Fatalf("defaultProfileServiceFactory returned error: %v", err)
	}
	if svc == nil {
		t.Fatal("defaultProfileServiceFactory returned a nil service")
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("defaultProfileServiceFactory made %d HTTP call(s); want 0 (no eager discovery)", got)
	}
}
