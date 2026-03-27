# FlowPulse

**eBPF-powered real-time observability for distributed ML training clusters.**

Monitor network flows, CPU scheduling, InfiniBand traffic, and training performance across your GPU fleet — with zero application instrumentation.

```
┌─────────────────┐     HTTP stream      ┌───────────────┐     batch insert     ┌────────────┐
│  FlowPulse      │ ──────────────────► │  Aggregator   │ ──────────────────► │ ClickHouse │
│  Agent (per-node)│                     │  (correlator) │                     │            │
│  eBPF + IB      │                     │               │ ──── publish ─────► │ NATS       │
└─────────────────┘                     └───────────────┘                     └────────────┘
                                                                                    │
                                              ┌─────────────┐    subscribe          │
                                              │ API Server  │ ◄────────────────────┘
                                              │ REST + WS   │
                                              └──────┬──────┘
                                                     │
                                              ┌──────▼──────┐
                                              │  React/Next │
                                              │  Dashboard  │
                                              └─────────────┘
```

## Key Metrics

- **Network**: per-flow bandwidth, RDMA message rate, QP congestion (ECN/CNP), retransmissions, port errors
- **CPU**: per-core utilization (NUMA-aware), kernel vs user ratio, context switches, softirq time
- **Training**: straggler score, bubble ratio, gradient sync overhead, network saturation index

---

## Installation

### Option 1: One-Command Docker Install (Recommended)

The fastest way to run FlowPulse. Requires only Docker.

```bash
curl -sSL https://raw.githubusercontent.com/KAPS3000/flowpulse.ai/main/install.sh | bash
```

This will:
1. Clone the repository to `~/flowpulse`
2. Generate a `.env` file with secure defaults
3. Build and start all containers (ClickHouse, NATS, Redis, Aggregator, Server, Web)
4. Print the dashboard URL

**After install:**
- Dashboard: http://localhost:3000
- API: http://localhost:8080/healthz

### Option 2: All-in-One Docker Container

Run the entire FlowPulse stack in a single container — ideal for demos and evaluation.

```bash
# Build the all-in-one image
docker build -f docker/Dockerfile.allinone -t flowpulse .

# Run it
docker run -d --name flowpulse \
  -p 3000:3000 \
  -p 8080:8080 \
  -v flowpulse_data:/var/lib/clickhouse \
  flowpulse

# Open the dashboard
open http://localhost:3000
```

Or with `make`:

```bash
make docker-allinone
docker run -d --name flowpulse -p 3000:3000 -p 8080:8080 flowpulse
```

**What's inside:** ClickHouse, NATS, Redis, Aggregator, API Server, and the Next.js Dashboard — all managed by supervisord.

### Option 3: Docker Compose (Multi-Container)

For development or customized deployments with individual service control.

```bash
git clone https://github.com/KAPS3000/flowpulse.ai.git
cd flowpulse.ai

# Start the full stack
docker compose -f docker-compose.quickstart.yml up -d

# Include simulated data for demo
docker compose -f docker-compose.quickstart.yml --profile demo up -d
```

**Customize ports and settings** by creating a `.env` file:

```bash
cat > .env <<EOF
WEB_PORT=3000
API_PORT=8080
JWT_SECRET=$(openssl rand -hex 32)
EOF
```

**Manage the stack:**

```bash
# View logs
docker compose -f docker-compose.quickstart.yml logs -f

# View logs for a specific service
docker compose -f docker-compose.quickstart.yml logs -f server

# Restart a service
docker compose -f docker-compose.quickstart.yml restart aggregator

# Stop everything
docker compose -f docker-compose.quickstart.yml down

# Stop and delete all data
docker compose -f docker-compose.quickstart.yml down -v
```

### Option 4: Kubernetes (Helm)

For production deployment on GPU clusters.

#### Prerequisites

- Kubernetes 1.27+
- Helm 3.12+
- Nodes with Linux kernel 5.15+ and BTF support
- A container registry accessible from the cluster

#### Step 1: Build and Push Images

```bash
REGISTRY=your-registry.example.com/flowpulse
VERSION=$(git describe --tags --always)

make docker

for component in agent aggregator server web; do
  docker tag flowpulse-${component}:${VERSION} ${REGISTRY}/flowpulse-${component}:${VERSION}
  docker push ${REGISTRY}/flowpulse-${component}:${VERSION}
done
```

#### Step 2: Label GPU Nodes

```bash
# Label nodes that should run the eBPF agent
kubectl label node gpu-node-001 flowpulse.io/monitor=true
kubectl label node gpu-node-002 flowpulse.io/monitor=true

# Or label an entire node pool
kubectl label nodes -l node-pool=gpu flowpulse.io/monitor=true
```

#### Step 3: Install

**Quick start** (in-cluster storage, no ingress):

```bash
helm install flowpulse deploy/kubernetes/helm/flowpulse \
  --namespace flowpulse --create-namespace \
  --set global.imageRegistry=${REGISTRY}
```

**Production** (external storage, ingress, network policies):

```bash
helm install flowpulse deploy/kubernetes/helm/flowpulse \
  --namespace flowpulse --create-namespace \
  --set global.imageRegistry=${REGISTRY} \
  --set agent.image.tag=v0.1.0 \
  --set aggregator.image.tag=v0.1.0 \
  --set server.image.tag=v0.1.0 \
  --set web.image.tag=v0.1.0 \
  --set clickhouse.enabled=false \
  --set clickhouse.dsn="clickhouse://clickhouse.infra:9000/flowpulse" \
  --set nats.enabled=false \
  --set nats.url="nats://nats.infra:4222" \
  --set redis.enabled=false \
  --set redis.addr="redis.infra:6379" \
  --set ingress.enabled=true \
  --set ingress.host=flowpulse.your-domain.com \
  --set ingress.className=nginx \
  --set networkPolicy.enabled=true
```

#### Step 4: Verify

```bash
# All pods running
kubectl -n flowpulse get pods

# Agent running on labeled nodes
kubectl -n flowpulse get ds flowpulse-agent

# Access the dashboard
kubectl -n flowpulse port-forward svc/flowpulse-web 3000:3000
open http://localhost:3000
```

#### Step 5: Upgrade

```bash
helm upgrade flowpulse deploy/kubernetes/helm/flowpulse \
  --namespace flowpulse \
  --set global.imageRegistry=${REGISTRY} \
  --set agent.image.tag=v0.2.0
```

### Option 5: Bare Metal (systemd)

```bash
# Build Go binaries
make build

# Install binaries, configs, and systemd units
sudo ./deploy/systemd/install.sh

# Configure storage endpoints
sudo tee /etc/flowpulse/env <<'EOF'
FLOWPULSE_CLICKHOUSE_DSN=clickhouse://clickhouse-host:9000/flowpulse
FLOWPULSE_NATS_URL=nats://nats-host:4222
FLOWPULSE_REDIS_ADDR=redis-host:6379
FLOWPULSE_JWT_SECRET=$(openssl rand -hex 32)
EOF

# Start services
sudo systemctl enable --now flowpulse-agent
sudo systemctl enable --now flowpulse-aggregator
```

---

## Build Requirements

| Requirement | Version | Purpose |
|------------|---------|---------|
| Docker | 20.10+ | Container builds and local dev |
| Go | 1.22+ | Backend binaries |
| Node.js | 20+ | Frontend dashboard |
| clang/llvm | 14+ | eBPF program compilation |
| Linux kernel | 5.15+ with BTF | eBPF agent runtime |

### Building from Source

```bash
# Install Go dependencies
go mod tidy

# Build all binaries
make build

# Build all Docker images
make docker

# Build the all-in-one image
make docker-allinone

# Build individual images
make docker-agent
make docker-aggregator
make docker-server
make docker-web
```

### Running the Frontend (Development)

```bash
cd web
npm install
npm run dev
# Opens at http://localhost:3000
```

To run with mock data (simulates a 32-node GPU cluster):

```bash
# Terminal 1: Start storage
docker compose up -d

# Terminal 2: Run mock data server
node mock-server.mjs

# Terminal 3: Run frontend
cd web && npm run dev
```

---

## Configuration Reference

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `FLOWPULSE_CLICKHOUSE_DSN` | ClickHouse connection string | `clickhouse://localhost:9000` |
| `FLOWPULSE_NATS_URL` | NATS server URL | `nats://localhost:4222` |
| `FLOWPULSE_REDIS_ADDR` | Redis server address | `localhost:6379` |
| `FLOWPULSE_JWT_SECRET` | JWT signing secret | auto-generated |
| `NEXT_PUBLIC_API_URL` | API server URL (for frontend) | `http://localhost:8080` |
| `NEXT_PUBLIC_WS_URL` | WebSocket URL (for frontend) | `ws://localhost:8080/ws` |

### Docker Compose `.env` Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `FLOWPULSE_VERSION` | Image version tag | `latest` |
| `WEB_PORT` | Dashboard port | `3000` |
| `API_PORT` | API server port | `8080` |
| `AGGREGATOR_HTTP_PORT` | Aggregator ingest port | `9092` |
| `CLICKHOUSE_HTTP_PORT` | ClickHouse HTTP port | `8123` |
| `CLICKHOUSE_NATIVE_PORT` | ClickHouse native port | `9000` |
| `NATS_PORT` | NATS client port | `4222` |
| `REDIS_PORT` | Redis port | `6379` |
| `JWT_SECRET` | JWT signing secret | auto-generated |

### Helm `values.yaml` Key Settings

| Parameter | Description | Default |
|-----------|-------------|---------|
| `global.imageRegistry` | Container registry prefix | `""` |
| `agent.nodeSelector` | Nodes to monitor | `flowpulse.io/monitor: "true"` |
| `agent.pollInterval` | eBPF map poll interval | `1s` |
| `agent.maxFlows` | Max tracked flows per node | `1000000` |
| `aggregator.replicas` | Aggregator shards | `3` |
| `aggregator.batchSize` | ClickHouse batch size | `10000` |
| `aggregator.flushInterval` | Batch flush interval | `5s` |
| `server.replicas` | API server replicas | `2` |
| `web.replicas` | Frontend replicas | `2` |
| `clickhouse.enabled` | Deploy in-cluster ClickHouse | `true` |
| `clickhouse.dsn` | External ClickHouse DSN | `""` |
| `nats.enabled` | Deploy in-cluster NATS | `true` |
| `nats.url` | External NATS URL | `""` |
| `redis.enabled` | Deploy in-cluster Redis | `true` |
| `ingress.enabled` | Enable Ingress resource | `false` |
| `ingress.host` | Ingress hostname | `flowpulse.example.com` |
| `networkPolicy.enabled` | Enable NetworkPolicies | `false` |
| `smtp.enabled` | Enable email alerts | `false` |

---

## Component Architecture

| Component | K8s Resource | Ports | Privileges |
|-----------|-------------|-------|------------|
| **Agent** | DaemonSet | 8081 (health) | Privileged (BPF, NET_ADMIN) |
| **Aggregator** | StatefulSet | 9091 (gRPC), 9092 (HTTP) | Non-root (65534) |
| **Server** | Deployment | 8080 (REST/WS) | Non-root (65534) |
| **Web** | Deployment | 3000 (HTTP) | Non-root (1001) |
| **ClickHouse** | StatefulSet | 8123, 9000 | Non-root (101) |
| **NATS** | StatefulSet | 4222, 8222 | Non-root (1000) |
| **Redis** | Deployment | 6379 | Non-root (999) |

## Security Notes

- The eBPF agent requires privileged access (`CAP_BPF`, `CAP_PERFMON`, `CAP_NET_ADMIN`, `CAP_SYS_ADMIN`) for kernel instrumentation.
- All other components run as non-root with `readOnlyRootFilesystem` and all capabilities dropped.
- Secrets are stored in Kubernetes Secrets, never in ConfigMaps or environment variable literals.
- Network policies (when enabled) enforce default-deny and explicitly allow only required traffic.
- JWT secrets are auto-generated at install time but should be explicitly set for production.

## Troubleshooting

### Common Issues

**Dashboard shows "Connection failed"**
- Ensure the API server is running: `curl http://localhost:8080/healthz`
- Check that `NEXT_PUBLIC_API_URL` points to the correct API host and port

**No data appearing in dashboard**
- If using Docker Compose, start with the demo profile: `--profile demo`
- Check aggregator logs: `docker compose logs aggregator`
- Verify ClickHouse is healthy: `curl http://localhost:8123/ping`

**Port conflicts**
- Edit `.env` to change port mappings (see Configuration Reference above)
- Or pass environment variables: `WEB_PORT=3001 docker compose -f docker-compose.quickstart.yml up -d`

**ClickHouse out of disk**
- Data has TTL-based retention (flows: 30 days, node_metrics: 7 days, training_metrics: 30 days)
- To purge immediately: `docker compose exec clickhouse clickhouse-client --query "OPTIMIZE TABLE flowpulse.flows FINAL"`

### Viewing Logs

```bash
# All services (Docker Compose)
docker compose -f docker-compose.quickstart.yml logs -f

# Specific service
docker compose -f docker-compose.quickstart.yml logs -f aggregator

# All-in-one container
docker logs flowpulse
docker exec flowpulse cat /var/log/supervisor/aggregator.log

# Kubernetes
kubectl -n flowpulse logs -f deployment/flowpulse-server
kubectl -n flowpulse logs -f daemonset/flowpulse-agent
```

## Documentation

- [PRD & System Design](docs/PRD-and-Functional-Spec.md) — comprehensive product requirements, system architecture, data model, API spec, and capacity planning

## License

Apache-2.0
