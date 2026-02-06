#!/usr/bin/env bash
set -euo pipefail

REPO="${REPO:-royisme/vibebox}"
VERSION="${VERSION:-latest}"
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"
SKIP_VERIFY="${SKIP_VERIFY:-0}"
SIGN_MACOS="${SIGN_MACOS:-1}"

usage() {
  cat <<'EOF'
install-vibebox.sh options:
  --repo <owner/name>      GitHub repository (default: royisme/vibebox)
  --version <vX.Y.Z|latest>
  --bin-dir <path>         Install directory (default: ~/.local/bin)
  --skip-verify            Skip checksum verification
  --no-sign-macos          Disable ad-hoc signing on macOS
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo)
      REPO="$2"
      shift 2
      ;;
    --version)
      VERSION="$2"
      shift 2
      ;;
    --bin-dir)
      BIN_DIR="$2"
      shift 2
      ;;
    --skip-verify)
      SKIP_VERIFY=1
      shift
      ;;
    --no-sign-macos)
      SIGN_MACOS=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
done

uname_s="$(uname -s)"
uname_m="$(uname -m)"

case "${uname_s}" in
  Darwin) os="darwin" ;;
  Linux) os="linux" ;;
  *)
    echo "unsupported OS: ${uname_s}" >&2
    exit 1
    ;;
esac

case "${uname_m}" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *)
    echo "unsupported architecture: ${uname_m}" >&2
    exit 1
    ;;
esac

if [[ "${VERSION}" == "latest" ]]; then
  VERSION="$(
    curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
      | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' \
      | head -n 1
  )"
  if [[ -z "${VERSION}" ]]; then
    echo "failed to resolve latest release from ${REPO}" >&2
    exit 1
  fi
fi

if [[ "${VERSION}" != v* ]]; then
  VERSION="v${VERSION}"
fi
version_no_v="${VERSION#v}"

asset="vibebox_${version_no_v}_${os}_${arch}.tar.gz"
base_url="https://github.com/${REPO}/releases/download/${VERSION}"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

archive_path="${tmp_dir}/${asset}"
checksums_path="${tmp_dir}/checksums.txt"

echo "downloading ${asset} from ${REPO}@${VERSION}"
curl -fsSL "${base_url}/${asset}" -o "${archive_path}"
curl -fsSL "${base_url}/checksums.txt" -o "${checksums_path}"

if [[ "${SKIP_VERIFY}" != "1" ]]; then
  if command -v shasum >/dev/null 2>&1; then
    expected="$(grep " ${asset}\$" "${checksums_path}" | awk '{print $1}')"
    actual="$(shasum -a 256 "${archive_path}" | awk '{print $1}')"
  elif command -v sha256sum >/dev/null 2>&1; then
    expected="$(grep " ${asset}\$" "${checksums_path}" | awk '{print $1}')"
    actual="$(sha256sum "${archive_path}" | awk '{print $1}')"
  else
    echo "warning: no shasum/sha256sum found, skipping checksum verification" >&2
    expected=""
    actual=""
  fi

  if [[ -n "${expected}" && "${expected}" != "${actual}" ]]; then
    echo "checksum mismatch for ${asset}" >&2
    exit 1
  fi
fi

mkdir -p "${BIN_DIR}"
tar -xzf "${archive_path}" -C "${tmp_dir}"
install -m 0755 "${tmp_dir}/vibebox" "${BIN_DIR}/vibebox"

if [[ "${os}" == "darwin" && "${SIGN_MACOS}" == "1" ]]; then
  if command -v codesign >/dev/null 2>&1; then
    entitlements_path="${tmp_dir}/vibebox.entitlements.plist"
    cat >"${entitlements_path}" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>com.apple.security.virtualization</key>
	<true/>
</dict>
</plist>
EOF
    codesign --force --sign - --entitlements "${entitlements_path}" "${BIN_DIR}/vibebox"
  else
    echo "warning: codesign not found, apple-vm may be unavailable until signed" >&2
  fi
fi

echo "installed vibebox to ${BIN_DIR}/vibebox"
echo "ensure ${BIN_DIR} is in PATH"
