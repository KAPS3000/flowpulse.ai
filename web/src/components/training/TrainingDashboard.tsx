'use client';

import { useStore } from '@/lib/store';

export function TrainingDashboard() {
  const metrics = useStore((s) => s.trainingMetrics);

  if (!metrics) {
    return (
      <div className="card text-center py-12 text-fp-muted">
        No training metrics available. Waiting for data...
      </div>
    );
  }

  const gauges = [
    {
      label: 'Network Saturation',
      value: metrics.network_saturation_index,
      max: 100,
      unit: '%',
      description: 'Actual bandwidth / peak capacity per rail',
    },
    {
      label: 'Straggler Score',
      value: metrics.straggler_score,
      max: 100,
      unit: '%',
      description: 'Per-node deviation from median collective time',
    },
    {
      label: 'Bubble Ratio',
      value: metrics.bubble_ratio,
      max: 100,
      unit: '%',
      description: 'GPU idle time from network/CPU waits',
    },
    {
      label: 'Gradient Sync Overhead',
      value: metrics.gradient_sync_overhead_pct,
      max: 100,
      unit: '%',
      description: 'Step time spent in NCCL collectives',
    },
    {
      label: 'Traffic Imbalance',
      value: metrics.imbalance_score,
      max: 100,
      unit: '%',
      description: 'Asymmetry across InfiniBand rails',
    },
  ];

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {gauges.map((g) => (
          <div key={g.label} className="card">
            <div className="flex items-center justify-between mb-3">
              <span className="metric-label">{g.label}</span>
              <span className={`text-2xl font-bold tabular-nums ${getColor(g.value)}`}>
                {g.value.toFixed(1)}{g.unit}
              </span>
            </div>
            <div className="w-full bg-fp-bg rounded-full h-2">
              <div
                className={`h-2 rounded-full transition-all ${getBarColor(g.value)}`}
                style={{ width: `${Math.min(g.value, 100)}%` }}
              />
            </div>
            <p className="mt-2 text-xs text-fp-muted">{g.description}</p>
          </div>
        ))}
      </div>

      {metrics.stragglers && metrics.stragglers.length > 0 && (
        <div className="card">
          <h3 className="text-sm font-semibold text-white mb-3">Straggler Nodes</h3>
          <table className="w-full text-sm">
            <thead>
              <tr className="text-fp-muted text-left">
                <th className="pb-2">Node</th>
                <th className="pb-2">Deviation</th>
                <th className="pb-2">P99 Latency</th>
              </tr>
            </thead>
            <tbody>
              {metrics.stragglers.map((s) => (
                <tr key={s.node_id} className="border-t border-fp-border/50">
                  <td className="py-1.5 font-mono text-xs">{s.node_id}</td>
                  <td className="py-1.5 text-fp-warning">{s.deviation.toFixed(1)}%</td>
                  <td className="py-1.5">{s.latency_p99.toFixed(2)}ms</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function getColor(val: number): string {
  if (val > 60) return 'text-fp-danger';
  if (val > 30) return 'text-fp-warning';
  return 'text-fp-success';
}

function getBarColor(val: number): string {
  if (val > 60) return 'bg-fp-danger';
  if (val > 30) return 'bg-fp-warning';
  return 'bg-fp-success';
}
