package metric

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func scrape(t *testing.T, r *Registry) string {
	t.Helper()
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	return rec.Body.String()
}

func TestPeerMetrics(t *testing.T) {
	r := initRegistry()

	r.SetPeerConnected("client-a", true)
	r.RecordPeerHeartbeat("client-a", time.Unix(1700000000, 0))
	out := scrape(t, r)
	for _, want := range []string{
		`backrest_peer_connected{instance_id="client-a"} 1`,
		`backrest_peer_last_heartbeat_timestamp_seconds{instance_id="client-a"} 1.7e+09`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}

	r.SetPeerConnected("client-a", false)
	if out := scrape(t, r); !strings.Contains(out, `backrest_peer_connected{instance_id="client-a"} 0`) {
		t.Errorf("expected peer to be reported disconnected:\n%s", out)
	}
}

func TestRemoteOperationKeepsOnlyLatestStatus(t *testing.T) {
	r := initRegistry()

	r.RecordRemoteOperation("client-a", "repo", "plan", "backup", "failed", time.Unix(1700000000, 0))
	r.RecordRemoteOperation("client-a", "repo", "plan", "backup", "success", time.Unix(1700003600, 0))
	r.RecordRemoteOperation("client-a", "repo", "plan", "copy", "success", time.Unix(1700003700, 0))
	out := scrape(t, r)

	if strings.Contains(out, `status="failed"`) {
		t.Errorf("stale status should have been replaced:\n%s", out)
	}
	for _, want := range []string{
		`backrest_remote_last_operation_timestamp_seconds{instance_id="client-a",op_type="backup",plan_id="plan",repo_id="repo",status="success"} 1.7000036e+09`,
		`op_type="copy"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}
}
