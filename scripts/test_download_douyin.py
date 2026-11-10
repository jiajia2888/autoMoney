import asyncio
from pathlib import Path
import tempfile
import unittest
import httpx
from download_douyin import stream_media, media_url_allowed, DownloadError, video_id


class DownloadTest(unittest.IsolatedAsyncioTestCase):
    async def transfer(self, handler, url='https://v11.douyinvod.com/video'):
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / 'video.part'
            async with httpx.AsyncClient(transport=httpx.MockTransport(handler), follow_redirects=False) as client:
                return await stream_media(client, url, target)

    async def test_redirect_cannot_escape_media_hosts(self):
        calls = []
        def respond(request):
            calls.append(request)
            return httpx.Response(302, headers={'Location': 'https://evil.example/video'})
        with self.assertRaisesRegex(DownloadError, '允许'):
            await self.transfer(respond)
        self.assertEqual(len(calls), 1)

    async def test_html_success_and_truncated_mp4_rejected(self):
        for headers, content in [({'Content-Type': 'text/html'}, b'<html>blocked'),
                                 ({'Content-Type': 'video/mp4', 'Content-Length': '100'}, b'short')]:
            with self.assertRaises(DownloadError):
                await self.transfer(lambda r: httpx.Response(200, headers=headers, content=content))

    async def test_valid_bounded_stream(self):
        count = await self.transfer(lambda r: httpx.Response(200, headers={'Content-Type': 'video/mp4'}, content=b'video bytes'))
        self.assertEqual(count, 11)

    def test_only_official_inputs_and_https_media(self):
        self.assertEqual(video_id('https://www.douyin.com/video/7639685139231984939'), '7639685139231984939')
        for url in ['http://v11.douyinvod.com/v', 'https://douyinvod.com.evil.example/v', 'https://user@v11.douyinvod.com/v', 'https://127.0.0.1/v']:
            self.assertFalse(media_url_allowed(url))
        with self.assertRaises(DownloadError):
            video_id('https://evil.example/video/7639685139231984939')
