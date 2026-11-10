"""Project-owned, isolated Yuanbao login. Never opens the user's Chrome profile."""
import argparse
import json
import os
from pathlib import Path
import tempfile
import time
from urllib.parse import urlsplit


def save_credentials(path, data):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    fd, name = tempfile.mkstemp(prefix='.yuanbao-', dir=path.parent)
    try:
        with os.fdopen(fd, 'w') as out:
            os.fchmod(out.fileno(), 0o600)
            json.dump(data, out, ensure_ascii=False)
            out.flush()
            os.fsync(out.fileno())
        os.replace(name, path)
    finally:
        if os.path.exists(name):
            os.unlink(name)


def cookie_header(cookies, now):
    # These cookies come from cookies(endpoint), which applies domain/path rules.
    valid = [c for c in cookies if c.get('expires', -1) == -1 or c.get('expires', 0) > now]
    selected = {}
    for c in sorted(valid, key=lambda c: len(c.get('path', '/')), reverse=True):
        name, value = c.get('name', ''), c.get('value', '')
        if name and value and not any(x in name + value for x in '\r\n;'):
            selected.setdefault(name, value)
    if not selected.get('hy_user') or not selected.get('hy_token'):
        return None
    return '; '.join(k + '=' + v for k, v in selected.items())


def save_qr(path, png):
    path = Path(str(path) + '.qr.png')
    fd, name = tempfile.mkstemp(prefix='.yuanbao-qr-', dir=path.parent)
    try:
        with os.fdopen(fd, 'wb') as out:
            os.fchmod(out.fileno(), 0o600)
            out.write(png)
            out.flush()
            os.fsync(out.fileno())
        os.replace(name, path)
    finally:
        if os.path.exists(name):
            os.unlink(name)


def open_login_widget(page):
    widget = page.locator('iframe[src^="https://open.weixin.qq.com/connect/qrconnect"]')
    if not widget.is_visible():
        try:
            page.get_by_role('button', name='登录', exact=True).or_(page.get_by_text('未登录', exact=True)).filter(visible=True).first.click(timeout=10000, no_wait_after=True)
        except Exception:
            # The site can automatically open its login overlay while
            # the entry is hydrating, making the original entry covered.
            if not widget.is_visible():
                raise
    return widget


def login(path, timeout=300, proxy_server=None):
    from playwright.sync_api import sync_playwright
    with sync_playwright() as pw:
        Path(path).parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        stage = '启动浏览器'
        browser = pw.chromium.launch(headless=True, timeout=45000, proxy={'server':proxy_server} if proxy_server else None)
        context = None
        capture = None
        try:
            stage = '初始化独立会话'
            context = browser.new_context(locale='zh-CN', viewport={'width': 1280, 'height': 900})
            page = context.new_page()
            captured = {}
            def capture(request):
                if urlsplit(request.url).hostname == 'yuanbao.tencent.com' and '/api/' in urlsplit(request.url).path:
                    for key, value in request.headers.items():
                        if key in ('authorization', 't-userid', 'x-id', 'x-device-id', 'x-hy92', 'x-hy93') and '\r' not in value and '\n' not in value:
                            captured[key] = value
            context.on('request', capture)
            stage = '加载元宝官网'
            page.goto('https://yuanbao.tencent.com/', wait_until='domcontentloaded', timeout=45000)
            stage = '打开微信登录'
            widget = open_login_widget(page)
            stage = '加载官方二维码'
            widget.wait_for(state='visible', timeout=30000)
            # Only a clean, project-owned context is used. Waiting for the QR
            # image prevents publishing an empty iframe as "ready".
            frame = page.frame_locator('iframe[src^="https://open.weixin.qq.com/connect/qrconnect"]')
            other_account = frame.get_by_role('button', name='使用其他头像、昵称或账号', exact=True)
            if other_account.is_visible():
                other_account.click()
            qr = frame.locator('img.qrcode')
            qr.wait_for(state='visible', timeout=30000)
            save_qr(path, qr.screenshot())
            stage = '等待手机确认'
            deadline = time.monotonic() + timeout
            while time.monotonic() < deadline:
                if page.is_closed():
                    raise ValueError('元宝登录会话已关闭，尚未保存新会话')
                cookies = context.cookies('https://yuanbao.tencent.com/api/weixin/get_parse_result')
                cookie = cookie_header(cookies, time.time())
                if cookie:
                    save_credentials(path, {'cookie': cookie, 'user_agent': page.evaluate('navigator.userAgent'), 'headers': captured, 'saved_at': int(time.time())})
                    return {'status': 'saved'}
                page.wait_for_timeout(1000)
            raise ValueError('元宝登录等待超时，请重新获取二维码')
        except ValueError:
            raise
        except Exception as exc:
            raise ValueError('元宝登录未完成（' + stage + '），请重试') from exc
        finally:
            if context is not None and capture is not None:
                context.remove_listener('request', capture)
            browser.close()
            Path(str(path) + '.qr.png').unlink(missing_ok=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--credentials-file', required=True)
    parser.add_argument('--proxy-server')
    args = parser.parse_args()
    try:
        result = login(args.credentials_file, proxy_server=args.proxy_server)
    except ValueError as exc:
        result = {'error': str(exc)}
    except Exception as exc:
        result = {'error': '元宝登录浏览器未能启动，请安装项目Playwright Chromium后重试'}
    print(json.dumps(result, ensure_ascii=False))
    raise SystemExit(1 if 'error' in result else 0)
