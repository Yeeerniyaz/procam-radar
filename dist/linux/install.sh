#!/usr/bin/env bash
# ==============================================================================
# ProCam Radar v4.0 — Автоматический установщик для Linux (Ubuntu / Debian / Arch)
# Устанавливает основное приложение ProCamRadar_Linux_x64, иконку Кибер-Совы
# и ярлык в меню приложений.
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

install -d /usr/local/bin \
  /usr/share/pixmaps \
  /usr/share/icons/hicolor/64x64/apps \
  /usr/share/icons/hicolor/256x256/apps \
  /usr/share/icons/hicolor/512x512/apps \
  /usr/share/icons/hicolor/scalable/apps \
  /usr/share/applications

# Удаляем старые ярлыки от прежних версий
rm -f /usr/share/applications/yeee-cam.desktop

# 1. Ядро CLI (procam-radar)
if [[ -f "${ROOT_DIR}/procam-radar" ]]; then
  install -m 0755 "${ROOT_DIR}/procam-radar" /usr/local/bin/procam-radar
else
  echo "[1/4] Скачивание ядра procam-radar из GitHub..."
  curl -fsSL "${REPO_RAW}/procam-radar" -o /usr/local/bin/procam-radar
  chmod 0755 /usr/local/bin/procam-radar
fi
ln -sf /usr/local/bin/procam-radar /usr/local/bin/yeee

# 2. Главное графическое приложение (ProCamRadar_Linux_x64)
if [[ -f "${SCRIPT_DIR}/ProCamRadar_Linux_x64" ]]; then
  install -m 0755 "${SCRIPT_DIR}/ProCamRadar_Linux_x64" /usr/local/bin/ProCamRadar_Linux_x64
elif [[ -f "${ROOT_DIR}/dist/linux/ProCamRadar_Linux_x64" ]]; then
  install -m 0755 "${ROOT_DIR}/dist/linux/ProCamRadar_Linux_x64" /usr/local/bin/ProCamRadar_Linux_x64
else
  echo "[2/4] Скачивание графического приложения ProCamRadar_Linux_x64..."
  curl -fsSL "${REPO_REL}/ProCamRadar_Linux_x64" -o /usr/local/bin/ProCamRadar_Linux_x64
  chmod 0755 /usr/local/bin/ProCamRadar_Linux_x64
fi
ln -sf /usr/local/bin/ProCamRadar_Linux_x64 /usr/local/bin/procam-radar-gui
ln -sf /usr/local/bin/ProCamRadar_Linux_x64 /usr/local/bin/yeee-gui

# 3. Фирменная иконка Кибер-Совы (во все системные каталоги иконок)
if [[ -f "${ROOT_DIR}/assets/procam-radar-256.png" ]]; then
  install -m 0644 "${ROOT_DIR}/assets/procam-radar-256.png" /usr/share/pixmaps/procam-radar.png
  install -m 0644 "${ROOT_DIR}/assets/procam-radar-256.png" /usr/share/icons/hicolor/256x256/apps/procam-radar.png
else
  echo "[3/4] Скачивание иконки procam-radar.png..."
  curl -fsSL "${REPO_RAW}/assets/procam-radar-256.png" -o /usr/share/pixmaps/procam-radar.png
  cp -f /usr/share/pixmaps/procam-radar.png /usr/share/icons/hicolor/256x256/apps/procam-radar.png
fi

if [[ -f "${ROOT_DIR}/assets/procam-radar.png" ]]; then
  install -m 0644 "${ROOT_DIR}/assets/procam-radar.png" /usr/share/icons/hicolor/512x512/apps/procam-radar.png
fi
if [[ -f "${ROOT_DIR}/assets/procam-radar-64.png" ]]; then
  install -m 0644 "${ROOT_DIR}/assets/procam-radar-64.png" /usr/share/icons/hicolor/64x64/apps/procam-radar.png
fi
if [[ -f "${ROOT_DIR}/assets/procam-radar.svg" ]]; then
  install -m 0644 "${ROOT_DIR}/assets/procam-radar.svg" /usr/share/icons/hicolor/scalable/apps/procam-radar.svg
else
  curl -fsSL "${REPO_RAW}/assets/procam-radar.svg" -o /usr/share/icons/hicolor/scalable/apps/procam-radar.svg || true
fi
gtk-update-icon-cache -f -t /usr/share/icons/hicolor 2>/dev/null || true

# 4. Ярлык в меню приложений (.desktop) — запускает ProCamRadar_Linux_x64
cat > /usr/share/applications/procam-radar.desktop << 'EOF'
[Desktop Entry]
Version=4.0
Type=Application
Name=ProCam Radar
GenericName=Поиск и диагностика IP-камер
Comment=Инженерный графический комплекс поиска, диагностики и настройки IP-камер ProCam
Exec=/usr/local/bin/ProCamRadar_Linux_x64
Icon=/usr/share/pixmaps/procam-radar.png
Terminal=false
Categories=Network;Utility;Security;
StartupWMClass=ProCamRadar
EOF
chmod 0644 /usr/share/applications/procam-radar.desktop

for id in ProCamRadar 127.0.0.1 crx__127_0_0_1 chrome-http___127.0.0.1_8765_-Default; do
  cat > "/usr/share/applications/${id}.desktop" << EOF
[Desktop Entry]
Version=4.0
Type=Application
Name=ProCam Radar
Exec=/usr/local/bin/ProCamRadar_Linux_x64
Icon=/usr/share/pixmaps/procam-radar.png
Terminal=false
NoDisplay=true
StartupWMClass=${id}
EOF
  chmod 0644 "/usr/share/applications/${id}.desktop"
done

# 5. Обработчик двойного клика для бинарных файлов в GNOME Files (Nautilus)
cat > /usr/local/bin/procam-run-binary << 'EOF'
#!/usr/bin/env bash
TARGET="$1"
if [[ -f "$TARGET" ]]; then
  chmod +x "$TARGET" 2>/dev/null || true
  exec "$TARGET"
fi
EOF
chmod 0755 /usr/local/bin/procam-run-binary

cat > /usr/share/applications/procam-run-binary.desktop << 'EOF'
[Desktop Entry]
Type=Application
Name=Запуск приложения Linux (ProCam)
Exec=/usr/local/bin/procam-run-binary %f
Icon=/usr/share/pixmaps/procam-radar.png
Terminal=false
NoDisplay=true
MimeType=application/x-executable;application/x-pie-executable;
EOF
chmod 0644 /usr/share/applications/procam-run-binary.desktop
update-desktop-database /usr/share/applications 2>/dev/null || true

echo "================================================================"
echo "  ✔ ProCam Radar v4.0 успешно установлен!"
echo "  • Запуск из меню приложений:  ProCam Radar (с иконкой совы)"
echo "  • Запуск из терминала:        ProCamRadar_Linux_x64"
echo "================================================================"
