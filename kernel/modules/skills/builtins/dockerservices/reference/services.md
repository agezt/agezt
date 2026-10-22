# docker-services recipes

Ready-to-run services. Each starts detached, labelled `agezt.service=1`, with a
named volume so data survives `down`/restart (use `svc.sh nuke` to wipe). Adjust
passwords/ports as the task needs. After `up`, connect over the mapped host port.

## PostgreSQL

```sh
scripts/svc.sh up pg postgres:16 \
  -e POSTGRES_PASSWORD=secret -p 5432:5432 -v agezt-pg:/var/lib/postgresql/data
```
Connect: `postgresql://postgres:secret@localhost:5432/postgres`

## Redis

```sh
scripts/svc.sh up redis redis:7 -p 6379:6379 -v agezt-redis:/data \
  redis-server --appendonly yes
```
Connect: `redis://localhost:6379`

## MinIO (S3-compatible object store)

```sh
scripts/svc.sh up minio minio/minio \
  -e MINIO_ROOT_USER=admin -e MINIO_ROOT_PASSWORD=secret123 \
  -p 9000:9000 -p 9001:9001 -v agezt-minio:/data \
  server /data --console-address ":9001"
```
S3 endpoint `http://localhost:9000`, console `http://localhost:9001`.

## Ollama (local LLM server)

```sh
scripts/svc.sh up ollama ollama/ollama -p 11434:11434 -v agezt-ollama:/root/.ollama
# then pull a model into the running container:
docker exec agezt-ollama ollama pull llama3.2
```
API: `http://localhost:11434` (OpenAI-compatible at `/v1`).

## Meilisearch (full-text search)

```sh
scripts/svc.sh up meili getmeili/meilisearch:v1.6 \
  -e MEILI_MASTER_KEY=masterKey -p 7700:7700 -v agezt-meili:/meili_data
```
API: `http://localhost:7700`

## n8n (workflow automation)

```sh
scripts/svc.sh up n8n n8nio/n8n -p 5678:5678 -v agezt-n8n:/home/node/.n8n
```
UI: `http://localhost:5678`

## Tips

- **Reuse, don't duplicate.** Before `up`, run `scripts/svc.sh ls` (or
  `docker ps --filter label=agezt.service=1`) — `up` is idempotent on
  `agezt-<name>`, so the same name = the same service.
- **Health.** Some services take a few seconds. Poll readiness, e.g.
  `until docker exec agezt-pg pg_isready -U postgres; do sleep 1; done`.
- **Persist the connection.** Save the URL to a memory note or a Data Lake
  `services` collection so later runs (and other agents) find it.
- **Multi-container stacks** → write a `docker-compose.yml`, add
  `labels: ["agezt.service=1"]` to every service, then `docker compose up -d`.
