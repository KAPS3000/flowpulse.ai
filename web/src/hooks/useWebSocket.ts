'use client';

import { useEffect, useRef } from 'react';
import { WSManager } from '@/lib/websocket';
import { useStore } from '@/lib/store';
import type { Flow, TrainingMetrics, TopologyNode, Alert } from '@/lib/api';

export function useWebSocket(tenantId: string) {
  const wsRef = useRef<WSManager | null>(null);
  const addFlow = useStore((s) => s.addRealtimeFlow);
  const setTrainingMetrics = useStore((s) => s.setTrainingMetrics);
  const setTopologyNodes = useStore((s) => s.setTopologyNodes);
  const setConnected = useStore((s) => s.setConnected);
  const addRealtimeAlert = useStore((s) => s.addRealtimeAlert);
  const updateAlert = useStore((s) => s.updateAlert);

  useEffect(() => {
    if (!tenantId) return;

    const ws = new WSManager(tenantId);
    wsRef.current = ws;

    ws.subscribe((msg) => {
      if (typeof msg.subject !== 'string' || !msg.data) return;

      if (msg.subject.endsWith('.flows')) {
        const batch = msg.data as { flows?: Flow[] };
        if (batch.flows) {
          batch.flows.forEach((f) => addFlow(f));
        }
      } else if (msg.subject.endsWith('.training')) {
        setTrainingMetrics(msg.data as TrainingMetrics);
      } else if (msg.subject.endsWith('.metrics')) {
        setTopologyNodes(msg.data as TopologyNode[]);
        setConnected(true);
      } else if (msg.subject.endsWith('.alert')) {
        addRealtimeAlert(msg.data as Alert);
      } else if (msg.subject.endsWith('.alert_ack') || msg.subject.endsWith('.alert_resolved')) {
        updateAlert(msg.data as Alert);
      }
    });

    ws.connect();
    setConnected(true);

    return () => {
      ws.disconnect();
      setConnected(false);
    };
  }, [tenantId, addFlow, setTrainingMetrics, setTopologyNodes, setConnected, addRealtimeAlert, updateAlert]);
}
