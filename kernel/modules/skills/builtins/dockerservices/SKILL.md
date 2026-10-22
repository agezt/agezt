---
name: docker-services
description: Run self-hosted services (Postgres, Redis, MinIO, Ollama, n8n, …) in the background with Docker — start them detached and surviving reboots, list and inspect them, read their logs, and tear them down, when a task needs a real running service instead of a one-shot script
triggers: [docker, service, postgres, redis, database, container, self-host, background, daemon, minio, ollama, compose]
tools: [shell, code_exec]
---

# Docker services — stand up real background services

When a task needs a *running* service — a database to hold state across runs, a
cache, an object store, a local model server, a webhook receiver — don't fake it
with a throwaway script. Run it as a Docker container, detached, so it keeps
serving after this run ends and across daemon/host reboots. You have full machine
capability (shell, code_exec); this skill is the lifecycle discipline that keeps
those services discoverable and reapable instead of becoming orphaned junk.

## The one rule: label everything

Every service you start MUST carry the label `agezt.service=1` (the helper does
this for you). That label is how you — and the operator, and any cleanup pass —
find *only* the services agezt agents started, without touching the user's own
containers. Never start an agezt service without it.

## One-time check

```sh
docker version            # is the daemon up and reachable?
```

If Docker isn't installed, install it via the computer-use skill (the OS package
manager), then re-check. On Linux the daemon may need `sudo systemctl start docker`.

## Lifecycle with the helper

`scripts/svc.sh` wraps the common operations and enforces the label + naming
convention. Run it via shell or code_exec. Use `skill op=files docker-services`
to find the bundle directory.

```sh
# Start (idempotent — reuses a healthy container of the same name):
scripts/svc.sh up pg postgres:16 -e POSTGRES_PASSWORD=secret -p 5432:5432 \
  -v agezt-pg:/var/lib/postgresql/data

scripts/svc.sh ls               # only agezt services, with status + ports
scripts/svc.sh logs pg 100      # last 100 log lines
scripts/svc.sh ip pg            # the container's address + mapped ports
scripts/svc.sh down pg          # stop + remove the container (named volume kept)
scripts/svc.sh nuke pg          # down AND delete its named volumes (destroys data)
```

`up` adds `--restart unless-stopped` so the service comes back after a reboot,
names the container `agezt-<name>`, and stamps the `agezt.service=1` label. It is
safe to call again: if `agezt-<name>` is already running it is left alone; if it
exists but is stopped it is started; otherwise it is created.

## Doing it by hand

The helper is a convenience, not a cage. Raw `docker run` is fine as long as you
keep the contract:

```sh
docker run -d --name agezt-redis --label agezt.service=1 \
  --restart unless-stopped -p 6379:6379 redis:7
```

For multi-container stacks, write a `docker-compose.yml` and
`docker compose up -d`, but add `labels: ["agezt.service=1"]` to every service so
the cleanup contract still holds.

## Connecting from agent code

Once a service is up, connect over the mapped host port like any client —
e.g. `postgresql://postgres:secret@localhost:5432/postgres` from a pandas/psql
run, or `redis://localhost:6379`. Persist connection details to a memory note or
the Data Lake so later runs reuse the same service instead of starting a second.

## Clean up after yourself

A self-hosted service is durable on purpose, but don't leave dead ones running.
When a task that owned a scratch service is done, `svc.sh down <name>` it. To see
everything ag(ezt) ever started: `docker ps -a --filter label=agezt.service=1`.
See `reference/services.md` for ready-to-run recipes (Postgres, Redis, MinIO,
Ollama, n8n, Meilisearch) with ports, volumes, and connection strings.
