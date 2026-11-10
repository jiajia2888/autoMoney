import json
from pathlib import Path
from tempfile import TemporaryDirectory
from types import SimpleNamespace
import unittest
from unittest.mock import patch
import extract_copy


class CopyTest(unittest.TestCase):
    def test_cookie_file_is_private_and_scoped_to_douyin(self):
        with TemporaryDirectory() as directory:
            path = Path(directory) / "cookies.txt"
            extract_copy.cookie_file(("sessionid=test", "Mozilla Test", {"sessionid": "test"}), path)
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            self.assertIn(".douyin.com\tTRUE\t/\tTRUE\t0\tsessionid\ttest", path.read_text())

    def test_download_reuses_dedicated_profile_without_manual_cookie(self):
        with TemporaryDirectory() as directory:
            target = Path(directory) / "cookies.txt"
            with patch("extract_copy.load_credentials", return_value=None), patch("extract_copy.profile_credentials", return_value=("sessionid=test", "Mozilla Test", {"sessionid": "test"})) as profile:
                options = extract_copy.download_credentials(SimpleNamespace(credentials_file="missing", profile="dedicated"), target)
                profile.assert_called_once_with("dedicated")
                self.assertEqual(options, ["--cookies", str(target), "--user-agent", "Mozilla Test"])
                self.assertEqual(target.stat().st_mode & 0o777, 0o600)

    def test_manual_credentials_do_not_open_browser(self):
        with TemporaryDirectory() as directory:
            with patch("extract_copy.load_credentials", return_value=("sessionid=test", "Mozilla Test", {"sessionid": "test"})), patch("extract_copy.profile_credentials") as profile:
                extract_copy.download_credentials(SimpleNamespace(credentials_file="saved", profile="dedicated"), Path(directory) / "cookies.txt")
                profile.assert_not_called()

    def test_arbitrary_urls_are_rejected_before_download(self):
        args = SimpleNamespace(model="model", input="", url="https://evil.example/video/123")
        with patch("extract_copy.tools", return_value=({}, [])), patch("extract_copy.command") as command:
            with self.assertRaises(extract_copy.CopyError): extract_copy.extract(args)
            command.assert_not_called()

    def test_long_audio_is_not_silently_truncated(self):
        with TemporaryDirectory() as directory:
            source = Path(directory) / "audio.mp3"
            source.write_bytes(b"test")
            args = SimpleNamespace(model="model", input=str(source), url="")
            with patch("extract_copy.tools", return_value=({"ffprobe": "ffprobe"}, [])), patch("extract_copy.command", return_value=json.dumps({"format": {"duration": 1801}}).encode()):
                with self.assertRaisesRegex(extract_copy.CopyError, "30 分钟"):
                    extract_copy.extract(args)

if __name__ == "__main__": unittest.main()
