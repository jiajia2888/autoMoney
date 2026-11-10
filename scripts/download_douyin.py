"""Download an authorized Douyin video using the preserved expert client.

Credentials are sent only to the official API. Media requests use no Cookie.
A file is published only after bounded streaming and ffprobe verification.
"""
from __future__ import annotations
import argparse
import asyncio
import contextlib
import importlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
from urllib.parse import urlsplit

from douyin_collector import load_credentials, load_expert_client, CookieSigningContext, ROOT

MAX_BYTES = 150 * 1024 * 1024

class DownloadError(Exception):
    pass


def video_id(url):
    match = re.fullmatch(r'https://www\.douyin\.com/video/(\d{10,30})', url)
    if not match:
        raise DownloadError('只支持抖音官方视频链接')
    return match[1]


def media_url_allowed(url):
    try:
        parsed = urlsplit(url)
        host = parsed.hostname or ''
        return (parsed.scheme == 'https' and not parsed.username and not parsed.password
                and parsed.port in (None, 443) and any(host == root or host.endswith('.' + root)
                for root in ('douyin.com', 'douyinvod.com', 'douyincdn.com', 'bytecdn.cn', 'byteimg.com')))
    except ValueError:
        return False


def inspect_media(path, ffprobe):
    try:
        result = subprocess.run([ffprobe, '-v', 'error', '-show_entries',
            'format=duration:stream=codec_type,codec_name,width,height', '-of', 'json', str(path)],
            capture_output=True, timeout=20)
        data = json.loads(result.stdout)
        duration = float(data['format']['duration'])
        if result.returncode or not any(s.get('codec_type') == 'video' for s in data.get('streams', [])):
            raise ValueError()
        if not 0 < duration <= 1800:
            raise DownloadError('下载支持30分钟以内的视频，未截断文件')
        return data
    except DownloadError:
        raise
    except (OSError, ValueError, KeyError, TypeError, subprocess.TimeoutExpired):
        raise DownloadError('下载文件未通过视频完整性检查') from None


async def stream_media(client, url, target):
    for _ in range(6):
        if not media_url_allowed(url):
            raise DownloadError('媒体地址不是允许的抖音HTTPS地址')
        async with client.stream('GET', url) as response:
            if response.status_code in (301, 302, 303, 307, 308):
                from urllib.parse import urljoin
                url = urljoin(url, response.headers.get('location', ''))
                continue
            if response.status_code != 200:
                raise DownloadError('抖音媒体请求失败，地址可能过期或访问受限')
            kind = response.headers.get('content-type', '').split(';')[0].lower()
            if kind not in ('video/mp4', 'application/octet-stream', 'binary/octet-stream', 'application/mp4'):
                raise DownloadError('媒体接口未返回MP4视频')
            length = response.headers.get('content-length')
            if length is not None and (not length.isdigit() or int(length) > MAX_BYTES):
                raise DownloadError('媒体大小无效或超过150MB')
            size = 0
            with target.open('wb') as output:
                async for chunk in response.aiter_bytes():
                    size += len(chunk)
                    if size > MAX_BYTES:
                        raise DownloadError('视频超过150MB，未保存为完成文件')
                    output.write(chunk)
            if size == 0 or (length is not None and size != int(length)):
                raise DownloadError('媒体文件不完整')
            return size
    raise DownloadError('媒体重定向次数过多')


async def _download(url, destination, credentials, ffprobe):
    import httpx
    vid = video_id(url)
    cls = load_expert_client()
    cookie, ua, cookies = credentials
    api = cls(headers={'User-Agent': ua, 'Origin': 'https://www.douyin.com',
              'Referer': url, 'Cookie': cookie}, playwright_page=CookieSigningContext(cookies),
              cookie_dict=cookies, timeout=30)
    detail = await api.get_video_by_id(vid)
    if not isinstance(detail, dict) or str(detail.get('aweme_id')) != vid or detail.get('images'):
        raise DownloadError('专家详情接口未返回目标视频，可能需要验证或作品不可访问')
    extractor = importlib.import_module('media_platform.douyin.media')
    candidates = extractor.extract_video_urls(detail)
    if not candidates:
        raise DownloadError('专家详情接口未返回视频媒体地址')
    destination.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='.douyin-part-', dir=destination.parent) as temp:
        part = Path(temp) / 'video.part'
        # No Cookie, authorization or credential-bearing redirect follows here.
        async with httpx.AsyncClient(timeout=30, follow_redirects=False,
                headers={'User-Agent': ua, 'Referer': 'https://www.douyin.com/', 'Accept-Encoding': 'identity'}) as client:
            for candidate in candidates[:3]:
                try:
                    size = await stream_media(client, candidate, part)
                    probe = inspect_media(part, ffprobe)
                    if destination.exists():
                        raise DownloadError('目标视频文件已存在，请选择其他输出路径')
                    os.link(part, destination)  # Atomic publish, cannot overwrite existing work.
                    return {'file': str(destination), 'platform_id': vid, 'bytes': size,
                            'caption': str(detail.get('desc') or '')[:20000], 'probe': probe}
                except (DownloadError, httpx.HTTPError):
                    part.unlink(missing_ok=True)
    raise DownloadError('专家媒体地址下载或视频校验失败，未生成完成文件')


def download_video(url, destination, credentials, ffprobe=None):
    video_id(url)
    ffprobe = ffprobe or shutil.which('ffprobe')
    if not ffprobe:
        raise DownloadError('缺少ffprobe，无法验证下载文件')
    try:
        return asyncio.run(asyncio.wait_for(_download(url, Path(destination).resolve(), credentials, ffprobe), 180))
    except DownloadError:
        raise
    except Exception:
        raise DownloadError('专家下载链路请求失败或超时，请核对抖音访问状态') from None


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--url', required=True)
    parser.add_argument('--output-dir', default=str(ROOT / '.local/downloads/douyin'))
    parser.add_argument('--credentials-file', default=str(ROOT / '.local/douyin-credentials.json'))
    parser.add_argument('--profile', default=str(ROOT / '.local/douyin-profile'))
    args = parser.parse_args()
    try:
        vid = video_id(args.url)
        credentials = load_credentials(args.credentials_file)
        if credentials is None:
            from extract_copy import profile_credentials
            credentials = profile_credentials(args.profile)
        with contextlib.redirect_stdout(sys.stderr):
            result = download_video(args.url, Path(args.output_dir) / (vid + '.mp4'), credentials)
        print(json.dumps(result, ensure_ascii=False))
    except (DownloadError, ValueError) as exc:
        print(json.dumps({'error': str(exc)}, ensure_ascii=False))
        sys.exit(1)

if __name__ == '__main__':
    main()
