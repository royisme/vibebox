#!/usr/bin/env bash
set -euo pipefail

VERSION_TAG="${1:-dev}"
OUT_DIR="${2:-dist}"
VERSION="${VERSION_TAG#v}"

mkdir -p "${OUT_DIR}"
rm -f "${OUT_DIR}"/vibebox_"${VERSION}"_*.tar.gz "${OUT_DIR}/checksums.txt"

targets=(
  "darwin/arm64"
  "darwin/amd64"
  "linux/arm64"
  "linux/amd64"
)

for target in "${targets[@]}"; do
  IFS="/" read -r goos goarch <<<"${target}"
  stage_dir="$(mktemp -d)"
  bin_path="${stage_dir}/vibebox"
  archive_name="vibebox_${VERSION}_${goos}_${goarch}.tar.gz"

  cgo_enabled=0
  if [[ "${goos}" == "darwin" ]]; then
    cgo_enabled=1
  fi

  GOOS="${goos}" GOARCH="${goarch}" CGO_ENABLED="${cgo_enabled}" \
    go build -trimpath -ldflags="-s -w" -o "${bin_path}" ./cmd/vibebox

  tar -C "${stage_dir}" -czf "${OUT_DIR}/${archive_name}" vibebox
  rm -rf "${stage_dir}"
done

(
  cd "${OUT_DIR}"
  shasum -a 256 vibebox_"${VERSION}"_*.tar.gz > checksums.txt
)

echo "release artifacts generated under ${OUT_DIR}/"
