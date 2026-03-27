'use client';

import { FlowTable } from '@/components/flows/FlowTable';
import { useWebSocket } from '@/hooks/useWebSocket';
import { useDataLoader } from '@/hooks/useDataLoader';

export default function FlowsPage() {
  useWebSocket('default');
  useDataLoader();

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold text-white mb-1">Network Flows</h2>
        <p className="text-sm text-fp-muted">Live east-west traffic flows with RDMA metadata</p>
      </div>
      <FlowTable />
    </div>
  );
}
