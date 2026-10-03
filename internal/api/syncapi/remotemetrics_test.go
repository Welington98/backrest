package syncapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	v1 "github.com/garethgeorge/backrest/gen/go/v1"
	"github.com/garethgeorge/backrest/internal/metric"
)

func TestRecordRemoteOperationMetric(t *testing.T) {
	h := &syncSessionHandlerServer{peer: &v1.Multihost_Peer{InstanceId: "metrics-client", Keyid: "k"}}
	endMs := int64(1700000000000)

	mk := func(status v1.OperationStatus, endMs int64, op *v1.Operation) *v1.Operation {
		op.RepoId, op.PlanId, op.Status, op.UnixTimeEndMs = "r", "p", status, endMs
		return op
	}
	backup := func(dry bool) *v1.Operation {
		return &v1.Operation{Op: &v1.Operation_OperationBackup{OperationBackup: &v1.OperationBackup{DryRun: dry}}}
	}

	h.recordRemoteOperationMetric(mk(v1.OperationStatus_STATUS_INPROGRESS, 0, backup(false))) // unfinished
	h.recordRemoteOperationMetric(mk(v1.OperationStatus_STATUS_SUCCESS, endMs, backup(true))) // dry run
	h.recordRemoteOperationMetric(mk(v1.OperationStatus_STATUS_USER_CANCELLED, endMs, backup(false)))
	h.recordRemoteOperationMetric(mk(v1.OperationStatus_STATUS_SUCCESS, endMs, &v1.Operation{ // not a tracked type
		Op: &v1.Operation_OperationStats{OperationStats: &v1.OperationStats{}},
	}))

	scrape := func() string {
		rec := httptest.NewRecorder()
		metric.GetRegistry().Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
		return rec.Body.String()
	}
	if strings.Contains(scrape(), `instance_id="metrics-client"`) {
		t.Fatalf("operations without an outcome must not be exported:\n%s", scrape())
	}

	h.recordRemoteOperationMetric(mk(v1.OperationStatus_STATUS_ERROR, endMs, backup(false)))
	if want := `status="failed"`; !strings.Contains(scrape(), want) {
		t.Fatalf("expected %q after a failed backup:\n%s", want, scrape())
	}
	h.recordRemoteOperationMetric(mk(v1.OperationStatus_STATUS_SUCCESS, endMs+1000, backup(false)))
	out := scrape()
	if !strings.Contains(out, `instance_id="metrics-client",op_type="backup",plan_id="p",repo_id="r",status="success"} 1.700000001e+09`) {
		t.Fatalf("expected the success to replace the failure:\n%s", out)
	}
}
