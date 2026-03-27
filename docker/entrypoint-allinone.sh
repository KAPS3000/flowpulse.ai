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

mkdir -p /etc/clickhouse-server /var/log/clickhouse-server

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
    <tmp_path>/var/lib/clickhouse/tmp/</tmp_path>
    <user_files_path>/var/lib/clickhouse/user_files/</user_files_path>
    <format_schema_path>/var/lib/clickhouse/format_schemas/</format_schema_path>
    <mark_cache_size>5368709120</mark_cache_size>
    <max_concurrent_queries>100</max_concurrent_queries>
    <profiles>
        <default>
            <max_memory_usage>10000000000</max_memory_usage>
        </default>
    </profiles>
    <users>
        <default>
            <password></password>
            <networks>
                <ip>::/0</ip>
            </networks>
            <profile>default</profile>
            <quota>default</quota>
            <access_management>1</access_management>
        </default>
    </users>
    <quotas>
        <default>
            <interval>
                <duration>3600</duration>
                <queries>0</queries>
                <errors>0</errors>
                <result_rows>0</result_rows>
                <read_rows>0</read_rows>
                <execution_time>0</execution_time>
            </interval>
        </default>
    </quotas>
</clickhouse>
XMLEOF

exec /usr/bin/supervisord -c /etc/supervisor/conf.d/flowpulse.conf
