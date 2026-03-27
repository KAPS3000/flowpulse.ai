'use client';

import { useEffect, useRef } from 'react';
import { api } from '@/lib/api';
import { useStore } from '@/lib/store';

const TOKEN = 'demo';

export function useDataLoader() {
  const setFlows = useStore((s) => s.setFlows);
  const setTrainingMetrics = useStore((s) => s.setTrainingMetrics);
  const setTopologyNodes = useStore((s) => s.setTopologyNodes);
  const setAlerts = useStore((s) => s.setAlerts);
  const setAlertSummary = useStore((s) => s.setAlertSummary);
  const started = useRef(false);

  useEffect(() => {
    if (started.current) return;
    started.current = true;

    async function load() {
      try {
        const [flowsRes, topoRes, tmRes, alertsRes, summaryRes] = await Promise.all([
          api.getFlows(TOKEN, { limit: '500' }).catch(() => null),
          api.getTopology(TOKEN).catch(() => null),
          api.getTrainingMetrics(TOKEN).catch(() => null),
          api.getAlerts(TOKEN).catch(() => null),
          api.getAlertSummary(TOKEN).catch(() => null),
        ]);
        if (flowsRes) setFlows(flowsRes.flows, flowsRes.total_count);
        if (topoRes) setTopologyNodes(topoRes.nodes);
        if (tmRes) setTrainingMetrics(tmRes);
        if (alertsRes) setAlerts(alertsRes.alerts);
        if (summaryRes) setAlertSummary(summaryRes);
      } catch {
        // mock server may not be running yet
      }
    }

    load();
    const interval = setInterval(load, 3000);
    return () => clearInterval(interval);
  }, [setFlows, setTrainingMetrics, setTopologyNodes, setAlerts, setAlertSummary]);
}
