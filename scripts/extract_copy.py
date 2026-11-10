"""Download one Douyin video and transcribe speech entirely on this machine.

Uses the preserved expert client, FFmpeg and whisper.cpp with an existing local model.
Only the selected video's media is fetched. No audio is uploaded to an ASR API.
"""
from __future__ import annotations
import argparse
import asyncio
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile

from douyin_collector import load_credentials
from download_douyin import download_video, DownloadError

DEFAULT_MODEL = Path.home() / ".cache/whisper/ggml-large-v3-turbo-q5_0.bin"


class CopyError(Exception):
    pass


def tools(model):
    paths = {name: shutil.which(name) for name in ("ffmpeg", "ffprobe", "whisper-cli")}
    missing = [name for name, path in paths.items() if not path]
    if not Path(model).is_file():
        missing.append("本地 Whisper 模型")
    return paths, missing


def command(args, timeout, message):
    try:
        result = subprocess.run(args, capture_output=True, timeout=timeout)
    except (OSError, subprocess.TimeoutExpired):
        raise CopyError(message) from None
    if result.returncode != 0:
        # Upstream logs can contain signed media URLs or cookies. Never echo them.
        raise CopyError(message)
    return result.stdout


def cookie_file(credentials, path):
    cookie, ua, cookies = credentials
    lines = ["# Netscape HTTP Cookie File"]
    for name, value in cookies.items():
        lines.append(f".douyin.com\tTRUE\t/\tTRUE\t0\t{name}\t{value}")
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w") as f:
        f.write("\n".join(lines) + "\n")
    return ua


def profile_credentials(profile):
    """Read only this project's Chrome profile, without a visible login window."""
    if not Path(profile).is_dir():
        raise CopyError("尚未保存抖音登录，请先在连接抖音中登录一次")
    async def read():
        from playwright.async_api import async_playwright
        async with async_playwright() as pw:
            try:
                context = await pw.chromium.launch_persistent_context(
                    str(Path(profile).resolve()), channel="chrome", headless=True,
                    timeout=20000, args=["--no-first-run"])
            except Exception:
                raise CopyError("无法读取专用登录态；如采集或登录窗口正在使用，请等结束后重试，无需重新扫码") from None
            try:
                items = await asyncio.wait_for(context.cookies("https://www.douyin.com"), 5)
                page = context.pages[0] if context.pages else await context.new_page()
                ua = await asyncio.wait_for(page.evaluate("() => navigator.userAgent"), 5)
                cookies = {item["name"]: item["value"] for item in items}
                if not (cookies.get("sessionid") or cookies.get("sessionid_ss")):
                    raise CopyError("专用窗口没有保存的登录态，请先在连接抖音中登录一次")
                return "; ".join(f"{k}={v}" for k, v in cookies.items()), ua.replace("HeadlessChrome", "Chrome"), cookies
            finally:
                await context.close()
    return asyncio.run(read())


def download_credentials(args, path):
    credentials = load_credentials(args.credentials_file)
    if credentials is None:
        credentials = profile_credentials(args.profile)
    ua = cookie_file(credentials, path)
    return ["--cookies", str(path), "--user-agent", ua]


def extract(args):
    paths, missing = tools(args.model)
    if missing:
        raise CopyError("本机缺少：" + "、".join(missing))
    if not args.input and not re.fullmatch(r"https://www\.douyin\.com/video/\d{10,30}", args.url):
        raise CopyError("只支持已收录的抖音视频链接")
    with tempfile.TemporaryDirectory(prefix="automoney-copy-") as temp:
        folder = Path(temp)
        caption = ""
        if args.input:
            media = Path(args.input).resolve()
            if not media.is_file() or media.stat().st_size > 150 * 1024 * 1024:
                raise CopyError("本地媒体不存在或超过 150MB")
        else:
            credentials = load_credentials(args.credentials_file)
            if credentials is None:
                credentials = profile_credentials(args.profile)
            media = folder / 'source.mp4'
            try:
                downloaded = download_video(args.url, media, credentials, paths['ffprobe'])
            except DownloadError as exc:
                raise CopyError(str(exc)) from None
            caption = downloaded['caption']
        probe = command([paths["ffprobe"], "-v", "error", "-show_entries", "format=duration", "-of", "json", str(media)],
                        20, "无法读取视频/音频时长")
        try:
            duration = float(json.loads(probe)["format"]["duration"])
        except (ValueError, KeyError, TypeError):
            raise CopyError("视频/音频时长无效") from None
        if not 0 < duration <= 1800:
            raise CopyError("本地版单次提取支持 30 分钟以内的视频；未截断后冒充完整文案")
        audio = folder / "audio.wav"
        command([paths["ffmpeg"], "-nostdin", "-v", "error", "-i", str(media), "-vn", "-ac", "1", "-ar", "16000",
                 "-c:a", "pcm_s16le", str(audio)], 120, "音频提取失败，视频可能没有可用音轨")
        output = folder / "transcript"
        command([paths["whisper-cli"], "--model", str(Path(args.model).expanduser()), "--file", str(audio),
                 "--language", "zh", "--output-txt", "--output-srt", "--output-file", str(output), "--no-prints"],
                1500, "本机语音转写失败或超时，请检查 Whisper 模型与运行环境")
        text_file = output.with_suffix(".txt")
        srt_file = output.with_suffix(".srt")
        text = text_file.read_text().strip() if text_file.exists() else ""
        if not text:
            raise CopyError("未识别出可用口播；不以视频标题代替转写结果")
        return {"text": text, "srt": srt_file.read_text() if srt_file.exists() else "", "caption": caption,
                "duration": duration, "engine": "whisper.cpp / large-v3-turbo（本机）"}


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--doctor", action="store_true")
    p.add_argument("--url", default="")
    p.add_argument("--input", default="", help="Local development verification only; not accepted by HTTP API")
    p.add_argument("--credentials-file", default=str(Path(__file__).resolve().parents[1] / ".local/douyin-credentials.json"))
    p.add_argument("--profile", default=str(Path(__file__).resolve().parents[1] / ".local/douyin-profile"))
    p.add_argument("--model", default=str(DEFAULT_MODEL))
    args = p.parse_args()
    try:
        if args.doctor:
            _, missing = tools(args.model)
            print(json.dumps({"ready": not missing, "missing": missing, "engine": "whisper.cpp（本机）"}, ensure_ascii=False))
        else:
            print(json.dumps(extract(args), ensure_ascii=False))
    except CopyError as exc:
        print(json.dumps({"error": str(exc)}, ensure_ascii=False))
        sys.exit(1)
    except Exception:
        print(json.dumps({"error": "本地文案提取异常，请检查下载、音频与模型环境"}, ensure_ascii=False))
        sys.exit(1)


if __name__ == "__main__":
    main()
