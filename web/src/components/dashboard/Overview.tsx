'use client';

import { useStore } from '@/lib/store';
import { MetricCard } from './MetricCard';

export function Overview() {
  const metrics = useStore((s) => s.trainingMetrics);
  const flowCount = useStore((s) => s.totalFlowCount);
  const nodeCount = useStore((s) => s.topologyNodes.length);

  const getStatus = (val: number, warnThresh: number, dangerThresh: number) => {
    if (val >= dangerThresh) return 'danger' as const;
    if (val >= warnThresh) return 'warning' as const;
    return 'healthy' as const;
  };

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
      <MetricCard label="Active Flows" value={flowCount.toLocaleString()} />
      <MetricCard label="Nodes" value={nodeCount} />
      <MetricCard
        label="Straggler Score"
        value={metrics?.straggler_score?.toFixed(1) ?? '--'}
        unit="%"
        status={metrics ? getStatus(metrics.straggler_score, 15, 30) : 'healthy'}
      />
      <MetricCard
        label="Bubble Ratio"
        value={metrics?.bubble_ratio?.toFixed(1) ?? '--'}
        unit="%"
        status={metrics ? getStatus(metrics.bubble_ratio, 10, 25) : 'healthy'}
      />
      <MetricCard
        label="Gradient Sync Overhead"
        value={metrics?.gradient_sync_overhead_pct?.toFixed(1) ?? '--'}
        unit="%"
        status={metrics ? getStatus(metrics.gradient_sync_overhead_pct, 20, 40) : 'healthy'}
      />
      <MetricCard
        label="Network Saturation"
        value={metrics?.network_saturation_index?.toFixed(1) ?? '--'}
        unit="%"
        status={metrics ? getStatus(metrics.network_saturation_index, 70, 90) : 'healthy'}
      />
      <MetricCard
        label="Imbalance Score"
        value={metrics?.imbalance_score?.toFixed(1) ?? '--'}
        unit="%"
        status={metrics ? getStatus(metrics.imbalance_score, 15, 30) : 'healthy'}
      />
      <MetricCard
        label="Connection"
        value={useStore((s) => (s.isConnected ? 'Live' : 'Disconnected'))}
        status={useStore((s) => (s.isConnected ? 'healthy' : 'danger'))}
      />
    </div>
  );
}
