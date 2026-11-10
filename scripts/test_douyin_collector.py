import unittest
import asyncio
import json
import os
from pathlib import Path
from tempfile import TemporaryDirectory
from types import SimpleNamespace
from unittest.mock import AsyncMock, patch
from douyin_collector import (normalize_post, canonical_id, logged_in, load_credentials,
                              CookieSigningContext, load_expert_client, run, collect_posts)

URL = "https://www.douyin.com/user/MS4wLjABAAAAovmrcujSuO71ql7DfF0X6PymlOfV5komvBZscsJ75H-TaRccS6pAudUnWcYAuX0E"


class ManualCookieTest(unittest.IsolatedAsyncioTestCase):
    async def test_manual_cookie_collects_without_starting_browser(self):
        with TemporaryDirectory() as directory:
            path = Path(directory) / "credentials.json"
            path.write_text(json.dumps({"cookie": "sessionid=test-only; msToken=test-token", "user_agent": "Mozilla/5.0 TestBrowser"}))
            client = AsyncMock()
            client.get_user_aweme_posts.return_value = {"status_code": 0, "has_more": 0, "aweme_list": [
                {"aweme_id": "7639685139231984939", "video": {}, "statistics": {"digg_count": 10001}}]}
            def factory(**kwargs):
                self.assertEqual(kwargs["headers"]["Cookie"], "sessionid=test-only; msToken=test-token")
                self.assertIsInstance(kwargs["playwright_page"], CookieSigningContext)
                return client
            args = SimpleNamespace(mode="collect", url=URL, max_items=10, credentials_file=str(path))
            with patch("douyin_collector.load_expert_client", return_value=factory), patch("playwright.async_api.async_playwright", side_effect=AssertionError("browser must not start")):
                result = await run(args)
            self.assertEqual(result["posts"][0]["likes"], 10001)
            self.assertTrue(result["complete"])

    async def test_real_expert_signing_with_cookie_metadata(self):
        previous = os.getcwd()
        try:
            client_type = load_expert_client()
            cookies = {"sessionid": "test-only", "msToken": "test-token"}
            client = client_type(headers={"User-Agent": "Mozilla/5.0 TestBrowser"},
                                 playwright_page=CookieSigningContext(cookies), cookie_dict=cookies)
            client.request = AsyncMock(return_value={"status_code": 0})
            await client.get_user_aweme_posts(canonical_id(URL), "")
            params = client.request.call_args.kwargs["params"]
            self.assertEqual(params["msToken"], "test-token")
            self.assertTrue(params["a_bogus"])
        finally:
            os.chdir(previous)

    async def test_interface_failure_is_not_empty_success(self):
        client = AsyncMock()
        args = SimpleNamespace(url=URL, max_items=10)
        for data in ({"status_code": 1}, {"status_code": 0, "aweme_list": []}):
            client.get_user_aweme_posts.return_value = data
            with self.assertRaises(ValueError):
                await collect_posts(client, args)

    def test_browser_nameless_cookie_preserves_header(self):
        with TemporaryDirectory() as directory:
            path = Path(directory) / "credentials.json"
            raw = "sessionid=test-only; browserBare123; msToken=test-token"
            path.write_text(json.dumps({"cookie": raw, "user_agent": "Mozilla/5.0 TestBrowser"}))
            cookie, _, cookies = load_credentials(path)
            self.assertEqual(cookie, raw)
            self.assertEqual(cookies, {"sessionid": "test-only", "msToken": "test-token"})

    def test_invalid_credential_errors_do_not_echo_input(self):
        with TemporaryDirectory() as directory:
            path = Path(directory) / "credentials.json"
            for cookie in ("sensitive-token-alone", "sessionid; msToken=x", "sessionid=x; invalid bare value", "sessionid=x; sessionid=y", "sessionid=x\r\nX: y"):
                path.write_text(json.dumps({"cookie": cookie, "user_agent": "Mozilla/5.0 TestBrowser"}))
                with self.assertRaises(ValueError) as error:
                    load_credentials(path)
                self.assertNotIn(cookie, str(error.exception))
            self.assertIsNone(load_credentials(Path(directory) / "missing.json"))


class LoginTest(unittest.IsolatedAsyncioTestCase):
    async def test_only_explicit_login_opens_visible_browser(self):
        from tempfile import TemporaryDirectory
        from types import SimpleNamespace
        from unittest.mock import MagicMock
        from douyin_collector import run
        for mode in ("login", "collect"):
            with TemporaryDirectory() as directory:
                page, context, pw = AsyncMock(), AsyncMock(), MagicMock()
                context.pages = [page]
                context.cookies.return_value = [{"name": "LOGIN_STATUS", "value": "1"}]
                page.evaluate.return_value = "Mozilla Test"
                pw.chromium.launch_persistent_context = AsyncMock(return_value=context)
                manager = AsyncMock()
                manager.__aenter__.return_value = pw
                args = SimpleNamespace(mode=mode, credentials_file="missing", profile=directory,
                    url="https://www.douyin.com/user/MS4wLjABAAAAAAAAAAAAAAAAAAAAAAAA", max_items=1)
                with patch("playwright.async_api.async_playwright", return_value=manager), patch("douyin_collector.load_expert_client", return_value=MagicMock()), patch("douyin_collector.load_credentials", return_value=None), patch("douyin_collector.collect_posts", new_callable=AsyncMock):
                    await run(args)
                self.assertEqual(pw.chromium.launch_persistent_context.call_args.kwargs["headless"], mode != "login")
                context.close.assert_awaited_once()

    async def test_cookie_login_does_not_wait_for_page_script(self):
        async def blocked_page(*args):
            await asyncio.Event().wait()
        page = AsyncMock()
        page.evaluate.side_effect = blocked_page
        context = AsyncMock()
        context.cookies.return_value = [{"name": "LOGIN_STATUS", "value": "1"}]
        self.assertTrue(await asyncio.wait_for(logged_in(page, context), timeout=0.2))

    async def test_blocked_page_is_unknown_instead_of_logged_out(self):
        async def blocked_page(*args):
            await asyncio.Event().wait()
        page, context = AsyncMock(), AsyncMock()
        page.evaluate.side_effect = blocked_page
        context.cookies.return_value = []
        with patch("douyin_collector.LOGIN_PROBE_TIMEOUT", 0.02):
            self.assertIsNone(await asyncio.wait_for(logged_in(page, context), timeout=0.2))

    async def test_responsive_page_login_marker(self):
        page, context = AsyncMock(), AsyncMock()
        context.cookies.return_value = []
        for value, expected in (("1", True), (None, False), ("0", False)):
            page.evaluate.return_value = value
            self.assertIs(await logged_in(page, context), expected)

class CollectorTest(unittest.TestCase):
    def post(self, likes):
        return {"aweme_id": "7639685139231984939", "video": {}, "statistics": {"digg_count": likes}}
    def test_exact_counts(self):
        for likes in (9999, 10000, 10001):
            self.assertEqual(normalize_post(self.post(likes))["likes"], likes)
    def test_unknown_counts_fail(self):
        for likes in (None, "1.0万", True, -1):
            with self.assertRaises(ValueError): normalize_post(self.post(likes))
    def test_image_post_excluded(self):
        p=self.post(10001);p['images']=[{}]
        self.assertIsNone(normalize_post(p))
    def test_bad_origin_rejected(self):
        with self.assertRaises(ValueError): canonical_id('https://foreign.test/user/MS4wLjABAAAAabcd')

if __name__ == '__main__': unittest.main()
