#!/usr/bin/env bash
# ==============================================================================
# ProCam Radar v4.0 — Автоматический установщик для Linux (Ubuntu / Debian / Arch)
# Устанавливает программу, иконку Кибер-Совы и ярлык в меню приложений.
# ==============================================================================
set -euo pipefail

REPO_RAW="https://raw.githubusercontent.com/Yeeerniyaz/procam-radar/main"
REPO_REL="https://github.com/Yeeerniyaz/procam-radar/releases/latest/download"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-.}")" 2>/dev/null && pwd || pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." 2>/dev/null && pwd || pwd)"

echo "================================================================"
echo "  Установка ProCam Radar v4.0 (Engineering Suite for Linux)"
echo "================================================================"

if [[ $EUID -ne 0 ]]; then
  echo "[!] Запустите установщик с правами администратора: sudo bash install.sh"
  exit 1
fi

install -d /usr/local/bin /usr/share/pixmaps /usr/share/icons/hicolor/scalable/apps /usr/share/applications

# 1. Ядро CLI (procam-radar)
if [[ -f "${ROOT_DIR}/procam-radar" ]]; then
  install -m 0755 "${ROOT_DIR}/procam-radar" /usr/local/bin/procam-radar
elif [[ -f "${ROOT_DIR}/yeee" ]]; then
  install -m 0755 "${ROOT_DIR}/yeee" /usr/local/bin/procam-radar
else
  echo "[1/4] Скачивание ядра procam-radar из GitHub..."
  curl -fsSL "${REPO_RAW}/procam-radar" -o /usr/local/bin/procam-radar
  chmod 0755 /usr/local/bin/procam-radar
fi
ln -sf /usr/local/bin/procam-radar /usr/local/bin/yeee

# 2. Нативное графическое приложение (procam-radar-gui) и студия (ProCamRadar_Linux_x64)
if [[ -f "${SCRIPT_DIR}/procam-radar-gui" ]]; then
  install -m 0755 "${SCRIPT_DIR}/procam-radar-gui" /usr/local/bin/procam-radar-gui
else
  echo "[2/4] Скачивание графического интерфейса procam-radar-gui..."
  curl -fsSL "${REPO_RAW}/dist/linux/procam-radar-gui" -o /usr/local/bin/procam-radar-gui
  chmod 0755 /usr/local/bin/procam-radar-gui
fi
ln -sf /usr/local/bin/procam-radar-gui /usr/local/bin/yeee-gui

if [[ -f "${SCRIPT_DIR}/ProCamRadar_Linux_x64" ]]; then
  install -m 0755 "${SCRIPT_DIR}/ProCamRadar_Linux_x64" /usr/local/bin/ProCamRadar_Linux_x64
fi

# 3. Фирменная иконка Кибер-Совы (PNG + SVG)
if [[ -f "${ROOT_DIR}/assets/procam-radar-256.png" ]]; then
  install -m 0644 "${ROOT_DIR}/assets/procam-radar-256.png" /usr/share/pixmaps/procam-radar.png
else
  echo "[3/4] Скачивание иконки procam-radar.png..."
  curl -fsSL "${REPO_RAW}/assets/procam-radar-256.png" -o /usr/share/pixmaps/procam-radar.png
fi

if [[ -f "${ROOT_DIR}/assets/procam-radar.svg" ]]; then
  install -m 0644 "${ROOT_DIR}/assets/procam-radar.svg" /usr/share/icons/hicolor/scalable/apps/procam-radar.svg
else
  curl -fsSL "${REPO_RAW}/assets/procam-radar.svg" -o /usr/share/icons/hicolor/scalable/apps/procam-radar.svg || true
fi

# 4. Ярлык в меню приложений (.desktop)
cat > /usr/share/applications/procam-radar.desktop << 'EOF'
[Desktop Entry]
Version=4.0
Type=Application
Name=ProCam Radar
GenericName=Поиск и диагностика IP-камер
Comment=Инженерный графический комплекс поиска, диагностики и настройки IP-камер ProCam
Exec=/usr/local/bin/procam-radar-gui
Icon=procam-radar
Terminal=false
Categories=Network;Utility;Security;
StartupWMClass=ProCam Radar
EOF
chmod 0644 /usr/share/applications/procam-radar.desktop
update-desktop-database /usr/share/applications 2>/dev/null || true

echo "================================================================"
echo "  ✔ ProCam Radar v4.0 успешно установлен!"
echo "  • Запуск из меню приложений:  ProCam Radar (с иконкой совы)"
echo "  • Запуск из терминала (GUI):  procam-radar-gui"
echo "  • Запуск веб-студии:          ProCamRadar_Linux_x64"
echo "================================================================"
