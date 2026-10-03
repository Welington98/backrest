# Hub Monitoring

When several Backrest instances sync into one **hub** (see [Multihost Sync](./multihost.md)),
the hub exposes the state of every peer on its `/metrics` endpoint, so a single Prometheus
scrape is enough to monitor all clients. Peers need the **Read Operations** permission for
the operation metrics.

| Metric | Labels | Meaning |
|---|---|---|
| `backrest_peer_connected` | `instance_id` | `1` while the peer has a sync session open, `0` after it disconnects |
| `backrest_peer_last_heartbeat_timestamp_seconds` | `instance_id` | Unix time of the last heartbeat from the peer |
| `backrest_remote_last_operation_timestamp_seconds` | `instance_id`, `repo_id`, `plan_id`, `op_type`, `status` | Unix time at which the latest finished `backup`, `copy`, `prune`, `check` or `forget` ended. `status` is `success`, `warning` or `failed`. Dry runs, unfinished and cancelled operations are ignored. |

The metrics describe what the hub has *seen*: they are updated when operations are synced
and reset when the hub restarts, until each peer reconnects.

## Example alert rules

```yaml
groups:
  - name: backrest-hub
    rules:
      - alert: BackrestPeerSilent
        expr: time() - backrest_peer_last_heartbeat_timestamp_seconds > 900
        for: 5m
        annotations:
          summary: "No heartbeat from {{ $labels.instance_id }} for 15 minutes"

      - alert: BackrestBackupFailed
        expr: backrest_remote_last_operation_timestamp_seconds{op_type="backup", status="failed"} > 0
        annotations:
          summary: "Last backup of {{ $labels.plan_id }} on {{ $labels.instance_id }} failed"

      - alert: BackrestBackupStale
        expr: time() - backrest_remote_last_operation_timestamp_seconds{op_type="backup"} > 26 * 3600
        annotations:
          summary: "No backup of {{ $labels.plan_id }} on {{ $labels.instance_id }} in 26 hours"
```

Adjust the `BackrestBackupStale` window to the plan's schedule.
