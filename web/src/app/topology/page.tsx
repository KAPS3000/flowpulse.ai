'use client';

import { TopologyHeatmap } from '@/components/topology/TopologyHeatmap';
import { TopologyView } from '@/components/topology/TopologyView';
import { useWebSocket } from '@/hooks/useWebSocket';
import { useDataLoader } from '@/hooks/useDataLoader';

export default function TopologyPage() {
  useWebSocket('default');
  useDataLoader();

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold text-white mb-1">Cluster Topology</h2>
        <p className="text-sm text-fp-muted">Node health, CPU utilization, and InfiniBand link status</p>
      </div>
      <TopologyHeatmap />
      <TopologyView />
    </div>
  );
}
