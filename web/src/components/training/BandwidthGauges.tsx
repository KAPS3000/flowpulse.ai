'use client';

import { useStore } from '@/lib/store';
import { formatBytes } from '@/lib/api';

export function BandwidthGauges() {
  const nodes = useStore((s) => s.topologyNodes);

  // Aggregate per-rail bandwidth data from nodes
  const rails = nodes.map((n) => ({
    nodeId: n.node_id,
    utilPct: n.ib_util_pct,
    txBps: n.tx_bytes,
    rxBps: n.rx_bytes,
  }));

  if (rails.length === 0) {
    return (
      <div className="card text-center py-12 text-fp-muted">
        No InfiniBand rail data available.
      </div>
    );
  }

  return (
    <div className="card p-0 overflow-hidden">
      <div className="px-4 py-3 border-b border-fp-border">
        <h2 className="text-sm font-semibold text-white">Bandwidth per Rail</h2>
      </div>
      <div className="p-4 grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3">
        {rails.slice(0, 16).map((rail) => {
          const angle = (rail.utilPct / 100) * 180;
          const color =
            rail.utilPct > 80 ? '#ef4444' :
            rail.utilPct > 60 ? '#f59e0b' :
            rail.utilPct > 30 ? '#3b82f6' : '#10b981';

          return (
            <div key={rail.nodeId} className="bg-fp-bg border border-fp-border rounded-lg p-3 text-center">
              {/* SVG gauge */}
              <svg viewBox="0 0 120 70" className="w-full h-auto mx-auto mb-2">
                {/* Background arc */}
                <path
                  d="M 10 60 A 50 50 0 0 1 110 60"
                  fill="none"
                  stroke="#1e293b"
                  strokeWidth="8"
                  strokeLinecap="round"
                />
                {/* Value arc */}
                <path
                  d={describeArc(60, 60, 50, 180, 180 + angle)}
                  fill="none"
                  stroke={color}
                  strokeWidth="8"
                  strokeLinecap="round"
                />
                <text x="60" y="55" textAnchor="middle" fill="white" fontSize="14" fontWeight="bold">
                  {rail.utilPct.toFixed(0)}%
                </text>
              </svg>
              <div className="text-xs font-mono text-fp-muted truncate">{rail.nodeId.slice(-8)}</div>
              <div className="text-xs text-fp-muted mt-1">
                TX {formatBytes(rail.txBps)} / RX {formatBytes(rail.rxBps)}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function describeArc(cx: number, cy: number, r: number, startAngle: number, endAngle: number): string {
  const start = polarToCartesian(cx, cy, r, endAngle);
  const end = polarToCartesian(cx, cy, r, startAngle);
  const largeArcFlag = endAngle - startAngle <= 180 ? '0' : '1';
  return `M ${start.x} ${start.y} A ${r} ${r} 0 ${largeArcFlag} 0 ${end.x} ${end.y}`;
}

function polarToCartesian(cx: number, cy: number, r: number, angleDeg: number) {
  const angleRad = ((angleDeg - 90) * Math.PI) / 180;
  return {
    x: cx + r * Math.cos(angleRad),
    y: cy + r * Math.sin(angleRad),
  };
}
