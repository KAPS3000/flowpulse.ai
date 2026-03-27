import { create } from 'zustand';
import type { Flow, TrainingMetrics, TopologyNode, Alert, AlertSummary } from './api';

interface FlowPulseState {
  flows: Flow[];
  totalFlowCount: number;
  trainingMetrics: TrainingMetrics | null;
  topologyNodes: TopologyNode[];
  isConnected: boolean;

  alerts: Alert[];
  alertSummary: AlertSummary | null;
  unacknowledgedCount: number;

  setFlows: (flows: Flow[], total: number) => void;
  addRealtimeFlow: (flow: Flow) => void;
  setTrainingMetrics: (metrics: TrainingMetrics) => void;
  setTopologyNodes: (nodes: TopologyNode[]) => void;
  setConnected: (connected: boolean) => void;

  setAlerts: (alerts: Alert[]) => void;
  addRealtimeAlert: (alert: Alert) => void;
  updateAlert: (alert: Alert) => void;
  setAlertSummary: (summary: AlertSummary) => void;
}

const MAX_REALTIME_FLOWS = 10000;
const MAX_ALERTS = 500;

export const useStore = create<FlowPulseState>((set) => ({
  flows: [],
  totalFlowCount: 0,
  trainingMetrics: null,
  topologyNodes: [],
  isConnected: false,
  alerts: [],
  alertSummary: null,
  unacknowledgedCount: 0,

  setFlows: (flows, total) => set({ flows, totalFlowCount: total }),

  addRealtimeFlow: (flow) =>
    set((state) => {
      const next = [flow, ...state.flows];
      if (next.length > MAX_REALTIME_FLOWS) next.length = MAX_REALTIME_FLOWS;
      return { flows: next };
    }),

  setTrainingMetrics: (metrics) => set({ trainingMetrics: metrics }),
  setTopologyNodes: (nodes) => set({ topologyNodes: nodes }),
  setConnected: (connected) => set({ isConnected: connected }),

  setAlerts: (alerts) =>
    set({
      alerts,
      unacknowledgedCount: alerts.filter((a) => !a.acknowledged && !a.resolved).length,
    }),

  addRealtimeAlert: (alert) =>
    set((state) => {
      const next = [alert, ...state.alerts];
      if (next.length > MAX_ALERTS) next.length = MAX_ALERTS;
      return {
        alerts: next,
        unacknowledgedCount: next.filter((a) => !a.acknowledged && !a.resolved).length,
      };
    }),

  updateAlert: (updated) =>
    set((state) => {
      const next = state.alerts.map((a) => (a.id === updated.id ? updated : a));
      return {
        alerts: next,
        unacknowledgedCount: next.filter((a) => !a.acknowledged && !a.resolved).length,
      };
    }),

  setAlertSummary: (summary) => set({ alertSummary: summary }),
}));
