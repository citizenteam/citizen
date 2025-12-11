# Citizen Bootstrap Agent

Lightweight HTTP service that finalises a Citizen instance installation on a remote server.

## Features
- Authenticated `/bootstrap/init` endpoint writes provided bundle files (compose, env, etc.) under a configurable data directory.
- Executes `docker compose` commands (`pull` + `up -d --remove-orphans` by default).
- `/status` reports the last bootstrap job, `/logs` exposes a tail of the agent log, `/healthz` for readiness checks.
- Configuration via environment variables (see below). Defaults target `/opt/citizen`.

## Configuration
| Variable | Default | Description |
|---|---|---|
| `BOOTSTRAP_BIND` | `0.0.0.0:8085` | Listen address |
| `BOOTSTRAP_DATA_DIR` | `/opt/citizen/data` | Base directory to write files |
| `BOOTSTRAP_COMPOSE_PATH` | `docker/docker-compose.yml` | Default compose path (relative or absolute) |
| `BOOTSTRAP_ENV_PATH` | `docker/.env` | Default env path |
| `BOOTSTRAP_LOG_PATH` | `<dataDir>/citizen-bootstrap.log` | Log file location |
| `BOOTSTRAP_SHARED_SECRET` | — | Required token; requests must send `X-Provision-Token` header |
| `BOOTSTRAP_RUN_PULL` | `true` | Toggle `docker compose pull` before `up` |

## Example Request
```json
POST /bootstrap/init
Headers:
  X-Provision-Token: <shared-secret>

{
  "job_id": "job-123",
  "compose_path": "docker/docker-compose.yml",
  "files": [
    {
      "path": "docker/docker-compose.yml",
      "contents": "...yaml..."
    },
    {
      "path": "docker/.env",
      "contents": "ENV=prod\n"
    }
  ],
  "docker_args": ["up", "-d"]
}
```

## Running in Docker
The helper script `scripts/run-container.sh` starts the agent container using the published image (ensure `/var/run/docker.sock` is mounted so the agent can call `docker compose`):

```bash
export BOOTSTRAP_AGENT_SHARED_SECRET="super-secret"
citizen/bootstrap-agent/scripts/run-container.sh
```

Ports and data directories are configurable with environment variables (`BOOTSTRAP_AGENT_PORT`, `BOOTSTRAP_AGENT_DATA_DIR`, etc.).

## Building
```bash
cd citizen/bootstrap-agent
go build ./...
```

## Future Work
- Stream bootstrap logs back to CitizenAuth.
- Systemd unit generator & installer script.
- Agent-side TLS and IP allow-list enforcement.
*** End Patch
