import http from 'node:http';
import { WebSocketServer } from 'ws';
import crypto from 'node:crypto';

// ── Cluster simulation parameters ────────────────────────────
const NUM_NODES = 32;
const NUM_GPUS_PER_NODE = 8;
const IB_PEAK_GBPS = 400;
const TICK_MS = 1000;
const STRAGGLER_NODES = new Set([3, 12, 27]);

// ── State ────────────────────────────────────────────────────
const nodes = [];
const flows = [];
const alerts = [];
const alertHistory = [];
let trainingStep = 0;

function ip(a, b, c, d) { return ((a << 24) | (b << 16) | (c << 8) | d) >>> 0; }
function randBetween(lo, hi) { return lo + Math.random() * (hi - lo); }
function pick(arr) { return arr[Math.floor(Math.random() * arr.length)]; }

for (let i = 0; i < NUM_NODES; i++) {
  nodes.push({
    node_id: `gpu-node-${String(i).padStart(3, '0')}`,
    ip_base: ip(10, 0, i, 1),
    numa_nodes: 2,
    cores: 64,
    is_straggler: STRAGGLER_NODES.has(i),
  });
}

// ── Alert rules engine ───────────────────────────────────────
const ALERT_RULES = [
  {
    id: 'straggler-critical',
    name: 'Critical Straggler Detected',
    description: 'A node deviates >40% from median collective completion time, severely impacting training throughput',
    severity: 'critical',
    category: 'training',
    metric: 'straggler_score',
    condition: 'gt',
    threshold: 40,
    cooldown_sec: 60,
    runbook: 'Check node for hardware issues (IB link errors, GPU thermals). Consider draining and replacing the node. Run `ibstat` and `nvidia-smi` on the affected node.',
    enabled: true,
  },
  {
    id: 'straggler-warning',
    name: 'Straggler Warning',
    description: 'A node deviates >20% from median, causing training slowdown',
    severity: 'warning',
    category: 'training',
    metric: 'straggler_score',
    condition: 'gt',
    threshold: 20,
    cooldown_sec: 120,
    runbook: 'Monitor the node over the next few minutes. If persistent, check for competing workloads or NUMA misalignment.',
    enabled: true,
  },
  {
    id: 'bubble-ratio-high',
    name: 'High Bubble Ratio',
    description: 'GPU idle time from network/CPU waits exceeds 20%, reducing training efficiency',
    severity: 'warning',
    category: 'training',
    metric: 'bubble_ratio',
    condition: 'gt',
    threshold: 20,
    cooldown_sec: 90,
    runbook: 'Check gradient sync overhead. Consider adjusting NCCL_ALGO, NCCL_PROTO, or bucket sizes. Verify no background jobs are stealing CPU cycles.',
    enabled: true,
  },
  {
    id: 'gradient-sync-critical',
    name: 'Gradient Sync Bottleneck',
    description: 'Collective operations consume >30% of step time, indicating a network or configuration issue',
    severity: 'critical',
    category: 'network',
    metric: 'gradient_sync_overhead_pct',
    condition: 'gt',
    threshold: 30,
    cooldown_sec: 60,
    runbook: 'Profile NCCL with NCCL_DEBUG=INFO. Check for IB port errors (`perfquery`), ECN storm, or PFC deadlocks. Verify DCQCN/ECN configuration on switches.',
    enabled: true,
  },
  {
    id: 'network-saturation-low',
    name: 'Low Network Utilization',
    description: 'Average IB utilization is below 40%, suggesting underutilized interconnect or compute-bound phase',
    severity: 'info',
    category: 'network',
    metric: 'network_saturation_index',
    condition: 'lt',
    threshold: 40,
    cooldown_sec: 300,
    runbook: 'If unexpected, check that all GPUs are active and data pipeline is not starved. Verify NCCL is using all IB rails.',
    enabled: true,
  },
  {
    id: 'network-saturation-critical',
    name: 'Network Saturated',
    description: 'Average IB utilization exceeds 90%, risking congestion and increased latency',
    severity: 'critical',
    category: 'network',
    metric: 'network_saturation_index',
    condition: 'gt',
    threshold: 90,
    cooldown_sec: 60,
    runbook: 'Check for traffic imbalance across rails. Verify adaptive routing is enabled on switches. Consider gradient compression or reducing batch size.',
    enabled: true,
  },
  {
    id: 'imbalance-high',
    name: 'Traffic Imbalance Detected',
    description: 'Significant asymmetry in east-west traffic across InfiniBand rails',
    severity: 'warning',
    category: 'network',
    metric: 'imbalance_score',
    condition: 'gt',
    threshold: 25,
    cooldown_sec: 120,
    runbook: 'Check IB port mapping and NCCL topology file. Verify SHARP or adaptive routing config. Run `ibdiagnet` to check for link-level issues.',
    enabled: true,
  },
  {
    id: 'node-cpu-critical',
    name: 'Node CPU Overloaded',
    description: 'A node has CPU utilization above 90%, potentially starving GPU data pipeline',
    severity: 'critical',
    category: 'compute',
    metric: 'node_cpu_max',
    condition: 'gt',
    threshold: 90,
    cooldown_sec: 60,
    runbook: 'Check for runaway processes (`top`/`htop`). Verify no data preprocessing is CPU-bound. Check kernel softirq time with `mpstat`.',
    enabled: true,
  },
  {
    id: 'node-degraded',
    name: 'Node Degraded',
    description: 'A node is reporting degraded status, indicating hardware or software issues',
    severity: 'critical',
    category: 'health',
    metric: 'node_degraded_count',
    condition: 'gt',
    threshold: 0,
    cooldown_sec: 30,
    runbook: 'Inspect node `dmesg` for hardware errors. Check IB link with `ibstat`. Verify GPU health with `nvidia-smi -q`. Consider draining the node.',
    enabled: true,
  },
  {
    id: 'ecn-storm',
    name: 'ECN Congestion Storm',
    description: 'High rate of ECN marks across flows indicates network congestion',
    severity: 'warning',
    category: 'network',
    metric: 'ecn_rate_high',
    condition: 'gt',
    threshold: 500,
    cooldown_sec: 60,
    runbook: 'Check switch ECN/DCQCN settings. Verify PFC is not causing head-of-line blocking. Monitor with `perfquery` and switch telemetry.',
    enabled: true,
  },
  {
    id: 'rdma-retransmit',
    name: 'High RDMA Retransmissions',
    description: 'Elevated retransmission rate on RDMA flows suggests link errors or congestion',
    severity: 'warning',
    category: 'network',
    metric: 'rdma_retransmit_high',
    condition: 'gt',
    threshold: 100,
    cooldown_sec: 90,
    runbook: 'Check physical layer: cable, transceiver, and port errors via `ibdiagnet`. Look for bit error rate (BER) issues. May need cable replacement.',
    enabled: true,
  },
];

const lastAlertFired = new Map();

function evaluateAlerts(trainingMetrics, nodeMetrics, recentFlows) {
  const now = Date.now();
  const newAlerts = [];

  for (const rule of ALERT_RULES) {
    if (!rule.enabled) continue;

    const lastFired = lastAlertFired.get(rule.id) || 0;
    if (now - lastFired < rule.cooldown_sec * 1000) continue;

    let currentValue = null;
    let affectedEntities = [];

    switch (rule.metric) {
      case 'straggler_score':
        currentValue = trainingMetrics.straggler_score;
        if (trainingMetrics.stragglers) {
          affectedEntities = trainingMetrics.stragglers
            .filter(s => s.deviation > rule.threshold)
            .map(s => s.node_id);
        }
        break;
      case 'bubble_ratio':
        currentValue = trainingMetrics.bubble_ratio;
        break;
      case 'gradient_sync_overhead_pct':
        currentValue = trainingMetrics.gradient_sync_overhead_pct;
        break;
      case 'network_saturation_index':
        currentValue = trainingMetrics.network_saturation_index;
        break;
      case 'imbalance_score':
        currentValue = trainingMetrics.imbalance_score;
        break;
      case 'node_cpu_max': {
        const maxCPU = Math.max(...nodeMetrics.map(n => n.cpu_avg));
        currentValue = maxCPU;
        affectedEntities = nodeMetrics.filter(n => n.cpu_avg > rule.threshold).map(n => n.node_id);
        break;
      }
      case 'node_degraded_count': {
        const degraded = nodeMetrics.filter(n => n.status === 'degraded');
        currentValue = degraded.length;
        affectedEntities = degraded.map(n => n.node_id);
        break;
      }
      case 'ecn_rate_high': {
        const totalECN = recentFlows
          .filter(f => f.rdma)
          .reduce((sum, f) => sum + (f.rdma.ecn_marks || 0), 0);
        currentValue = totalECN;
        break;
      }
      case 'rdma_retransmit_high': {
        const totalRetrans = recentFlows
          .filter(f => f.rdma)
          .reduce((sum, f) => sum + (f.rdma.retransmissions || 0), 0);
        currentValue = totalRetrans;
        break;
      }
      default:
        continue;
    }

    if (currentValue === null) continue;

    let triggered = false;
    if (rule.condition === 'gt' && currentValue > rule.threshold) triggered = true;
    if (rule.condition === 'lt' && currentValue < rule.threshold) triggered = true;

    if (triggered) {
      const alert = {
        id: crypto.randomUUID(),
        rule_id: rule.id,
        name: rule.name,
        description: rule.description,
        severity: rule.severity,
        category: rule.category,
        metric: rule.metric,
        current_value: Math.round(currentValue * 100) / 100,
        threshold: rule.threshold,
        condition: rule.condition,
        affected_nodes: affectedEntities,
        runbook: rule.runbook,
        fired_at: new Date().toISOString(),
        acknowledged: false,
        acknowledged_by: null,
        acknowledged_at: null,
        resolved: false,
        resolved_at: null,
      };

      newAlerts.push(alert);
      lastAlertFired.set(rule.id, now);
    }
  }

  return newAlerts;
}

// ── Email notification service ───────────────────────────────
const EMAIL_CONFIG = {
  enabled: process.env.SMTP_HOST ? true : false,
  host: process.env.SMTP_HOST || 'smtp.gmail.com',
  port: parseInt(process.env.SMTP_PORT || '587'),
  user: process.env.SMTP_USER || '',
  pass: process.env.SMTP_PASS || '',
  from: process.env.ALERT_FROM || 'flowpulse-alerts@example.com',
  recipients: (process.env.ALERT_RECIPIENTS || '').split(',').filter(Boolean),
  min_severity: process.env.ALERT_MIN_EMAIL_SEVERITY || 'warning',
};

const SEVERITY_ORDER = { info: 0, warning: 1, critical: 2 };
const emailQueue = [];
let emailSending = false;

async function sendAlertEmail(alert) {
  if (!EMAIL_CONFIG.enabled) {
    console.log(`[email] SMTP not configured — would send: [${alert.severity.toUpperCase()}] ${alert.name}`);
    return;
  }

  if (SEVERITY_ORDER[alert.severity] < SEVERITY_ORDER[EMAIL_CONFIG.min_severity]) return;

  const subject = `[FlowPulse ${alert.severity.toUpperCase()}] ${alert.name}`;
  const body = [
    `Alert: ${alert.name}`,
    `Severity: ${alert.severity.toUpperCase()}`,
    `Category: ${alert.category}`,
    `Time: ${alert.fired_at}`,
    '',
    `Description: ${alert.description}`,
    '',
    `Metric: ${alert.metric}`,
    `Current Value: ${alert.current_value}`,
    `Threshold: ${alert.condition === 'gt' ? '>' : '<'} ${alert.threshold}`,
    '',
    alert.affected_nodes.length > 0 ? `Affected Nodes: ${alert.affected_nodes.join(', ')}` : '',
    '',
    '─── Runbook ───',
    alert.runbook,
    '',
    '─── Actions ───',
    `Acknowledge: ${API_BASE_URL}/api/v1/alerts/${alert.id}/acknowledge`,
    `Dashboard: ${API_BASE_URL}/alerts`,
    '',
    '-- FlowPulse Alert System',
  ].filter(Boolean).join('\n');

  emailQueue.push({ subject, body, alert });
  processEmailQueue();
}

const API_BASE_URL = process.env.API_BASE_URL || 'http://localhost:3000';

async function processEmailQueue() {
  if (emailSending || emailQueue.length === 0) return;
  emailSending = true;

  while (emailQueue.length > 0) {
    const { subject, body, alert } = emailQueue.shift();
    try {
      if (EMAIL_CONFIG.enabled) {
        // Dynamic import to avoid crash when nodemailer isn't installed
        const nodemailer = await import('nodemailer');
        const transporter = nodemailer.default.createTransport({
          host: EMAIL_CONFIG.host,
          port: EMAIL_CONFIG.port,
          secure: EMAIL_CONFIG.port === 465,
          auth: { user: EMAIL_CONFIG.user, pass: EMAIL_CONFIG.pass },
        });
        await transporter.sendMail({
          from: EMAIL_CONFIG.from,
          to: EMAIL_CONFIG.recipients.join(','),
          subject,
          text: body,
        });
        console.log(`[email] Sent: ${subject}`);
      } else {
        console.log(`[email-sim] ${subject}`);
        console.log(`           → ${alert.affected_nodes.length} nodes affected, value=${alert.current_value}`);
      }
    } catch (err) {
      console.error(`[email] Failed to send: ${err.message}`);
    }
  }

  emailSending = false;
}

// ── Flow generation ──────────────────────────────────────────

function generateFlow(src, dst, step) {
  const isRDMA = Math.random() > 0.15;
  const isStraggler = src.is_straggler || dst.is_straggler;
  const baseBW = isStraggler ? randBetween(5e8, 2e9) : randBetween(2e9, 8e9);
  const duration = randBetween(0.01, 2.0);
  const now = Date.now();

  const flow = {
    flow_id: crypto.randomUUID(),
    tenant_id: 'default',
    node_id: src.node_id,
    key: {
      src_ip: src.ip_base,
      dst_ip: dst.ip_base,
      src_port: 40000 + Math.floor(Math.random() * 20000),
      dst_port: isRDMA ? 4791 : 50000 + Math.floor(Math.random() * 10000),
      protocol: 17,
    },
    packets: Math.floor(baseBW / 4096),
    bytes: Math.floor(baseBW),
    first_seen: new Date(now - duration * 1000).toISOString(),
    last_seen: new Date(now).toISOString(),
    direction: Math.random() > 0.5 ? 1 : 2,
  };

  if (isRDMA) {
    const qp = 100 + Math.floor(Math.random() * 500);
    flow.rdma = {
      qp_number: qp,
      dest_qp: qp + 1,
      rdma_msg_rate: Math.floor(randBetween(50000, 500000)),
      retransmissions: isStraggler ? Math.floor(randBetween(50, 500)) : Math.floor(randBetween(0, 10)),
      ecn_marks: isStraggler ? Math.floor(randBetween(100, 2000)) : Math.floor(randBetween(0, 50)),
      cnp_count: isStraggler ? Math.floor(randBetween(20, 400)) : Math.floor(randBetween(0, 5)),
    };
  }

  return flow;
}

function generateNodeMetrics(node, step) {
  const cpuBase = node.is_straggler ? randBetween(70, 95) : randBetween(30, 65);
  const kernelPct = node.is_straggler ? randBetween(15, 35) : randBetween(5, 12);
  const softirqPct = node.is_straggler ? randBetween(8, 20) : randBetween(1, 5);
  const ibUtil = node.is_straggler ? randBetween(20, 50) : randBetween(55, 92);
  const phase = (step * 0.1 + nodes.indexOf(node) * 0.3);
  const wave = Math.sin(phase) * 5;

  return {
    node_id: node.node_id,
    cpu_avg: Math.min(100, Math.max(0, cpuBase + wave)),
    ib_util_pct: Math.min(100, Math.max(0, ibUtil + wave * 2)),
    tx_bytes: Math.floor(randBetween(1e9, 20e9)),
    rx_bytes: Math.floor(randBetween(1e9, 20e9)),
    status: node.is_straggler && Math.random() > 0.7 ? 'degraded' : 'healthy',
    cpu_kernel_pct: kernelPct,
    cpu_softirq_pct: softirqPct,
    context_switches: Math.floor(randBetween(50000, 300000)),
  };
}

function generateTrainingMetrics(nodeMetrics, step) {
  const avgIB = nodeMetrics.reduce((s, n) => s + n.ib_util_pct, 0) / nodeMetrics.length;
  const ibValues = nodeMetrics.map(n => n.ib_util_pct).sort((a, b) => a - b);
  const median = ibValues[Math.floor(ibValues.length / 2)];
  const maxDev = Math.max(...ibValues.map(v => Math.abs(v - median)));
  const phase = (step % 20) / 20;
  const inCollective = phase > 0.6 && phase < 0.9;

  const stragglers = nodeMetrics
    .filter(n => Math.abs(n.ib_util_pct - median) > 15)
    .map(n => ({
      node_id: n.node_id,
      deviation: Math.abs(n.ib_util_pct - median),
      latency_p99: n.status === 'degraded' ? randBetween(2.5, 8.0) : randBetween(0.5, 2.0),
    }))
    .sort((a, b) => b.deviation - a.deviation)
    .slice(0, 10);

  return {
    tenant_id: 'default',
    straggler_score: Math.min(100, maxDev / Math.max(median, 1) * 100),
    bubble_ratio: inCollective ? randBetween(8, 25) : randBetween(2, 8),
    gradient_sync_overhead_pct: inCollective ? randBetween(15, 35) : randBetween(5, 12),
    network_saturation_index: avgIB,
    imbalance_score: (maxDev / Math.max(avgIB, 1)) * 50,
    stragglers,
    timestamp: new Date().toISOString(),
  };
}

// ── REST API ─────────────────────────────────────────────────

function readBody(req) {
  return new Promise((resolve) => {
    const chunks = [];
    req.on('data', (c) => chunks.push(c));
    req.on('end', () => {
      try { resolve(JSON.parse(Buffer.concat(chunks).toString())); }
      catch { resolve({}); }
    });
  });
}

async function handleRequest(req, res) {
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Headers', 'Authorization, Content-Type');
  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, PATCH, OPTIONS');
  res.setHeader('Cache-Control', 'no-store');

  if (req.method === 'OPTIONS') { res.writeHead(204); res.end(); return; }

  const url = new URL(req.url, 'http://localhost');

  if (url.pathname === '/healthz') {
    json(res, { status: 'ok', flows: flows.length, nodes: NUM_NODES, step: trainingStep, active_alerts: alerts.filter(a => !a.resolved && !a.acknowledged).length });
    return;
  }

  if (url.pathname === '/api/v1/flows') {
    const limit = Math.min(parseInt(url.searchParams.get('limit') || '200'), 10000);
    const offset = parseInt(url.searchParams.get('offset') || '0');
    json(res, { flows: flows.slice(offset, offset + limit), total_count: flows.length });
    return;
  }

  if (url.pathname === '/api/v1/topology') {
    json(res, { nodes: nodes.map(n => generateNodeMetrics(n, trainingStep)) });
    return;
  }

  if (url.pathname === '/api/v1/metrics/training') {
    const nodeMetrics = nodes.map(n => generateNodeMetrics(n, trainingStep));
    json(res, generateTrainingMetrics(nodeMetrics, trainingStep));
    return;
  }

  // ── Alert endpoints ──────────────────────────────────────

  if (url.pathname === '/api/v1/alerts' && req.method === 'GET') {
    const severity = url.searchParams.get('severity');
    const status = url.searchParams.get('status');
    let filtered = [...alerts];
    if (severity) filtered = filtered.filter(a => a.severity === severity);
    if (status === 'active') filtered = filtered.filter(a => !a.resolved && !a.acknowledged);
    if (status === 'acknowledged') filtered = filtered.filter(a => a.acknowledged && !a.resolved);
    if (status === 'resolved') filtered = filtered.filter(a => a.resolved);
    json(res, { alerts: filtered, total: filtered.length });
    return;
  }

  if (url.pathname === '/api/v1/alerts/rules' && req.method === 'GET') {
    json(res, { rules: ALERT_RULES });
    return;
  }

  if (url.pathname === '/api/v1/alerts/summary' && req.method === 'GET') {
    const active = alerts.filter(a => !a.resolved && !a.acknowledged);
    const acknowledged = alerts.filter(a => a.acknowledged && !a.resolved);
    json(res, {
      total: alerts.length,
      active: active.length,
      acknowledged: acknowledged.length,
      by_severity: {
        critical: active.filter(a => a.severity === 'critical').length,
        warning: active.filter(a => a.severity === 'warning').length,
        info: active.filter(a => a.severity === 'info').length,
      },
      by_category: {
        training: active.filter(a => a.category === 'training').length,
        network: active.filter(a => a.category === 'network').length,
        compute: active.filter(a => a.category === 'compute').length,
        health: active.filter(a => a.category === 'health').length,
      },
    });
    return;
  }

  const ackMatch = url.pathname.match(/^\/api\/v1\/alerts\/([^/]+)\/acknowledge$/);
  if (ackMatch && req.method === 'POST') {
    const alertId = ackMatch[1];
    const alert = alerts.find(a => a.id === alertId);
    if (!alert) { res.writeHead(404); res.end(JSON.stringify({ error: 'alert not found' })); return; }
    const body = await readBody(req);
    alert.acknowledged = true;
    alert.acknowledged_by = body.user || 'operator';
    alert.acknowledged_at = new Date().toISOString();
    broadcast('flowpulse.default.alert_ack', alert);
    json(res, alert);
    return;
  }

  const resolveMatch = url.pathname.match(/^\/api\/v1\/alerts\/([^/]+)\/resolve$/);
  if (resolveMatch && req.method === 'POST') {
    const alertId = resolveMatch[1];
    const alert = alerts.find(a => a.id === alertId);
    if (!alert) { res.writeHead(404); res.end(JSON.stringify({ error: 'alert not found' })); return; }
    alert.resolved = true;
    alert.resolved_at = new Date().toISOString();
    broadcast('flowpulse.default.alert_resolved', alert);
    json(res, alert);
    return;
  }

  if (url.pathname === '/api/v1/alerts/test-email' && req.method === 'POST') {
    const testAlert = {
      id: 'test-' + crypto.randomUUID().slice(0, 8),
      rule_id: 'test',
      name: 'Test Alert Notification',
      description: 'This is a test alert to verify email delivery',
      severity: 'warning',
      category: 'test',
      metric: 'test',
      current_value: 42,
      threshold: 0,
      condition: 'gt',
      affected_nodes: ['gpu-node-000'],
      runbook: 'No action needed — this is a test alert.',
      fired_at: new Date().toISOString(),
      acknowledged: false,
    };
    await sendAlertEmail(testAlert);
    json(res, { message: 'Test email queued', smtp_configured: EMAIL_CONFIG.enabled });
    return;
  }

  // ── Email config endpoint ──────────────────────────────────
  if (url.pathname === '/api/v1/alerts/email-config' && req.method === 'GET') {
    json(res, {
      enabled: EMAIL_CONFIG.enabled,
      host: EMAIL_CONFIG.host,
      port: EMAIL_CONFIG.port,
      from: EMAIL_CONFIG.from,
      recipients: EMAIL_CONFIG.recipients,
      min_severity: EMAIL_CONFIG.min_severity,
    });
    return;
  }

  if (url.pathname === '/api/v1/alerts/email-config' && req.method === 'PUT') {
    const body = await readBody(req);
    if (body.host) EMAIL_CONFIG.host = body.host;
    if (body.port) EMAIL_CONFIG.port = body.port;
    if (body.user) EMAIL_CONFIG.user = body.user;
    if (body.pass) EMAIL_CONFIG.pass = body.pass;
    if (body.from) EMAIL_CONFIG.from = body.from;
    if (body.recipients) EMAIL_CONFIG.recipients = body.recipients;
    if (body.min_severity) EMAIL_CONFIG.min_severity = body.min_severity;
    if (body.host && body.user) EMAIL_CONFIG.enabled = true;
    json(res, { message: 'Email config updated', enabled: EMAIL_CONFIG.enabled });
    return;
  }

  res.writeHead(404);
  res.end(JSON.stringify({ error: 'not found' }));
}

function json(res, data) {
  res.writeHead(200, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify(data));
}

// ── WebSocket ────────────────────────────────────────────────
const server = http.createServer(handleRequest);
const wss = new WebSocketServer({ server, path: '/ws' });
const wsClients = new Set();

wss.on('connection', (ws, req) => {
  const tenant = new URL(req.url, 'http://localhost').searchParams.get('tenant_id') || 'default';
  console.log(`[ws] client connected (tenant: ${tenant})`);
  wsClients.add(ws);
  ws.on('close', () => wsClients.delete(ws));
  ws.on('error', () => wsClients.delete(ws));
});

function broadcast(subject, data) {
  const msg = JSON.stringify({ subject, data });
  for (const ws of wsClients) {
    if (ws.readyState === 1) ws.send(msg);
  }
}

// ── Simulation loop ──────────────────────────────────────────

for (let i = 0; i < 500; i++) {
  const src = pick(nodes);
  let dst = pick(nodes);
  while (dst === src) dst = pick(nodes);
  flows.push(generateFlow(src, dst, 0));
}

setInterval(() => {
  trainingStep++;

  const newFlows = [];
  const batchSize = 5 + Math.floor(Math.random() * 15);
  for (let i = 0; i < batchSize; i++) {
    const src = pick(nodes);
    let dst = pick(nodes);
    while (dst === src) dst = pick(nodes);
    const f = generateFlow(src, dst, trainingStep);
    newFlows.push(f);
    flows.unshift(f);
  }
  if (flows.length > 50000) flows.length = 50000;

  broadcast('flowpulse.default.flows', {
    node_id: newFlows[0]?.node_id,
    tenant_id: 'default',
    flows: newFlows,
  });

  if (trainingStep % 2 === 0) {
    const nodeMetrics = nodes.map(n => generateNodeMetrics(n, trainingStep));
    broadcast('flowpulse.default.metrics', nodeMetrics);
  }

  if (trainingStep % 3 === 0) {
    const nodeMetrics = nodes.map(n => generateNodeMetrics(n, trainingStep));
    const tm = generateTrainingMetrics(nodeMetrics, trainingStep);
    broadcast('flowpulse.default.training', tm);

    // ── Evaluate alert rules every 3 ticks ───────────────
    const recentFlows = flows.slice(0, 100);
    const newAlerts = evaluateAlerts(tm, nodeMetrics, recentFlows);

    for (const alert of newAlerts) {
      alerts.unshift(alert);
      alertHistory.push(alert);
      if (alerts.length > 1000) alerts.length = 1000;

      broadcast('flowpulse.default.alert', alert);
      sendAlertEmail(alert);

      const sev = alert.severity === 'critical' ? '\x1b[31m' : alert.severity === 'warning' ? '\x1b[33m' : '\x1b[36m';
      console.log(`${sev}[ALERT]  ${alert.severity.toUpperCase()}\x1b[0m  ${alert.name}  (${alert.metric}=${alert.current_value}, threshold=${alert.threshold})${alert.affected_nodes.length ? '  nodes: ' + alert.affected_nodes.join(', ') : ''}`);
    }
  }

}, TICK_MS);

// ── Start ────────────────────────────────────────────────────
const PORT = 8080;
server.listen(PORT, () => {
  console.log('');
  console.log('  ╔══════════════════════════════════════════════════════╗');
  console.log('  ║          FlowPulse Mock Data Server                  ║');
  console.log('  ╠══════════════════════════════════════════════════════╣');
  console.log(`  ║  REST API:     http://localhost:${PORT}                ║`);
  console.log(`  ║  WebSocket:    ws://localhost:${PORT}/ws                ║`);
  console.log(`  ║  Health:       http://localhost:${PORT}/healthz         ║`);
  console.log('  ╠══════════════════════════════════════════════════════╣');
  console.log(`  ║  Simulating:   ${NUM_NODES} GPU nodes, ${NUM_GPUS_PER_NODE} GPUs each          ║`);
  console.log(`  ║  IB Speed:     ${IB_PEAK_GBPS} Gbps NDR                      ║`);
  console.log(`  ║  Stragglers:   nodes ${[...STRAGGLER_NODES].join(', ')}                    ║`);
  console.log(`  ║  Alert Rules:  ${ALERT_RULES.length} active                         ║`);
  console.log(`  ║  Email:        ${EMAIL_CONFIG.enabled ? 'CONFIGURED' : 'disabled (set SMTP_* env vars)'}  ║`);
  console.log('  ╚══════════════════════════════════════════════════════╝');
  console.log('');
  console.log('  Email config env vars:');
  console.log('    SMTP_HOST, SMTP_PORT, SMTP_USER, SMTP_PASS');
  console.log('    ALERT_FROM, ALERT_RECIPIENTS (comma-separated)');
  console.log('    ALERT_MIN_EMAIL_SEVERITY (info|warning|critical)');
  console.log('');
});
