#!/usr/bin/env bash
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive

# Minimal baseline for Mozi/vibebox execution environments.
APT_PACKAGES=(
  ca-certificates
  curl
  git
  bash
  ripgrep
  jq
  unzip
)

apt-get update
apt-get install -y --no-install-recommends "${APT_PACKAGES[@]}"
apt-get clean
rm -rf /var/lib/apt/lists/*

cat >/etc/profile.d/vibebox-sandbox.sh <<'EOF'
export IS_SANDBOX=1
EOF
chmod 0644 /etc/profile.d/vibebox-sandbox.sh

if command -v hostnamectl >/dev/null 2>&1; then
  hostnamectl set-hostname vibebox || true
fi

if [[ "${VIBEBOX_PROVISION_POWEROFF:-1}" == "1" ]]; then
  poweroff
fi
