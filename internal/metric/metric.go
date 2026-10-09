package metric

import (
	"net/http"
	"slices"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	globalRegistry = initRegistry()
)

func initRegistry() *Registry {

	commonDims := []string{"repo_id", "plan_id"}

	registry := &Registry{
		reg: prometheus.NewRegistry(),
		backupBytesProcessed: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "backrest_backup_bytes_processed",
			Help: "The total number of bytes processed during a backup",
		}, commonDims),
		backupBytesAdded: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "backrest_backup_bytes_added",
			Help: "The total number of bytes added during a backup",
		}, commonDims),
		backupFileWarnings: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "backrest_backup_file_warnings",
			Help: "The total number of file warnings during a backup",
		}, commonDims),
		tasksDuration: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "backrest_tasks_duration_secs",
			Help: "The duration of a task in seconds",
		}, append(slices.Clone(commonDims), "task_type")),
		tasksRun: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "backrest_tasks_run_total",
			Help: "The total number of tasks run",
		}, append(slices.Clone(commonDims), "task_type", "status")),
		lastTaskStatus: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "backrest_last_task_status",
			Help: "The status of the last task",
		}, append(slices.Clone(commonDims), "task_type", "status")),
	}

	registry.peerConnected = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "backrest_peer_connected",
		Help: "1 if the multihost peer currently has a sync session open with this instance, 0 otherwise",
	}, []string{"instance_id"})
	registry.peerLastHeartbeat = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "backrest_peer_last_heartbeat_timestamp_seconds",
		Help: "Unix time of the last heartbeat received from the multihost peer",
	}, []string{"instance_id"})
	registry.remoteLastOperation = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "backrest_remote_last_operation_timestamp_seconds",
		Help: "Unix time at which the latest finished operation of this type synced from a multihost peer ended",
	}, []string{"instance_id", "repo_id", "plan_id", "op_type", "status"})

	registry.reg.MustRegister(registry.peerConnected)
	registry.reg.MustRegister(registry.peerLastHeartbeat)
	registry.reg.MustRegister(registry.remoteLastOperation)
	registry.reg.MustRegister(registry.backupBytesProcessed)
	registry.reg.MustRegister(registry.backupBytesAdded)
	registry.reg.MustRegister(registry.backupFileWarnings)
	registry.reg.MustRegister(registry.tasksDuration)
	registry.reg.MustRegister(registry.tasksRun)
	registry.reg.MustRegister(registry.lastTaskStatus)

	return registry
}

func GetRegistry() *Registry {
	return globalRegistry
}

type Registry struct {
	reg                  *prometheus.Registry
	backupBytesProcessed *prometheus.GaugeVec
	backupBytesAdded     *prometheus.GaugeVec
	backupFileWarnings   *prometheus.GaugeVec
	tasksDuration        *prometheus.GaugeVec
	tasksRun             *prometheus.CounterVec
	lastTaskStatus       *prometheus.GaugeVec

	// Multihost (hub side): state of the peers syncing into this instance.
	peerConnected       *prometheus.GaugeVec
	peerLastHeartbeat   *prometheus.GaugeVec
	remoteLastOperation *prometheus.GaugeVec
}

func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{})
}

func (r *Registry) RecordTaskRun(repoID, planID, taskType string, duration_secs float64, status string) {
	if repoID == "" {
		repoID = "_unassociated_"
	}
	if planID == "" {
		planID = "_unassociated_"
	}
	r.lastTaskStatus.DeletePartialMatch(prometheus.Labels{"repo_id": repoID, "plan_id": planID, "task_type": taskType})
	if status == "success" {
		r.lastTaskStatus.WithLabelValues(repoID, planID, taskType, status).Set(0)
	} else if status == "failed" {
		r.lastTaskStatus.WithLabelValues(repoID, planID, taskType, status).Set(1)
	} else {
		r.lastTaskStatus.WithLabelValues(repoID, planID, taskType, status).Set(-1)
	}
	r.tasksRun.WithLabelValues(repoID, planID, taskType, status).Inc()
	r.tasksDuration.WithLabelValues(repoID, planID, taskType).Set(duration_secs)
}

func (r *Registry) RecordBackupSummary(repoID, planID string, bytesProcessed, bytesAdded int64, fileWarnings int64) {
	r.backupBytesProcessed.WithLabelValues(repoID, planID).Set(float64(bytesProcessed))
	r.backupBytesAdded.WithLabelValues(repoID, planID).Set(float64(bytesAdded))
	r.backupFileWarnings.WithLabelValues(repoID, planID).Set(float64(fileWarnings))
}

// SetPeerConnected records whether a multihost peer currently has an open sync session.
func (r *Registry) SetPeerConnected(instanceID string, connected bool) {
	v := 0.0
	if connected {
		v = 1
	}
	r.peerConnected.WithLabelValues(instanceID).Set(v)
}

// RecordPeerHeartbeat records the time of the latest heartbeat from a multihost peer.
func (r *Registry) RecordPeerHeartbeat(instanceID string, at time.Time) {
	r.peerLastHeartbeat.WithLabelValues(instanceID).Set(float64(at.Unix()))
}

// RecordRemoteOperation records that an operation of opType finished on a multihost peer.
// Only the latest status per (instance, repo, plan, opType) is kept, so alerts can
// use the timestamp to detect peers that stopped producing backups.
func (r *Registry) RecordRemoteOperation(instanceID, repoID, planID, opType, status string, endedAt time.Time) {
	r.remoteLastOperation.DeletePartialMatch(prometheus.Labels{
		"instance_id": instanceID, "repo_id": repoID, "plan_id": planID, "op_type": opType,
	})
	r.remoteLastOperation.WithLabelValues(instanceID, repoID, planID, opType, status).Set(float64(endedAt.Unix()))
}
