'use client';

import { AlertPanel } from '@/components/alerts/AlertPanel';
import { useWebSocket } from '@/hooks/useWebSocket';
import { useDataLoader } from '@/hooks/useDataLoader';

export default function AlertsPage() {
  useWebSocket('default');
  useDataLoader();

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold text-white mb-1">Alerts & Monitoring</h2>
        <p className="text-sm text-fp-muted">
          Threshold-based alerts with runbooks and email notifications
        </p>
      </div>
      <AlertPanel />
    </div>
  );
}
