'use client';

import { useStore } from '@/lib/store';

export function StragglerLeaderboard() {
  const metrics = useStore((s) => s.trainingMetrics);
  const stragglers = metrics?.stragglers ?? [];

  return (
    <div className="card p-0 overflow-hidden">
      <div className="px-4 py-3 border-b border-fp-border">
        <h2 className="text-sm font-semibold text-white">Straggler Leaderboard</h2>
        <p className="text-xs text-fp-muted mt-0.5">
          Nodes with highest deviation from median collective completion time
        </p>
      </div>
      {stragglers.length === 0 ? (
        <div className="p-6 text-center text-fp-muted text-sm">
          No stragglers detected. All nodes performing within normal range.
        </div>
      ) : (
        <div className="divide-y divide-fp-border/50">
          {stragglers.map((s, i) => {
            const severity = s.deviation > 30 ? 'danger' : s.deviation > 15 ? 'warning' : 'healthy';
            return (
              <div key={s.node_id} className="px-4 py-3 flex items-center gap-4 hover:bg-fp-bg/50">
                <span className="text-lg font-bold text-fp-muted w-6 text-right">
                  {i + 1}
                </span>
                <div className="flex-1 min-w-0">
                  <span className="font-mono text-sm text-fp-text">{s.node_id}</span>
                </div>
                <div className="text-right">
                  <span className={`text-sm font-semibold ${
                    severity === 'danger' ? 'text-fp-danger' :
                    severity === 'warning' ? 'text-fp-warning' : 'text-fp-success'
                  }`}>
                    {s.deviation.toFixed(1)}%
                  </span>
                  <span className="text-xs text-fp-muted ml-1">deviation</span>
                </div>
                <div className="w-32">
                  <div className="w-full bg-fp-bg rounded-full h-1.5">
                    <div
                      className={`h-1.5 rounded-full ${
                        severity === 'danger' ? 'bg-fp-danger' :
                        severity === 'warning' ? 'bg-fp-warning' : 'bg-fp-success'
                      }`}
                      style={{ width: `${Math.min(s.deviation, 100)}%` }}
                    />
                  </div>
                </div>
                {s.latency_p99 > 0 && (
                  <span className="text-xs text-fp-muted">
                    p99: {s.latency_p99.toFixed(2)}ms
                  </span>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
