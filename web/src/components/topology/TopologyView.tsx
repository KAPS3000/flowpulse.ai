'use client';

import { useStore } from '@/lib/store';
import { formatBytes } from '@/lib/api';

export function TopologyView() {
  const nodes = useStore((s) => s.topologyNodes);

  if (nodes.length === 0) {
    return (
      <div className="card text-center py-12 text-fp-muted">
        No topology data. Waiting for node metrics...
      </div>
    );
  }

  return (
    <div className="card p-0 overflow-hidden">
      <div className="px-4 py-3 border-b border-fp-border">
        <h2 className="text-sm font-semibold text-white">Cluster Topology ({nodes.length} nodes)</h2>
      </div>
      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6 gap-3 p-4">
        {nodes.map((node) => {
          const cpuColor =
            node.cpu_avg > 80 ? 'text-fp-danger' :
            node.cpu_avg > 60 ? 'text-fp-warning' : 'text-fp-success';
          const ibColor =
            node.ib_util_pct > 80 ? 'text-fp-danger' :
            node.ib_util_pct > 50 ? 'text-fp-warning' : 'text-fp-success';

          return (
            <div
              key={node.node_id}
              className="bg-fp-bg border border-fp-border rounded-lg p-3 hover:border-fp-accent transition-colors"
            >
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs font-mono text-fp-text truncate">{node.node_id}</span>
                <span className={node.status === 'healthy' ? 'badge-healthy' : 'badge-danger'}>
                  {node.status}
                </span>
              </div>
              <div className="space-y-1 text-xs">
                <div className="flex justify-between">
                  <span className="text-fp-muted">CPU</span>
                  <span className={cpuColor}>{node.cpu_avg.toFixed(1)}%</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-fp-muted">IB Util</span>
                  <span className={ibColor}>{node.ib_util_pct.toFixed(1)}%</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-fp-muted">TX</span>
                  <span className="text-fp-text">{formatBytes(node.tx_bytes)}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-fp-muted">RX</span>
                  <span className="text-fp-text">{formatBytes(node.rx_bytes)}</span>
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
