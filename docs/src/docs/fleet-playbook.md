# Fleet Playbook: many clients, one hub

A reference setup for a service provider backing up many customer sites (Linux hosts and
Proxmox hosts) with a central place to watch and restore them.

The architecture diagram is kept as Mermaid source in
[`diagrams/fleet-architecture.mmd`](https://github.com/Welington98/backrest/blob/main/docs/src/docs/diagrams/fleet-architecture.mmd).
Open it in any Mermaid renderer (for example [mermaid.live](https://mermaid.live)) or render a PNG locally:

```sh
cd docs/src/docs/diagrams
npx -y @mermaid-js/mermaid-cli -i fleet-architecture.mmd -o fleet-architecture.png \
  -c mermaid-config.json --scale 2 -b white
```

What it shows: customer sites (Linux and Proxmox hosts, with Proxmox Backup Server for the VMs),
cloud storage with separate client and admin keys, the Backrest hub, the secrets store and the
monitoring stack. Green boxes exist in the fork (or in upstream Backrest), blue boxes are planned
or still to be configured, and the items marked *a validar* are the open questions listed in
[section 2](#_2-hub-maintenance-and-monitoring).

> The repository stores `docs/**/*.png` in Git LFS; the rendered PNG is not committed.

## 1. Client: write-only

Each client runs Backrest with a **plan** that backs up to a local repo and a
**copy policy** (plan → *Copy* section) that replicates each new snapshot to the cloud repo.

- The cloud repo's credentials on the client must **not** allow deleting objects
  (for example an "upload only" or "read and write without delete" key restricted to that
  customer's bucket). A compromised client then cannot erase its own cloud history.
- Enable **Disable scheduled maintenance** on the cloud repo (and on the local repo if its
  retention is handled elsewhere). The client then never schedules forget, prune or check
  for it, which would fail without delete permission.
- Keep passwords and keys out of `config.json` (see [Secrets](#_3-secrets)).

## 2. Hub: maintenance and monitoring

- Pair each client to the hub ([Multihost Sync](./multihost.md)) with a token that grants
  only **Read Operations**, scoped to that client's repos/plans.
- On the hub, add the customer's cloud repo with an **admin** key and a *forget* policy
  and *prune/check* schedules. Only the hub deletes.
- Scrape the hub's `/metrics` and alert as described in [Hub Monitoring](./hub-monitoring.md).

::: warning Validate in a pilot
These points depend on behavior that has not been verified end to end:
1. `restic backup` and `restic copy` finish successfully with a key that cannot delete
   objects (restic removes its own lock files at the end of a run; leftover locks expire
   by themselves).
2. The hub and a client can both reference the same cloud repo (same GUID) without the
   sync layer treating them as conflicting.
3. Whether a scoped user on the hub can be restricted to one customer. Backrest has one
   login system; if you need per-customer access for technicians, put an authenticating
   reverse proxy in front or run one hub per team until this is confirmed.
:::

## 3. Secrets

Backrest accepts passwords and credentials through the repo's environment, so no secret
has to live in `config.json`:

```json
{
  "id": "cloud",
  "uri": "s3:https://s3.example.com/customer-a",
  "env": [
    "RESTIC_PASSWORD_FILE=/run/backrest/restic-password",
    "AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID}",
    "AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY}"
  ]
}
```

`${VAR}` is expanded from the Backrest process environment. A secrets agent such as
[Vault Agent](https://developer.hashicorp.com/vault/docs/agent-and-proxy/agent) can render
`/run/backrest/restic-password` and a systemd `EnvironmentFile` for the service from a
Vault KV path per customer (e.g. `backup/<customer>/agent`). Store the hub's admin keys
under a separate path that only the hub's identity can read.

- A repo password from `RESTIC_PASSWORD_FILE`/`_COMMAND` also works as the source of a
  copy: the copy task passes it to restic as `RESTIC_FROM_PASSWORD_FILE`/`_COMMAND`.
- Back up the secrets store itself and keep an offline copy of each repo password. Losing a
  repo password means losing that customer's backups.

## 4. Proxmox

Backrest backs up files, not VM disks. For virtual machines use Proxmox Backup Server
(incremental, deduplicated, consistent snapshots). Use Backrest on the Proxmox host for
`/etc/pve` and other host files, and inside guests for file-level data.

## 5. Pilot checklist

1. One hub and two clients in test VMs; pair them; verify operations appear on the hub.
2. Configure a plan with a copy policy and confirm snapshots arrive in the cloud repo.
3. With the upload-only key: backup and copy succeed; `forget`/`prune` from the client are
   refused; the hub (admin key) can prune; a restore from the hub works.
4. Stop a client: `BackrestPeerSilent` fires. Break a plan: `BackrestBackupFailed` fires.
5. Rotate a secret in the secrets store and confirm the client picks it up without editing
   `config.json`.
