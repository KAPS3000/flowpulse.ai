'use client';

import { TrainingDashboard } from '@/components/training/TrainingDashboard';
import { CollectiveTimeline } from '@/components/training/CollectiveTimeline';
import { StragglerLeaderboard } from '@/components/training/StragglerLeaderboard';
import { BandwidthGauges } from '@/components/training/BandwidthGauges';
import { useWebSocket } from '@/hooks/useWebSocket';
import { useDataLoader } from '@/hooks/useDataLoader';

export default function TrainingPage() {
  useWebSocket('default');
  useDataLoader();

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold text-white mb-1">Training Efficiency</h2>
        <p className="text-sm text-fp-muted">
          Metrics that impact distributed training throughput
        </p>
      </div>
      <TrainingDashboard />
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <StragglerLeaderboard />
        <BandwidthGauges />
      </div>
      <CollectiveTimeline />
    </div>
  );
}
