'use client';

interface MetricCardProps {
  label: string;
  value: string | number;
  unit?: string;
  trend?: 'up' | 'down' | 'neutral';
  status?: 'healthy' | 'warning' | 'danger';
}

export function MetricCard({ label, value, unit, status = 'healthy' }: MetricCardProps) {
  const statusColor = {
    healthy: 'text-fp-success',
    warning: 'text-fp-warning',
    danger: 'text-fp-danger',
  }[status];

  return (
    <div className="card">
      <p className="metric-label">{label}</p>
      <div className="mt-2 flex items-baseline gap-2">
        <span className={`metric-value ${statusColor}`}>{value}</span>
        {unit && <span className="text-sm text-fp-muted">{unit}</span>}
      </div>
    </div>
  );
}
