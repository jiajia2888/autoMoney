import json
import os
from pathlib import Path
import tempfile
import unittest
from yuanbao_login import cookie_header, save_credentials, open_login_widget
from unittest.mock import MagicMock

class YuanbaoLoginTest(unittest.TestCase):
    def test_anonymous_expired_and_injection_are_not_login(self):
        self.assertIsNone(cookie_header([{'name':'tracking','value':'x','expires':-1}], 100))
        self.assertIsNone(cookie_header([{'name':'hy_user','value':'x','expires':99},{'name':'hy_token','value':'x','expires':-1}],100))
        self.assertIsNone(cookie_header([{'name':'hy_user','value':'x','expires':-1},{'name':'hy_token','value':'x\r\ny','expires':-1}],100))
        self.assertEqual(cookie_header([{'name':'hy_user','value':'x','expires':-1},{'name':'hy_token','value':'y','expires':200}],100),'hy_user=x; hy_token=y')

    def test_credentials_atomic_and_owner_only(self):
        with tempfile.TemporaryDirectory() as tmp:
            path=Path(tmp)/'auth.json'
            save_credentials(path,{'cookie':'private'})
            self.assertEqual(os.stat(path).st_mode & 0o777,0o600)
            self.assertEqual(json.loads(path.read_text()),{'cookie':'private'})
            self.assertEqual(len(list(Path(tmp).iterdir())),1)

class LoginOverlayTest(unittest.TestCase):
    def test_already_open_overlay_does_not_click_covered_entry(self):
        page = MagicMock()
        widget = page.locator.return_value
        widget.is_visible.return_value = True
        self.assertIs(open_login_widget(page), widget)
        page.get_by_role.assert_not_called()

    def test_auto_open_overlay_during_click_is_accepted(self):
        page = MagicMock()
        widget = page.locator.return_value
        widget.is_visible.side_effect = [False, True]
        page.get_by_role.return_value.or_.return_value.filter.return_value.first.click.side_effect = TimeoutError('entry covered by overlay')
        self.assertIs(open_login_widget(page), widget)

    def test_click_failure_without_overlay_is_not_success(self):
        page = MagicMock()
        page.locator.return_value.is_visible.return_value = False
        page.get_by_role.return_value.or_.return_value.filter.return_value.first.click.side_effect = TimeoutError('no overlay')
        with self.assertRaises(TimeoutError):
            open_login_widget(page)
