# FlowPulse

Live CPU and flow monitoring for distributed ML training clusters, using eBPF and InfiniBand east-west traffic analysis.

## Architecture

```
┌─────────────────┐     gRPC stream     ┌───────────────┐     batch insert     ┌────────────┐
│  FlowPulse      │ ──────────────────► │  Aggregator   │ ──────────────────► │ ClickHouse │
│  Agent (per-node)│                     │  (sharded)    │                     │            │
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

## Prerequisites

### Build Requirements

- Linux kernel 5.15+ with BTF support (for eBPF)
- Go 1.22+
- clang/llvm (for eBPF compilation)
- bpftool (for vmlinux.h generation)
- Node.js 20+ (for frontend)
- Docker (for container images)

### Kubernetes Deployment Requirements

- Kubernetes 1.27+
- Helm 3.12+
- Nodes with Linux kernel 5.15+ and BTF support
- GPU/training nodes labeled with `flowpulse.io/monitor=true`
- A container registry accessible from the cluster
- (Optional) An ingress controller (nginx, traefik, etc.)

## Quick Start (Local Dev)

```bash
# Start storage dependencies
docker compose up -d

# Run the mock data server (simulates 32 GPU nodes)
node mock-server.mjs &

# Run the frontend
cd web && npm install && npm run dev
```

Open http://localhost:3000 to see the dashboard with simulated data.

## Building

### Go Binaries

```bash
# Generate go.sum (required before first build)
go mod tidy

# Build all binaries
make build
```

### Docker Images

```bash
# Build all images (agent, aggregator, server, web)
make docker

# Build individual images
make docker-agent
make docker-aggregator
make docker-server
make docker-web
```

### Push to Registry

```bash
REGISTRY=your-registry.example.com/flowpulse
VERSION=$(git describe --tags --always)

for component in agent aggregator server web; do
  docker tag flowpulse-${component}:${VERSION} ${REGISTRY}/flowpulse-${component}:${VERSION}
  docker push ${REGISTRY}/flowpulse-${component}:${VERSION}
done
```

## Deployment

### Kubernetes (Helm)

#### 1. Prepare the Namespace

```bash
kubectl create namespace flowpulse
```

#### 2. Label GPU/Training Nodes

The agent DaemonSet only runs on nodes with the `flowpulse.io/monitor=true` label:

```bash
# Label all GPU nodes
kubectl label node gpu-node-001 flowpulse.io/monitor=true
kubectl label node gpu-node-002 flowpulse.io/monitor=true

# Or label all nodes in a node pool
kubectl label nodes -l node-pool=gpu flowpulse.io/monitor=true
```

#### 3. Install with Helm

**Quick start** (in-cluster storage, no ingress):

```bash
helm install flowpulse deploy/kubernetes/helm/flowpulse \
  --namespace flowpulse \
  --set global.imageRegistry=your-registry.example.com/flowpulse
```

**Production** (external storage, ingress, network policies):

```bash
helm install flowpulse deploy/kubernetes/helm/flowpulse \
  --namespace flowpulse \
  --set global.imageRegistry=your-registry.example.com/flowpulse \
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

**With email alerting**:

```bash
helm install flowpulse deploy/kubernetes/helm/flowpulse \
  --namespace flowpulse \
  --set global.imageRegistry=your-registry.example.com/flowpulse \
  --set smtp.enabled=true \
  --set smtp.host=smtp.example.com \
  --set smtp.port=587 \
  --set smtp.user=alerts@example.com \
  --set smtp.pass=your-smtp-password \
  --set smtp.from=flowpulse@example.com \
  --set smtp.recipients="oncall@example.com\,team@example.com"
```

#### 4. Verify the Deployment

```bash
# Check all pods are running
kubectl -n flowpulse get pods

# Check agent is running on labeled nodes
kubectl -n flowpulse get ds flowpulse-agent

# Check aggregator replicas
kubectl -n flowpulse get statefulset flowpulse-aggregator

# Check server health
kubectl -n flowpulse port-forward svc/flowpulse-server 8080:8080
curl http://localhost:8080/healthz

# Access the dashboard
kubectl -n flowpulse port-forward svc/flowpulse-web 3000:3000
# Open http://localhost:3000
```

#### 5. Upgrade

```bash
helm upgrade flowpulse deploy/kubernetes/helm/flowpulse \
  --namespace flowpulse \
  --set global.imageRegistry=your-registry.example.com/flowpulse \
  --set agent.image.tag=v0.2.0 \
  --set aggregator.image.tag=v0.2.0 \
  --set server.image.tag=v0.2.0 \
  --set web.image.tag=v0.2.0
```

### Bare-metal (systemd)

```bash
# Build binaries
make build

# Install (copies binaries, configs, systemd units)
sudo ./deploy/systemd/install.sh

# Configure storage endpoints
sudo tee /etc/flowpulse/env << 'EOF'
FLOWPULSE_CLICKHOUSE_DSN=clickhouse://clickhouse-host:9000/flowpulse
FLOWPULSE_NATS_URL=nats://nats-host:4222
FLOWPULSE_REDIS_ADDR=redis-host:6379
FLOWPULSE_JWT_SECRET=$(openssl rand -hex 32)
EOF

# Start services
sudo systemctl enable --now flowpulse-agent
sudo systemctl enable --now flowpulse-aggregator
```

## Configuration Reference

### `values.yaml` Key Settings

| Parameter | Description | Default |
|-----------|-------------|---------|
| `global.imageRegistry` | Container registry prefix | `""` |
| `imagePullSecrets` | Registry auth secrets | `[]` |
| `createNamespace` | Create the namespace | `false` |
| **Agent** | | |
| `agent.nodeSelector` | Node selector for DaemonSet | `flowpulse.io/monitor: "true"` |
| `agent.pollInterval` | eBPF map poll interval | `1s` |
| `agent.flowTimeout` | Flow inactivity timeout | `30s` |
| `agent.maxFlows` | Max tracked flows per node | `1000000` |
| **Aggregator** | | |
| `aggregator.replicas` | Number of aggregator shards | `3` |
| `aggregator.batchSize` | ClickHouse batch insert size | `10000` |
| `aggregator.flushInterval` | Batch flush interval | `5s` |
| **Server** | | |
| `server.replicas` | API server replicas | `2` |
| **Web** | | |
| `web.replicas` | Frontend replicas | `2` |
| **Storage** | | |
| `clickhouse.enabled` | Deploy in-cluster ClickHouse | `true` |
| `clickhouse.dsn` | External ClickHouse DSN | `""` |
| `clickhouse.storage.size` | PVC size | `50Gi` |
| `nats.enabled` | Deploy in-cluster NATS | `true` |
| `nats.url` | External NATS URL | `""` |
| `redis.enabled` | Deploy in-cluster Redis | `true` |
| `redis.addr` | External Redis address | `""` |
| **Ingress** | | |
| `ingress.enabled` | Enable Ingress resource | `false` |
| `ingress.className` | Ingress class | `""` |
| `ingress.host` | Ingress hostname | `flowpulse.example.com` |
| `ingress.tls` | TLS configuration | `[]` |
| **Security** | | |
| `auth.jwtSecret` | JWT signing secret | auto-generated |
| `networkPolicy.enabled` | Enable NetworkPolicies | `false` |
| **Alerts** | | |
| `smtp.enabled` | Enable email alerts | `false` |
| `smtp.host` | SMTP server host | `""` |
| `smtp.port` | SMTP server port | `587` |
| `smtp.recipients` | Comma-separated email list | `""` |

## Component Architecture

| Component | K8s Resource | Ports | Privileges |
|-----------|-------------|-------|------------|
| **Agent** | DaemonSet | 8081 (health) | Privileged (BPF, NET_ADMIN) |
| **Aggregator** | StatefulSet | 9091 (gRPC) | Non-root (65534) |
| **Server** | Deployment | 8080 (REST), 9090 (gRPC) | Non-root (65534) |
| **Web** | Deployment | 3000 (HTTP) | Non-root (1001) |
| **ClickHouse** | StatefulSet | 8123, 9000 | Non-root (101) |
| **NATS** | StatefulSet | 4222, 8222 | Non-root (1000) |
| **Redis** | Deployment | 6379 | Non-root (999) |

## Security Notes

- The agent requires privileged access for eBPF program loading. It needs `CAP_BPF`, `CAP_PERFMON`, `CAP_NET_ADMIN`, and `CAP_SYS_ADMIN`.
- All other components run as non-root with `readOnlyRootFilesystem` and all capabilities dropped.
- Secrets (ClickHouse DSN, NATS URL, JWT secret, SMTP credentials) are stored in Kubernetes Secrets, never in ConfigMaps or environment variable literals.
- Network policies (when enabled) enforce default-deny and explicitly allow only required component-to-component traffic.
- The JWT secret is auto-generated at install time if not provided, but should be explicitly set for production.

## License

Apache-2.0
