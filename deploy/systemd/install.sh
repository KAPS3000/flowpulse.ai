#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/flowpulse"
SYSTEMD_DIR="/etc/systemd/system"
BPF_DIR="/opt/flowpulse/bpf"

echo "=== FlowPulse Bare-Metal Installer ==="

# Create user if not exists
if ! id flowpulse &>/dev/null; then
    useradd --system --no-create-home --shell /bin/false flowpulse
    echo "Created flowpulse system user"
fi

# Create directories
mkdir -p "$CONFIG_DIR" "$BPF_DIR"

# Copy binaries
for bin in flowpulse-agent flowpulse-aggregator flowpulse-server fpctl; do
    if [ -f "bin/$bin" ]; then
        cp "bin/$bin" "$INSTALL_DIR/$bin"
        chmod 755 "$INSTALL_DIR/$bin"
        echo "Installed $bin"
    fi
done

# Copy BPF objects
if ls bpf/*.o &>/dev/null; then
    cp bpf/*.o "$BPF_DIR/"
    echo "Installed BPF objects"
fi

# Copy configs (don't overwrite existing)
for cfg in agent.yaml server.yaml; do
    if [ ! -f "$CONFIG_DIR/$cfg" ]; then
        cp "configs/$cfg" "$CONFIG_DIR/$cfg"
        echo "Installed $cfg"
    else
        echo "Skipping $cfg (already exists)"
    fi
done

# Install systemd units
cp deploy/systemd/flowpulse-agent.service "$SYSTEMD_DIR/"
cp deploy/systemd/flowpulse-aggregator.service "$SYSTEMD_DIR/"

systemctl daemon-reload
echo ""
echo "=== Installation Complete ==="
echo ""
echo "Next steps:"
echo "  1. Edit /etc/flowpulse/agent.yaml and /etc/flowpulse/server.yaml"
echo "  2. Set environment variables in /etc/flowpulse/env:"
echo "     FLOWPULSE_CLICKHOUSE_DSN=clickhouse://..."
echo "     FLOWPULSE_NATS_URL=nats://..."
echo "     FLOWPULSE_REDIS_ADDR=..."
echo "     FLOWPULSE_JWT_SECRET=<random-secret>"
echo "  3. Start services:"
echo "     sudo systemctl enable --now flowpulse-agent"
echo "     sudo systemctl enable --now flowpulse-aggregator"
