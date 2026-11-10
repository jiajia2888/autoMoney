"""Install/manage the local macOS login service. Never stores credentials."""
import argparse
import os
from pathlib import Path
import plistlib
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
LABEL = "com.automoney.local"
PLIST = Path.home() / "Library/LaunchAgents" / (LABEL + ".plist")
DOMAIN = "gui/" + str(os.getuid())


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("action", choices=["install", "stop", "status", "uninstall"])
    args = parser.parse_args()
    if sys.platform != "darwin":
        parser.error("此服务管理脚本仅支持 macOS；其他系统请使用 make run")
    if args.action == "status":
        return subprocess.call(["launchctl", "print", DOMAIN + "/" + LABEL])
    if args.action in ("stop", "uninstall"):
        subprocess.run(["launchctl", "bootout", DOMAIN + "/" + LABEL], check=False)
        if args.action == "uninstall":
            PLIST.unlink(missing_ok=True)
        return 0
    binary = ROOT / "HzTechGoBackService/bin/autoMoney"
    if not binary.is_file():
        parser.error("请先 make build")
    # Never terminate an unrelated listener or run a second scheduler.
    import socket
    with socket.socket() as sock:
        if sock.connect_ex(("127.0.0.1", 9010)) == 0:
            parser.error("9010 已被占用；请先停止现有服务，再安装")
    logs = ROOT / ".local/logs"
    logs.mkdir(parents=True, exist_ok=True, mode=0o700)
    PLIST.parent.mkdir(parents=True, exist_ok=True)
    config = {
        "Label": LABEL,
        "ProgramArguments": [str(binary)],
        "WorkingDirectory": str(ROOT / "HzTechGoBackService"),
        "EnvironmentVariables": {"PATH": "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"},
        "RunAtLoad": True, "KeepAlive": True, "ThrottleInterval": 10,
        "StandardOutPath": str(logs / "service.log"),
        "StandardErrorPath": str(logs / "service-error.log"),
    }
    with PLIST.open("wb") as f:
        plistlib.dump(config, f)
    PLIST.chmod(0o600)
    subprocess.run(["launchctl", "bootout", DOMAIN + "/" + LABEL], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    subprocess.run(["launchctl", "bootstrap", DOMAIN, str(PLIST)], check=True)
    print("本机服务已安装：登录后自动启动，退出后自动重启；日志位于 .local/logs")
    return 0


if __name__ == "__main__":
    sys.exit(main())
