'use client';

import { useState, useCallback } from 'react';
import { useStore } from '@/lib/store';
import { api, type Alert } from '@/lib/api';

const SEVERITY_STYLES = {
  critical: {
    bg: 'bg-red-500/10',
    border: 'border-red-500/30',
    badge: 'bg-red-500 text-white',
    icon: '!!!',
    text: 'text-red-400',
    pulse: 'animate-pulse',
  },
  warning: {
    bg: 'bg-amber-500/10',
    border: 'border-amber-500/30',
    badge: 'bg-amber-500 text-black',
    icon: '!!',
    text: 'text-amber-400',
    pulse: '',
  },
  info: {
    bg: 'bg-blue-500/10',
    border: 'border-blue-500/30',
    badge: 'bg-blue-500 text-white',
    icon: 'i',
    text: 'text-blue-400',
    pulse: '',
  },
};

type FilterStatus = 'all' | 'active' | 'acknowledged' | 'resolved';
type FilterSeverity = 'all' | 'critical' | 'warning' | 'info';

export function AlertPanel() {
  const alerts = useStore((s) => s.alerts);
  const alertSummary = useStore((s) => s.alertSummary);
  const updateAlert = useStore((s) => s.updateAlert);
  const [statusFilter, setStatusFilter] = useState<FilterStatus>('active');
  const [severityFilter, setSeverityFilter] = useState<FilterSeverity>('all');
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [actionLoading, setActionLoading] = useState<string | null>(null);

  const filtered = alerts.filter((a) => {
    if (statusFilter === 'active' && (a.acknowledged || a.resolved)) return false;
    if (statusFilter === 'acknowledged' && (!a.acknowledged || a.resolved)) return false;
    if (statusFilter === 'resolved' && !a.resolved) return false;
    if (severityFilter !== 'all' && a.severity !== severityFilter) return false;
    return true;
  });

  const handleAcknowledge = useCallback(async (alertId: string) => {
    setActionLoading(alertId);
    try {
      const updated = await api.acknowledgeAlert('demo', alertId);
      updateAlert(updated);
    } catch { /* ignore */ }
    setActionLoading(null);
  }, [updateAlert]);

  const handleResolve = useCallback(async (alertId: string) => {
    setActionLoading(alertId);
    try {
      const updated = await api.resolveAlert('demo', alertId);
      updateAlert(updated);
    } catch { /* ignore */ }
    setActionLoading(null);
  }, [updateAlert]);

  const handleTestEmail = useCallback(async () => {
    try {
      const result = await api.testEmail('demo');
      window.alert(result.smtp_configured
        ? 'Test email sent successfully!'
        : 'SMTP not configured. Set SMTP_HOST, SMTP_USER, SMTP_PASS environment variables on the server. Check server console for simulated output.');
    } catch { /* ignore */ }
  }, []);

  const timeAgo = (iso: string) => {
    const diff = Date.now() - new Date(iso).getTime();
    if (diff < 60000) return `${Math.floor(diff / 1000)}s ago`;
    if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
    return `${Math.floor(diff / 3600000)}h ago`;
  };

  return (
    <div className="space-y-6">
      {/* Summary cards */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <SummaryCard
          label="Active Alerts"
          value={alertSummary?.active ?? 0}
          color="text-red-400"
          pulse={!!alertSummary && alertSummary.active > 0}
        />
        <SummaryCard label="Critical" value={alertSummary?.by_severity.critical ?? 0} color="text-red-500" />
        <SummaryCard label="Warning" value={alertSummary?.by_severity.warning ?? 0} color="text-amber-400" />
        <SummaryCard label="Acknowledged" value={alertSummary?.acknowledged ?? 0} color="text-blue-400" />
      </div>

      {/* Category breakdown */}
      {alertSummary && (
        <div className="card">
          <h3 className="text-sm font-semibold text-white mb-3">Active by Category</h3>
          <div className="flex gap-6">
            {Object.entries(alertSummary.by_category).map(([cat, count]) => (
              <div key={cat} className="flex items-center gap-2">
                <span className={`w-2.5 h-2.5 rounded-full ${count > 0 ? 'bg-red-500' : 'bg-fp-border'}`} />
                <span className="text-sm text-fp-text capitalize">{cat}</span>
                <span className="text-sm font-mono text-fp-muted">{count}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Filters & actions */}
      <div className="flex flex-wrap items-center gap-3">
        <div className="flex bg-fp-bg rounded-lg p-0.5">
          {(['all', 'active', 'acknowledged', 'resolved'] as FilterStatus[]).map((s) => (
            <button
              key={s}
              onClick={() => setStatusFilter(s)}
              className={`px-3 py-1.5 text-xs rounded-md capitalize transition-colors ${
                statusFilter === s
                  ? 'bg-fp-surface text-white'
                  : 'text-fp-muted hover:text-fp-text'
              }`}
            >
              {s}
            </button>
          ))}
        </div>
        <div className="flex bg-fp-bg rounded-lg p-0.5">
          {(['all', 'critical', 'warning', 'info'] as FilterSeverity[]).map((s) => (
            <button
              key={s}
              onClick={() => setSeverityFilter(s)}
              className={`px-3 py-1.5 text-xs rounded-md capitalize transition-colors ${
                severityFilter === s
                  ? 'bg-fp-surface text-white'
                  : 'text-fp-muted hover:text-fp-text'
              }`}
            >
              {s}
            </button>
          ))}
        </div>
        <div className="flex-1" />
        <button
          onClick={handleTestEmail}
          className="px-3 py-1.5 text-xs bg-fp-surface border border-fp-border rounded-lg text-fp-muted hover:text-white hover:border-fp-accent transition-colors"
        >
          Test Email
        </button>
        <span className="text-xs text-fp-muted">{filtered.length} alerts</span>
      </div>

      {/* Alert list */}
      <div className="space-y-2">
        {filtered.length === 0 ? (
          <div className="card text-center py-12 text-fp-muted">
            {statusFilter === 'active'
              ? 'No active alerts. All systems healthy.'
              : 'No alerts matching filters.'}
          </div>
        ) : (
          filtered.map((alert) => (
            <AlertCard
              key={alert.id}
              alert={alert}
              expanded={expandedId === alert.id}
              onToggle={() => setExpandedId(expandedId === alert.id ? null : alert.id)}
              onAcknowledge={handleAcknowledge}
              onResolve={handleResolve}
              loading={actionLoading === alert.id}
              timeAgo={timeAgo}
            />
          ))
        )}
      </div>
    </div>
  );
}

function SummaryCard({ label, value, color, pulse }: { label: string; value: number; color: string; pulse?: boolean }) {
  return (
    <div className="card">
      <p className="metric-label">{label}</p>
      <div className="mt-2 flex items-center gap-2">
        <span className={`text-2xl font-bold tabular-nums ${color}`}>{value}</span>
        {pulse && <span className="w-2 h-2 rounded-full bg-red-500 animate-pulse" />}
      </div>
    </div>
  );
}

function AlertCard({
  alert,
  expanded,
  onToggle,
  onAcknowledge,
  onResolve,
  loading,
  timeAgo,
}: {
  alert: Alert;
  expanded: boolean;
  onToggle: () => void;
  onAcknowledge: (id: string) => void;
  onResolve: (id: string) => void;
  loading: boolean;
  timeAgo: (iso: string) => string;
}) {
  const style = SEVERITY_STYLES[alert.severity] || SEVERITY_STYLES.info;

  return (
    <div className={`rounded-lg border ${style.border} ${style.bg} overflow-hidden transition-all`}>
      {/* Header row */}
      <button
        onClick={onToggle}
        className="w-full px-4 py-3 flex items-center gap-3 text-left hover:bg-white/5 transition-colors"
      >
        <span className={`${style.badge} text-xs font-bold px-2 py-0.5 rounded ${style.pulse}`}>
          {alert.severity.toUpperCase()}
        </span>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="text-sm font-semibold text-white truncate">{alert.name}</span>
            <span className="text-xs text-fp-muted capitalize px-1.5 py-0.5 bg-fp-bg rounded">{alert.category}</span>
          </div>
          <p className="text-xs text-fp-muted mt-0.5 truncate">{alert.description}</p>
        </div>
        <div className="text-right shrink-0">
          <div className={`text-sm font-mono font-bold ${style.text}`}>
            {alert.current_value}
            <span className="text-fp-muted text-xs ml-1">/ {alert.condition === 'gt' ? '>' : '<'}{alert.threshold}</span>
          </div>
          <div className="text-xs text-fp-muted">{timeAgo(alert.fired_at)}</div>
        </div>
        {alert.acknowledged && (
          <span className="text-xs text-blue-400 bg-blue-500/10 px-2 py-0.5 rounded shrink-0">ACK</span>
        )}
        {alert.resolved && (
          <span className="text-xs text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded shrink-0">RESOLVED</span>
        )}
        <svg className={`w-4 h-4 text-fp-muted transition-transform ${expanded ? 'rotate-180' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      {/* Expanded detail */}
      {expanded && (
        <div className="px-4 pb-4 space-y-3 border-t border-white/5">
          {/* Affected nodes */}
          {alert.affected_nodes.length > 0 && (
            <div className="pt-3">
              <span className="text-xs font-semibold text-fp-muted uppercase tracking-wider">Affected Nodes</span>
              <div className="flex flex-wrap gap-1.5 mt-1.5">
                {alert.affected_nodes.map((node) => (
                  <span key={node} className="text-xs font-mono bg-fp-bg border border-fp-border px-2 py-0.5 rounded text-fp-text">
                    {node}
                  </span>
                ))}
              </div>
            </div>
          )}

          {/* Runbook */}
          <div>
            <span className="text-xs font-semibold text-fp-muted uppercase tracking-wider">Runbook</span>
            <div className="mt-1.5 bg-fp-bg border border-fp-border rounded-lg p-3">
              <p className="text-sm text-fp-text leading-relaxed whitespace-pre-wrap">{alert.runbook}</p>
            </div>
          </div>

          {/* Metric detail */}
          <div className="grid grid-cols-2 md:grid-cols-4 gap-3 text-xs">
            <div><span className="text-fp-muted">Metric</span><div className="font-mono text-fp-text mt-0.5">{alert.metric}</div></div>
            <div><span className="text-fp-muted">Rule ID</span><div className="font-mono text-fp-text mt-0.5">{alert.rule_id}</div></div>
            <div><span className="text-fp-muted">Fired</span><div className="font-mono text-fp-text mt-0.5">{new Date(alert.fired_at).toLocaleTimeString()}</div></div>
            <div><span className="text-fp-muted">Alert ID</span><div className="font-mono text-fp-text mt-0.5 truncate">{alert.id.slice(0, 12)}...</div></div>
          </div>

          {/* Acknowledge / Resolve timeline */}
          {alert.acknowledged && (
            <div className="text-xs text-blue-400">
              Acknowledged by <span className="font-mono">{alert.acknowledged_by}</span> at {new Date(alert.acknowledged_at!).toLocaleTimeString()}
            </div>
          )}
          {alert.resolved && (
            <div className="text-xs text-emerald-400">
              Resolved at {new Date(alert.resolved_at!).toLocaleTimeString()}
            </div>
          )}

          {/* Actions */}
          {!alert.resolved && (
            <div className="flex gap-2 pt-1">
              {!alert.acknowledged && (
                <button
                  onClick={() => onAcknowledge(alert.id)}
                  disabled={loading}
                  className="px-4 py-1.5 text-xs font-semibold bg-blue-500/20 border border-blue-500/30 text-blue-400 rounded-lg hover:bg-blue-500/30 transition-colors disabled:opacity-50"
                >
                  {loading ? 'Acknowledging...' : 'Acknowledge'}
                </button>
              )}
              <button
                onClick={() => onResolve(alert.id)}
                disabled={loading}
                className="px-4 py-1.5 text-xs font-semibold bg-emerald-500/20 border border-emerald-500/30 text-emerald-400 rounded-lg hover:bg-emerald-500/30 transition-colors disabled:opacity-50"
              >
                {loading ? 'Resolving...' : 'Resolve'}
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
