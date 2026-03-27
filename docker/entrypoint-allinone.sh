#!/usr/bin/env bash
set -e

echo "================================================"
echo "  FlowPulse All-in-One Container"
echo "  Dashboard:  http://localhost:3000"
echo "  API:        http://localhost:8080"
echo "  Aggregator: http://localhost:9092"
echo "================================================"
echo ""

mkdir -p /var/lib/clickhouse /var/lib/nats /var/lib/redis \
         /var/log/supervisor /var/run/redis \
         /etc/clickhouse-server

if [ ! -f /etc/clickhouse-server/config.xml ]; then
  cat > /etc/clickhouse-server/config.xml <<'XMLEOF'
<clickhouse>
    <logger>
        <level>warning</level>
        <log>/var/log/clickhouse-server/clickhouse-server.log</log>
        <errorlog>/var/log/clickhouse-server/clickhouse-server.err.log</errorlog>
    </logger>
    <http_port>8123</http_port>
    <tcp_port>9000</tcp_port>
    <path>/var/lib/clickhouse/</path>
    <mark_cache_size>5368709120</mark_cache_size>
    <max_concurrent_queries>100</max_concurrent_queries>
</clickhouse>
XMLEOF
fi

exec /usr/bin/supervisord -c /etc/supervisor/conf.d/flowpulse.conf
