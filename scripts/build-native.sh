#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

chmod +x "$ROOT_DIR/app.py" "$ROOT_DIR/procam-radar"
cp -f "$ROOT_DIR/app.py" "$ROOT_DIR/dist/linux/procam-radar-gui"
cp -f "$ROOT_DIR/procam-radar" "$ROOT_DIR/dist/linux/procam-radar"
chmod +x "$ROOT_DIR/dist/linux/procam-radar-gui" "$ROOT_DIR/dist/linux/procam-radar"

echo "[1/2] Сборка нативного бинарника Linux x64..."
(
  cd "$ROOT_DIR/dist/windows"
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "$ROOT_DIR/dist/linux/ProCamRadar_Linux_x64" .
)
chmod +x "$ROOT_DIR/dist/linux/ProCamRadar_Linux_x64"

echo "[2/2] Сборка нативного бинарника Windows x64 (.exe с встроенным манифестом и иконкой)..."
(
  cd "$ROOT_DIR/dist/windows"
  CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H=windowsgui" -o "$ROOT_DIR/dist/windows/ProCamRadar_Windows_x64.exe" .
)

echo "✔ Сборка успешно завершена:"
ls -lh "$ROOT_DIR/dist/linux/ProCamRadar_Linux_x64" "$ROOT_DIR/dist/windows/ProCamRadar_Windows_x64.exe"
