#!/usr/bin/env python3
"""UI-only CI smoke test for the native Awesome/XEmbed tray backend."""

from __future__ import annotations

import argparse
import os
import re
import signal
import subprocess
import tempfile
import time
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
AWESOME_CONFIG = ROOT / "internal/tray/testdata/awesome/rc.lua"
TIMEOUT = 10.0


def command_output(command: list[str], env: dict[str, str]) -> str:
    return subprocess.run(
        command,
        check=True,
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
    ).stdout


def wait_for(description: str, probe):
    deadline = time.monotonic() + TIMEOUT
    last_error: Exception | None = None
    while time.monotonic() < deadline:
        try:
            result = probe()
            if result:
                return result
        except (OSError, subprocess.SubprocessError) as error:
            last_error = error
        time.sleep(0.1)
    detail = f": {last_error}" if last_error else ""
    raise RuntimeError(f"timed out waiting for {description}{detail}")


def systray_window(env: dict[str, str]) -> str | None:
    tree = command_output(["xwininfo", "-root", "-tree"], env)
    match = re.search(r'^\s+(0x[0-9a-f]+) "Awesome systray window"', tree, re.MULTILINE)
    return match.group(1) if match else None


def embedded_icon(env: dict[str, str], systray: str) -> tuple[str, int, int] | None:
    tree = command_output(["xwininfo", "-id", systray, "-tree"], env)
    match = re.search(
        r'^\s+(0x[0-9a-f]+) "gswitch:[^"]*".*?\s+(\d+)x(\d+)[+-]',
        tree,
        re.MULTILINE,
    )
    if not match:
        return None
    return match.group(1), int(match.group(2)), int(match.group(3))


def visible_gswitch_windows(env: dict[str, str]) -> set[str]:
    result = subprocess.run(
        ["xdotool", "search", "--onlyvisible", "--class", "gswitch-tray"],
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        text=True,
        check=False,
    )
    return set(result.stdout.split())


def window_geometry(env: dict[str, str], window: str) -> tuple[int, int]:
    output = command_output(["xdotool", "getwindowgeometry", window], env)
    match = re.search(r"Geometry: (\d+)x(\d+)", output)
    if not match:
        raise RuntimeError(f"could not read geometry for window {window}")
    return int(match.group(1)), int(match.group(2))


def stop_process(process: subprocess.Popen, timeout: float = 5.0) -> int:
    if process.poll() is None:
        process.send_signal(signal.SIGTERM)
    try:
        return process.wait(timeout=timeout)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait(timeout=timeout)
        raise RuntimeError(f"process {process.args[0]} did not stop after SIGTERM")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--awesome", default="awesome")
    parser.add_argument("--tray", default=str(ROOT / "builds/gswitch-tray"))
    args = parser.parse_args()

    if "DISPLAY" not in os.environ:
        raise RuntimeError("DISPLAY is required; run this smoke under xvfb-run")

    env = os.environ.copy()
    env["NO_AT_BRIDGE"] = "1"
    version = command_output([args.awesome, "--version"], env).splitlines()[0]
    if "awesome v4.3" not in version:
        raise RuntimeError(f"expected Awesome 4.3, got: {version}")

    with tempfile.TemporaryDirectory(prefix="gswitch-awesome-smoke-") as temp_dir:
        temp = Path(temp_dir)
        awesome_log_path = temp / "awesome.log"
        tray_log_path = temp / "tray.log"
        awesome_env = env | {"HOME": temp_dir, "XDG_CONFIG_HOME": str(temp / "config")}
        tray_env = env | {
            "DBUS_SESSION_BUS_ADDRESS": f"unix:path={temp / 'no-session-bus'}",
            "G_DEBUG": "fatal-criticals",
            "XDG_CURRENT_DESKTOP": "awesome",
        }

        with awesome_log_path.open("w", encoding="utf-8") as awesome_log, tray_log_path.open(
            "w", encoding="utf-8"
        ) as tray_log:
            awesome = subprocess.Popen(
                [args.awesome, "-c", str(AWESOME_CONFIG)],
                env=awesome_env,
                stdout=awesome_log,
                stderr=subprocess.STDOUT,
                text=True,
            )
            tray: subprocess.Popen | None = None
            try:
                systray = wait_for("Awesome XEmbed host", lambda: systray_window(env))
                if awesome.poll() is not None:
                    raise RuntimeError(f"Awesome exited with code {awesome.returncode}")

                tray = subprocess.Popen(
                    [args.tray],
                    env=tray_env,
                    stdout=tray_log,
                    stderr=subprocess.STDOUT,
                    text=True,
                )

                icon, width, height = wait_for(
                    "mapped gswitch tray icon",
                    lambda: embedded_icon(env, systray),
                )
                if min(width, height) < 16:
                    raise RuntimeError(f"tray icon is too small: {width}x{height}")

                xembed_info = command_output(["xprop", "-id", icon, "_XEMBED_INFO"], env)
                if not re.search(r"=\s+0x1,\s+0x1\s*$", xembed_info):
                    raise RuntimeError(f"tray icon is not mapped: {xembed_info.strip()}")

                visible_before = visible_gswitch_windows(env)
                subprocess.run(
                    ["xdotool", "mousemove", "--sync", "--window", icon, "5", "5", "click", "3"],
                    check=True,
                    env=env,
                )

                def popup_menu() -> tuple[str, int, int] | None:
                    for window in visible_gswitch_windows(env) - visible_before:
                        menu_width, menu_height = window_geometry(env, window)
                        if menu_width >= 100 and menu_height >= 80:
                            return window, menu_width, menu_height
                    return None

                menu, menu_width, menu_height = wait_for("XEmbed popup menu", popup_menu)
                # Mapping the popup precedes its keyboard grab on a busy CI
                # runner. Focus it explicitly before driving the menu so the
                # key sequence cannot land on Awesome's no-input window.
                subprocess.run(
                    ["xdotool", "windowfocus", "--sync", menu],
                    check=True,
                    env=env,
                )
                subprocess.run(
                    ["xdotool", "key", "Home", "Down", "Down", "Return"],
                    check=True,
                    env=env,
                )

                def settings_window() -> str | None:
                    tree = command_output(["xwininfo", "-root", "-tree"], env)
                    match = re.search(
                        r'^\s+(0x[0-9a-f]+) "gswitch - Settings".*?\s+(\d+)x(\d+)[+-]',
                        tree,
                        re.MULTILINE,
                    )
                    if not match or min(int(match.group(2)), int(match.group(3))) < 100:
                        return None
                    return match.group(1)

                wait_for("settings window opened from the XEmbed menu", settings_window)

                return_code = stop_process(tray)
                tray = None
                tray_log.flush()
                log_text = tray_log_path.read_text(encoding="utf-8")
                if return_code != 0:
                    raise RuntimeError(f"gswitch-tray exited with code {return_code}\n{log_text}")
                if "Using XEmbed system tray backend" not in log_text:
                    raise RuntimeError(f"XEmbed backend was not selected\n{log_text}")
                if (
                    "Tray application exited" not in log_text
                    or "SIGSEGV" in log_text
                    or "CRITICAL" in log_text
                ):
                    raise RuntimeError(f"gswitch-tray did not exit cleanly\n{log_text}")

                print(
                    f"{version}: icon {width}x{height}, menu {menu_width}x{menu_height}, "
                    "settings opened, clean exit"
                )
            except Exception:
                tray_log.flush()
                awesome_log.flush()
                print(f"--- gswitch-tray log ---\n{tray_log_path.read_text(encoding='utf-8')}")
                print(f"--- Awesome log ---\n{awesome_log_path.read_text(encoding='utf-8')}")
                raise
            finally:
                if tray is not None:
                    stop_process(tray)
                stop_process(awesome)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
