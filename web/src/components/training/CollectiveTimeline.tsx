'use client';

import { useStore } from '@/lib/store';

interface CollectiveOp {
  id: string;
  nodeId: string;
  type: string;
  startMs: number;
  durationMs: number;
}

export function CollectiveTimeline() {
  const flows = useStore((s) => s.flows);

  // Derive a simplified Gantt chart from flow data
  // In production, the aggregator would tag flows with collective types
  const ops: CollectiveOp[] = flows.slice(0, 50).map((f, i) => ({
    id: f.flow_id || `op-${i}`,
    nodeId: f.node_id || `node-${i % 8}`,
    type: f.rdma ? 'all-reduce' : 'data-transfer',
    startMs: new Date(f.first_seen).getTime() || Date.now() - i * 100,
    durationMs: Math.max(
      new Date(f.last_seen).getTime() - new Date(f.first_seen).getTime(),
      10
    ),
  }));

  const uniqueNodes = Array.from(new Set(ops.map((o) => o.nodeId))).slice(0, 16);
  const minTime = Math.min(...ops.map((o) => o.startMs));
  const maxTime = Math.max(...ops.map((o) => o.startMs + o.durationMs));
  const timeRange = maxTime - minTime || 1;

  const typeColors: Record<string, string> = {
    'all-reduce': 'bg-blue-500',
    'all-gather': 'bg-purple-500',
    'reduce-scatter': 'bg-cyan-500',
    'data-transfer': 'bg-emerald-500',
  };

  if (ops.length === 0) {
    return (
      <div className="card text-center py-12 text-fp-muted">
        No collective operations detected yet.
      </div>
    );
  }

  return (
    <div className="card p-0 overflow-hidden">
      <div className="px-4 py-3 border-b border-fp-border flex items-center justify-between">
        <h2 className="text-sm font-semibold text-white">Collective Operation Timeline</h2>
        <div className="flex items-center gap-3 text-xs text-fp-muted">
          {Object.entries(typeColors).map(([type_, color]) => (
            <span key={type_} className="flex items-center gap-1">
              <span className={`w-2.5 h-2.5 rounded ${color}`} />
              {type_}
            </span>
          ))}
        </div>
      </div>
      <div className="p-4 overflow-x-auto">
        <div className="min-w-[600px]">
          {uniqueNodes.map((nodeId) => {
            const nodeOps = ops.filter((o) => o.nodeId === nodeId);
            return (
              <div key={nodeId} className="flex items-center gap-2 mb-1">
                <span className="w-20 text-xs font-mono text-fp-muted truncate shrink-0">
                  {nodeId.slice(-8)}
                </span>
                <div className="relative flex-1 h-5 bg-fp-bg rounded">
                  {nodeOps.map((op) => {
                    const left = ((op.startMs - minTime) / timeRange) * 100;
                    const width = Math.max((op.durationMs / timeRange) * 100, 0.5);
                    return (
                      <div
                        key={op.id}
                        className={`absolute top-0.5 h-4 rounded ${typeColors[op.type] || 'bg-gray-500'} opacity-80`}
                        style={{ left: `${left}%`, width: `${width}%` }}
                        title={`${op.type} (${op.durationMs}ms)`}
                      />
                    );
                  })}
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
