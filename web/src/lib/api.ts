const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

let cachedToken: string | null = null;

async function getAuthToken(): Promise<string> {
  if (cachedToken) return cachedToken;
  const res = await fetch(`${API_BASE}/api/v1/auth/token?tenant_id=local-dev`, {
    method: 'POST',
  });
  if (!res.ok) throw new Error('Failed to obtain auth token');
  const data = await res.json();
  cachedToken = data.token;
  return cachedToken!;
}

async function fetchAPI<T>(path: string, _token?: string): Promise<T> {
  const token = await getAuthToken();
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${token}`,
  };

  const res = await fetch(`${API_BASE}${path}`, { headers, cache: 'no-store' });
  if (res.status === 401) {
    cachedToken = null;
    const freshToken = await getAuthToken();
    const retry = await fetch(`${API_BASE}${path}`, {
      headers: { ...headers, 'Authorization': `Bearer ${freshToken}` },
      cache: 'no-store',
    });
    if (!retry.ok) throw new Error(`API error: ${retry.status} ${retry.statusText}`);
    return retry.json();
  }
  if (!res.ok) {
    throw new Error(`API error: ${res.status} ${res.statusText}`);
  }
  return res.json();
}

async function postAPI<T>(path: string, _token?: string, body?: unknown): Promise<T> {
  const token = await getAuthToken();
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${token}`,
  };

  const res = await fetch(`${API_BASE}${path}`, {
    method: 'POST',
    headers,
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw new Error(`API error: ${res.status} ${res.statusText}`);
  }
  return res.json();
}

export interface Flow {
  flow_id: string;
  tenant_id: string;
  node_id: string;
  key: {
    src_ip: number;
    dst_ip: number;
    src_port: number;
    dst_port: number;
    protocol: number;
  };
  packets: number;
  bytes: number;
  first_seen: string;
  last_seen: string;
  direction: number;
  rdma?: {
    qp_number: number;
    dest_qp: number;
    rdma_msg_rate: number;
    retransmissions: number;
    ecn_marks: number;
    cnp_count: number;
  };
}

export interface TrainingMetrics {
  tenant_id: string;
  straggler_score: number;
  bubble_ratio: number;
  gradient_sync_overhead_pct: number;
  network_saturation_index: number;
  imbalance_score: number;
  stragglers?: { node_id: string; deviation: number; latency_p99: number }[];
  timestamp: string;
}

export interface TopologyNode {
  node_id: string;
  cpu_avg: number;
  ib_util_pct: number;
  tx_bytes: number;
  rx_bytes: number;
  status: string;
}

export interface Alert {
  id: string;
  rule_id: string;
  name: string;
  description: string;
  severity: 'critical' | 'warning' | 'info';
  category: string;
  metric: string;
  current_value: number;
  threshold: number;
  condition: string;
  affected_nodes: string[];
  runbook: string;
  fired_at: string;
  acknowledged: boolean;
  acknowledged_by: string | null;
  acknowledged_at: string | null;
  resolved: boolean;
  resolved_at: string | null;
}

export interface AlertSummary {
  total: number;
  active: number;
  acknowledged: number;
  by_severity: { critical: number; warning: number; info: number };
  by_category: Record<string, number>;
}

export interface AlertRule {
  id: string;
  name: string;
  description: string;
  severity: string;
  category: string;
  metric: string;
  condition: string;
  threshold: number;
  cooldown_sec: number;
  runbook: string;
  enabled: boolean;
}

export function ipToString(ip: number): string {
  return `${(ip >>> 24) & 0xff}.${(ip >>> 16) & 0xff}.${(ip >>> 8) & 0xff}.${ip & 0xff}`;
}

export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
}

export function protocolName(proto: number): string {
  switch (proto) {
    case 6: return 'TCP';
    case 17: return 'UDP';
    case 1: return 'ICMP';
    default: return `Proto ${proto}`;
  }
}

export const api = {
  getFlows: (token: string, params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ flows: Flow[]; total_count: number }>(`/api/v1/flows${qs}`, token);
  },
  getTrainingMetrics: (token: string, window = '5m') =>
    fetchAPI<TrainingMetrics>(`/api/v1/metrics/training?window=${window}`, token),
  getTopology: (token: string) =>
    fetchAPI<{ nodes: TopologyNode[] }>('/api/v1/topology', token),
  getAlerts: (token: string, params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ alerts: Alert[]; total: number }>(`/api/v1/alerts${qs}`, token);
  },
  getAlertSummary: (token: string) =>
    fetchAPI<AlertSummary>('/api/v1/alerts/summary', token),
  getAlertRules: (token: string) =>
    fetchAPI<{ rules: AlertRule[] }>('/api/v1/alerts/rules', token),
  acknowledgeAlert: (token: string, alertId: string) =>
    postAPI<Alert>(`/api/v1/alerts/${alertId}/acknowledge`, token, { user: 'operator' }),
  resolveAlert: (token: string, alertId: string) =>
    postAPI<Alert>(`/api/v1/alerts/${alertId}/resolve`, token, {}),
  testEmail: (token: string) =>
    postAPI<{ message: string; smtp_configured: boolean }>('/api/v1/alerts/test-email', token, {}),
};
