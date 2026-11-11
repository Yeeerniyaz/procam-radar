#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
ProCam Radar v5.0 — Нативное графическое приложение (GTK3 + WebKit2 Desktop Shell)
Запускает локальное ядро ProCamRadar_Linux_x64 в режиме --headless и открывает
полноценное десктопное окно GTK3/WebKit2 с темной темой и фирменной иконкой.
"""

import os
import sys
import time
import signal
import socket
import subprocess
import urllib.request

import gi
gi.require_version("Gtk", "3.0")
try:
    gi.require_version("WebKit2", "4.1")
except ValueError:
    gi.require_version("WebKit2", "4.0")
from gi.repository import Gtk, WebKit2, Gdk, GdkPixbuf, GLib


def find_free_port(preferred: int = 8765) -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        if s.connect_ex(("127.0.0.1", preferred)) != 0:
            return preferred
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def find_radar_binary() -> str:
    script_dir = os.path.dirname(os.path.abspath(__file__))
    candidates = [
        os.path.join(script_dir, "ProCamRadar_Linux_x64"),
        os.path.join(script_dir, "dist", "linux", "ProCamRadar_Linux_x64"),
        os.path.join(script_dir, "..", "linux", "ProCamRadar_Linux_x64"),
        "/home/yerniyaz/Desktop/projects/procam-radar/dist/linux/ProCamRadar_Linux_x64",
        "/usr/local/bin/ProCamRadar_Linux_x64",
    ]
    for path in candidates:
        if os.path.isfile(path) and os.access(path, os.X_OK):
            return os.path.abspath(path)
    return ""


def find_icon_path() -> str:
    script_dir = os.path.dirname(os.path.abspath(__file__))
    candidates = [
        os.path.join(script_dir, "dist", "windows", "icon.png"),
        os.path.join(script_dir, "..", "windows", "icon.png"),
        os.path.join(script_dir, "icon.png"),
        "/home/yerniyaz/Desktop/projects/procam-radar/dist/windows/icon.png",
    ]
    for path in candidates:
        if os.path.isfile(path):
            return os.path.abspath(path)
    return ""


def wait_for_server(url: str, timeout_sec: float = 5.0) -> bool:
    deadline = time.time() + timeout_sec
    while time.time() < deadline:
        try:
            with urllib.request.urlopen(url + "/api/info", timeout=0.4) as resp:
                if resp.status == 200:
                    return True
        except Exception:
            time.sleep(0.1)
    return False


def main() -> int:
    bin_path = find_radar_binary()
    if not bin_path:
        sys.stderr.write("Ошибка: не найден исполняемый файл ProCamRadar_Linux_x64\n")
        return 1

    port = find_free_port(8765)
    url = f"http://127.0.0.1:{port}"

    proc = subprocess.Popen(
        [bin_path, "--headless", "--port", str(port)],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )

    if not wait_for_server(url, timeout_sec=6.0):
        proc.terminate()
        sys.stderr.write("Ошибка: локальный сервер ProCam Radar не ответил.\n")
        return 1

    settings = Gtk.Settings.get_default()
    if settings:
        settings.set_property("gtk-application-prefer-dark-theme", True)

    win = Gtk.Window(title="ProCam Radar v5.0 — Инженерный комплекс IP-камер и NVR")
    win.set_default_size(1440, 880)
    win.set_position(Gtk.WindowPosition.CENTER)
    win.set_wmclass("procam-radar", "ProCam Radar")

    icon_path = find_icon_path()
    if icon_path:
        try:
            pixbuf = GdkPixbuf.Pixbuf.new_from_file_at_size(icon_path, 256, 256)
            win.set_icon(pixbuf)
        except Exception:
            pass

    webview = WebKit2.WebView()
    ws = webview.get_settings()
    ws.set_enable_developer_extras(False)
    ws.set_enable_write_console_messages_to_stdout(False)
    webview.set_background_color(Gdk.RGBA(0.035, 0.039, 0.055, 1.0))
    webview.load_uri(url)

    def on_decide_policy(view, decision, decision_type):
        if decision_type == WebKit2.PolicyDecisionType.NEW_WINDOW_ACTION:
            decision.ignore()
            return True
        return False

    webview.connect("decide-policy", on_decide_policy)
    win.add(webview)

    def cleanup(*args):
        try:
            urllib.request.urlopen(url + "/api/quit", timeout=0.35)
        except Exception:
            pass
        if proc.poll() is None:
            proc.terminate()
            try:
                proc.wait(timeout=1.0)
            except Exception:
                proc.kill()
        Gtk.main_quit()

    win.connect("destroy", cleanup)
    GLib.unix_signal_add(GLib.PRIORITY_DEFAULT, signal.SIGINT, cleanup)
    GLib.unix_signal_add(GLib.PRIORITY_DEFAULT, signal.SIGTERM, cleanup)

    win.show_all()
    Gtk.main()
    return 0


if __name__ == "__main__":
    sys.exit(main())
