# Citizen Instance on K3s

This directory contains a baseline manifest for running a single Citizen stack (Traefik + platform API + Postgres/Redis + watcher + future builder) on a K3s node. It mirrors the services defined in `docker/docker-compose.prod.yml`, but expresses them as Kubernetes objects so that customer servers can run the platform natively on K3s (or any CNCF-compatible distro).

## Prerequisites

1. **K3s install**
   ```bash
   curl -sfL https://get.k3s.io | sh -
   sudo k3s kubectl get nodes
   ```
   For multi-node setups (optional builders/workers), install `k3s agent` on additional nodes pointing to the same server.

2. **kubectl context**
   ```bash
   sudo k3s kubectl config view --raw > ~/.kube/citizen-k3s.yaml
   export KUBECONFIG=~/.kube/citizen-k3s.yaml
   ```

3. **Container Registry**
   Push the Citizen API image (and watcher/builder images) to a registry accessible by the cluster, e.g. `ghcr.io/<org>/citizen-api:tag`. Update the manifest image references accordingly.

4. **Secrets**
   - `dotenv` secret stores sensitive env vars (DB, Redis, CF token, encryption key, etc.).

   Create it before applying the manifest:
   ```bash
   kubectl -n citizen-system create secret generic dotenv \
     --from-literal=CF_DNS_API_TOKEN=... \
     --from-literal=LETSENCRYPT_EMAIL=ops@example.com \
     --from-literal=DB_NAME=citizen \
     --from-literal=DB_USER=citizen \
     --from-literal=DB_PASSWORD=... \
     --from-literal=REDIS_PASSWORD=... \
     --from-literal=ENCRYPTION_KEY=...
   ```

## Deploy

1. Edit `citizen-system.yaml`:
   - Paste the contents of `docker/config/traefik.yml` into the `traefik-static` ConfigMap.
   - Update image references (`citizen-platform-api`, `citizen-traefik-watcher`, `citizen-builder-controller`) to the correct registry tags.
   - Adjust storage requests if needed.

2. Apply:
   ```bash
   kubectl apply -f citizen/kubernetes/citizen-system.yaml
   ```

3. Verify:
   ```bash
   kubectl -n citizen-system get pods
   kubectl -n citizen-system logs deploy/citizen-platform-api
   kubectl -n citizen-system get svc citizen-traefik
   ```

4. Update DNS so that `citizen-traefik` Service external IP (or LoadBalancer hostname) points to your wildcard/domain records. The Traefik watcher continues to mutate `dynamic_conf.yml` (stored on a PVC) just like in the Docker setup.

## Files

- `citizen-system.yaml`: Namespace, Secrets, ConfigMaps, PVCs, Deployments/StatefulSets/Services for Traefik, platform API, Postgres, Redis, watcher, and the placeholder builder controller.

Feel free to split the monolithic manifest into smaller files or convert it into a Helm chart for production use. This baseline ensures the same components that previously lived in Docker Compose now have first-class Kubernetes equivalents.
