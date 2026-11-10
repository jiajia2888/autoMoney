"""Local adapter around the preserved MediaCrawler DouYinClient.

Uses a separate persistent browser profile; never reads the user's default Chrome
cookies. stdout is one JSON document. No videos or comments are downloaded.
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
import signal
import sys
import types
from datetime import datetime, timezone
from urllib.parse import urlparse

ROOT = Path(__file__).resolve().parents[1]
VENDOR = ROOT / "vendor" / "media-crawler"
ID_RE = re.compile(r"MS4wLj[A-Za-z0-9_-]{20,180}")
STAGE = "初始化采集环境"
LOGIN_PROBE_TIMEOUT = 3


def load_expert_client():
    # Import the preserved client without __init__.py eagerly importing the
    # whole crawler/store/wordcloud pipeline. No vendor source is rewritten.
    os.chdir(VENDOR)
    sys.path.insert(0, str(VENDOR))
    package = types.ModuleType("media_platform.douyin")
    package.__path__ = [str(VENDOR / "media_platform" / "douyin")]
    sys.modules["media_platform.douyin"] = package
    return importlib.import_module("media_platform.douyin.client").DouYinClient


def canonical_id(value):
    u = urlparse(value)
    if u.scheme != "https" or u.netloc != "www.douyin.com":
        raise ValueError("账号主页地址无效")
    match = re.fullmatch(r"/user/(MS4wLj[A-Za-z0-9_-]{20,180})/?", u.path)
    if not match:
        raise ValueError("无法识别抖音账号标识")
    return match[1]


def normalize_post(item):
    if not isinstance(item, dict):
        raise ValueError("作品字段结构异常")
    if item.get("images") or item.get("aweme_type") in (68, 150):
        return None
    if not isinstance(item.get("video"), dict):
        return None
    vid = str(item.get("aweme_id", ""))
    if not re.fullmatch(r"\d{10,30}", vid):
        raise ValueError("作品缺少有效视频 ID")
    stats = item.get("statistics")
    if not isinstance(stats, dict):
        raise ValueError("作品缺少点赞统计，不以零替代")
    likes = stats.get("digg_count")
    if isinstance(likes, bool) or not isinstance(likes, int) or likes < 0:
        raise ValueError("作品点赞数不是有效原始整数")
    created = item.get("create_time")
    published = None
    if isinstance(created, int) and created > 0:
        published = datetime.fromtimestamp(created, timezone.utc).isoformat()
    video = item["video"]
    cover = ""
    for key in ("origin_cover", "cover", "dynamic_cover"):
        node = video.get(key) or {}
        urls = node.get("url_list") if isinstance(node, dict) else None
        if urls:
            cover = urls[-1]
            break
    return dict(platform_id=vid, url="https://www.douyin.com/video/" + vid,
                title=item.get("desc") or "无标题", cover_url=cover,
                likes=likes, comments=stats.get("comment_count") or 0,
                favorites=stats.get("collect_count") or 0,
                shares=stats.get("share_count") or 0, published_at=published)


async def logged_in(page, context):
    # Cookie queries do not depend on the page's JS thread being responsive.
    cookies = {c["name"]: c["value"] for c in await asyncio.wait_for(
        context.cookies("https://www.douyin.com"), timeout=LOGIN_PROBE_TIMEOUT)}
    if cookies.get("LOGIN_STATUS") == "1":
        return True
    try:
        storage = await asyncio.wait_for(
            page.evaluate("() => localStorage.getItem('HasUserLogin')"),
            timeout=LOGIN_PROBE_TIMEOUT)
    except TimeoutError:
        return None  # Unknown is different from a confirmed logged-out page.
    return storage == "1"


def load_credentials(path):
    if not path or not Path(path).exists():
        return None
    try:
        raw = Path(path).read_bytes()
        if len(raw) > 65536:
            raise ValueError()
        data = json.loads(raw)
        cookie, ua = data["cookie"], data["user_agent"]
        if not isinstance(cookie, str) or not isinstance(ua, str):
            raise ValueError()
        if not 0 < len(cookie) <= 32768 or not 10 <= len(ua) <= 1024:
            raise ValueError()
        if any(ord(c) < 32 or ord(c) > 126 for c in cookie + ua):
            raise ValueError()
        cookies = {}
        for part in cookie.split(";"):
            if not part.strip():
                continue
            name, sep, value = part.strip().partition("=")
            # Chromium can send nameless cookies as bare tokens. Keep them
            # in the raw HTTP header, not the named signing/auth dictionary.
            if not sep and re.fullmatch(r"[!#$%&'*+.^_`|~0-9A-Za-z-]+", name):
                continue
            if not sep or not re.fullmatch(r"[!#$%&'*+.^_`|~0-9A-Za-z-]+", name) or name in cookies:
                raise ValueError()
            cookies[name] = value
        if not (cookies.get("sessionid") or cookies.get("sessionid_ss")):
            raise ValueError()
        return cookie, ua, cookies
    except (OSError, ValueError, KeyError, TypeError):
        raise ValueError("本地 Cookie 配置无效，请在连接抖音中重新保存") from None


class CookieSigningContext:
    """Supply user-provided msToken to the original client's signing metadata.

    The upstream client only reads localStorage here; signing still uses its
    original Node.js implementation. This does not fabricate an API response.
    """
    def __init__(self, cookies):
        self.storage = {"xmst": cookies["msToken"]} if cookies.get("msToken") else {}

    async def evaluate(self, expression):
        if expression != "() => window.localStorage":
            raise ValueError("手动 Cookie 模式不支持此页面操作")
        return dict(self.storage)


async def collect_posts(client, args):
    global STAGE
    sec = canonical_id(args.url)
    posts, seen_ids, cursors = [], set(), set()
    cursor = ""
    for _ in range(35):
        STAGE = "请求账号作品接口"
        data = await client.get_user_aweme_posts(sec, cursor)
        STAGE = "校验作品数据"
        if not isinstance(data, dict) or data.get("status_code") != 0:
            raise ValueError("抖音接口未返回成功数据；请更新登录凭据或在专用窗口检查验证提示")
        items = data.get("aweme_list")
        if not isinstance(items, list) or not items:
            if not posts:
                raise ValueError("未获取作品；无法区分无作品与访问受限，请检查账号主页与登录凭据")
            raise ValueError("翻页返回空数据，未将不确定的结果写入")
        for item in items:
            post = normalize_post(item)
            if post and post["platform_id"] not in seen_ids:
                seen_ids.add(post["platform_id"])
                posts.append(post)
                if len(posts) >= args.max_items:
                    return dict(posts=posts, complete=False, stop_reason="item_limit")
        if not data.get("has_more"):
            return dict(posts=posts, complete=True, stop_reason="end")
        new_cursor = str(data.get("max_cursor", ""))
        if not new_cursor or new_cursor == cursor or new_cursor in cursors:
            raise ValueError("作品分页游标重复，已停止本轮；请稍后重试")
        cursors.add(new_cursor)
        cursor = new_cursor
        await asyncio.sleep(3)
    return dict(posts=posts, complete=False, stop_reason="page_limit")


async def run(args):
    global STAGE
    from playwright.async_api import async_playwright, TimeoutError as BrowserTimeout
    client_type = load_expert_client()
    if args.mode == "collect":
        canonical_id(args.url)
        STAGE = "读取本地登录配置"
        credentials = load_credentials(args.credentials_file)
        if credentials:
            cookie, ua, cookies = credentials
            headers = {"User-Agent": ua, "Referer": args.url, "Cookie": cookie}
            client = client_type(headers=headers, playwright_page=CookieSigningContext(cookies),
                                 cookie_dict=cookies, timeout=30)
            return await collect_posts(client, args)
    profile = Path(args.profile).resolve()
    profile.mkdir(parents=True, exist_ok=True, mode=0o700)
    async with async_playwright() as pw:
        opts = dict(user_data_dir=str(profile), channel="chrome", headless=args.mode != "login",
                    no_viewport=True, args=["--no-first-run"], timeout=45000)
        STAGE = "启动专用浏览器"
        context = await pw.chromium.launch_persistent_context(**opts)
        try:
            page = context.pages[0] if context.pages else await context.new_page()
            target = "https://www.douyin.com/user/self" if args.mode == "login" else args.url
            STAGE = "打开抖音页面"
            # Douyin may keep its document parser busy. Login probing below is
            # bounded independently and does not require all page scripts to finish.
            await page.goto(target, wait_until="commit", timeout=45000)
            STAGE = "检查专用窗口登录状态"
            if args.mode == "login":
                await page.bring_to_front()
                if not await logged_in(page, context):
                    STAGE = "显示登录入口"
                    button = page.get_by_role("button", name="登录", exact=True)
                    if await button.count() == 1 and await button.is_visible():
                        # A login modal may already cover the page's login button.
                        # Keep the window open for the user rather than fail on it.
                        try:
                            await button.click(timeout=3000)
                        except BrowserTimeout:
                            pass
                STAGE = "等待专用窗口扫码登录"
                deadline = asyncio.get_running_loop().time() + 600
                while asyncio.get_running_loop().time() < deadline:
                    if await logged_in(page, context):
                        return {"ok": True}
                    await asyncio.sleep(2)
                raise ValueError("登录等待超时，请点击连接抖音后在专用窗口完成扫码/验证")
            authenticated = await logged_in(page, context)
            if authenticated is None:
                raise ValueError("抖音页面脚本未响应，无法确认登录状态；请检查专用窗口页面是否正常加载")
            if not authenticated:
                raise ValueError("专用采集浏览器尚未登录，请先点击连接抖音并扫码（与普通 Chrome 登录态独立）")
            sec = canonical_id(args.url)
            cookies = {c["name"]: c["value"] for c in await context.cookies(["https://www.douyin.com", "https://douyin.com"])}
            ua = await page.evaluate("() => navigator.userAgent")
            headers = {"User-Agent": ua, "Referer": "https://www.douyin.com/user/" + sec,
                       "Cookie": "; ".join(f"{k}={v}" for k, v in cookies.items())}
            client = client_type(headers=headers, playwright_page=page, cookie_dict=cookies, timeout=30)
            return await collect_posts(client, args)
        finally:
            await context.close()


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--mode", choices=["doctor", "login", "collect"], default="doctor")
    p.add_argument("--profile", default=str(ROOT / ".local" / "douyin-profile"))
    p.add_argument("--url", default="")
    p.add_argument("--max-items", type=int, default=100)
    p.add_argument("--credentials-file", default=str(ROOT / ".local" / "douyin-credentials.json"))
    args = p.parse_args()
    if not 1 <= args.max_items <= 500:
        p.error("max-items must be 1..500")
    try:
        if args.mode == "doctor":
            with contextlib.redirect_stdout(sys.stderr):
                client = load_expert_client()
                from playwright.async_api import async_playwright
                import execjs
                if not execjs.get().is_available():
                    raise ValueError("缺少 Node.js 签名运行环境")
            print(json.dumps({"ok": True, "adapter": client.__name__, "note": "imports only; login/network not verified"}))
        else:
            async def entry():
                task = asyncio.current_task()
                loop = asyncio.get_running_loop()
                loop.add_signal_handler(signal.SIGTERM, task.cancel)
                return await run(args)
            with contextlib.redirect_stdout(sys.stderr):
                result = asyncio.run(entry())
            print(json.dumps(result, ensure_ascii=False))
    except (KeyboardInterrupt, asyncio.CancelledError):
        print(json.dumps({"error": "任务已取消"}, ensure_ascii=False))
        sys.exit(1)
    except Exception as exc:
        # Do not emit headers, cookies, or raw upstream responses into application logs.
        message = str(exc) if isinstance(exc, ValueError) else f"{STAGE}失败（{type(exc).__name__}），请在连接抖音中检查登录配置或运行 make collector-doctor"
        print(json.dumps({"error": message}, ensure_ascii=False))
        sys.exit(1)

if __name__ == "__main__":
    main()
