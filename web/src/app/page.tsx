'use client';

import { Overview } from '@/components/dashboard/Overview';
import { FlowTable } from '@/components/flows/FlowTable';
import { useWebSocket } from '@/hooks/useWebSocket';
import { useDataLoader } from '@/hooks/useDataLoader';

export default function DashboardPage() {
  useWebSocket('default');
  useDataLoader();

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold text-white mb-1">Dashboard</h2>
        <p className="text-sm text-fp-muted">Real-time training cluster overview</p>
      </div>
      <Overview />
      <FlowTable />
    </div>
  );
}
