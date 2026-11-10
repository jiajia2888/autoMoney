"""只读示例：投手 Cookie 查询作者、作品和分享链接；不实现视频文件下载。"""
from __future__ import annotations

import argparse
import json
import os
import sys
import time
from urllib.parse import quote

import httpx

ORIGIN = "https://channels.weixin.qq.com"
ROOT = "/promote/api/web/"
LOGIN_PAGE = ORIGIN + "/promote/pages/platform/login-iframe"
CREATE_PAGE = ORIGIN + "/promote/pages/platform/create-order"
HOME_PAGE = ORIGIN + "/promote/pages/platform/home"
SHARE_PAGE = ORIGIN + "/promote/pages/platform/short-video/detail"


class ApiError(Exception):
    pass


class PromoteClient:
    def __init__(self, session_value="", *, transport=None):
        self.client = httpx.Client(
            base_url=ORIGIN, timeout=15, trust_env=False, follow_redirects=False,
            transport=transport,
            headers={"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"},
        )
        if session_value:
            self.client.cookies.set("promotewebsessionid", session_value, domain="channels.weixin.qq.com", path="/")

    def close(self):
        self.client.close()

    def _post(self, suffix, page, body, *, form=False):
        try:
            res = self.client.post(ROOT + suffix, params={"_pageUrl": page}, **({"data": body} if form else {"json": body}))
            value = res.json()
        except (httpx.HTTPError, ValueError):
            raise ApiError("上游网络失败或返回非 JSON，请稍后检查") from None
        if not 200 <= res.status_code < 300 or not isinstance(value, dict) or value.get("error"):
            raise ApiError("上游请求失败，请核验会话和接口响应")
        current = value
        for _ in range(5):
            for key in ("errCode", "errcode", "ret", "Ret"):
                if key in current and str(current[key]) != "0":
                    raise ApiError("上游业务失败，请核验会话和接口参数")
            for key in ("baseResp", "baseResponse", "BaseResponse"):
                base = current.get(key)
                if isinstance(base, dict):
                    for field in ("errcode", "errCode", "ret", "Ret"):
                        if field in base and str(base[field]) != "0":
                            raise ApiError("上游业务失败，请核验会话和接口参数")
            if not isinstance(current.get("data"), dict):
                break
            current = current["data"]
        data = value.get("data")
        if not isinstance(data, dict):
            raise ApiError("上游响应缺少 data 对象")
        return data

    def login_challenge(self):
        data = self._post("auth/auth_login_token", LOGIN_PAGE, {})
        token = data.get("token")
        if not isinstance(token, str) or not token:
            raise ApiError("上游未返回二维码 token")
        return {"challenge": data, "qr_text": ORIGIN + "/promote/pages/mobile_login?token=" + quote(token, safe="")}

    def poll_login(self, challenge):
        # 手机确认后，httpx CookieJar 自动保存本次响应的 Set-Cookie。
        # 调用方应限制轮询频率，并随后调用 profile() 核验身份。
        return self._post("auth/auth_login_status", LOGIN_PAGE, challenge)

    def profile(self):
        data = self._post("transfer/MMFinderPromotionLiveDspApiSvr/getLivePromotionUserProfile", HOME_PAGE, {})
        identity = data.get("selfAccountInfo")
        if not isinstance(identity, dict) or not identity.get("username"):
            raise ApiError("投手身份尚未验证，请重新扫码")
        return identity

    def search_authors(self, keyword, *, page=1, page_size=5):
        data = self._post("transfer/MmFinderPromotionApiSvr/searchAccount", CREATE_PAGE,
                          {"query": keyword, "flag": "1", "currentPage": str(page), "pageSize": str(page_size)}, form=True)
        if not isinstance(data.get("list"), list):
            raise ApiError("上游响应缺少作者数组")
        return data

    def author_videos(self, author, *, start, end, limit=5, cursor=""):
        body = {"encryptedUsername": author, "startTs": str(start), "endTs": str(end), "limit": str(limit)}
        if cursor:
            body["lastBuf"] = cursor
        data = self._post("transfer/MmFinderPromotionApiSvr/getFinderObject", CREATE_PAGE, body, form=True)
        if not isinstance(data.get("objects"), list):
            raise ApiError("上游响应缺少作品数组")
        return data

    def share_link(self, export_id, nonce):
        data = self._post("transfer/MmFinderAssistantBroker/getObjectShortLink", SHARE_PAGE,
                          {"exportId": export_id, "nonceId": nonce}, form=True)
        value = data.get("shortUrl")
        if not isinstance(value, str) or not value.startswith("https://"):
            raise ApiError("上游未返回有效分享链接")
        return value


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument("--keyword", help="搜索作者昵称/关键词")
    source.add_argument("--author", help="目标作者 username / finder_id")
    parser.add_argument("--days", type=int, default=90)
    parser.add_argument("--pages", type=int, choices=range(1, 11), default=1)
    parser.add_argument("--limit", type=int, choices=range(1, 16), default=5)
    parser.add_argument("--share", action="store_true", help="仅为本轮首条作品取得分享链接")
    args = parser.parse_args()
    if args.days <= 0:
        parser.error("--days 应为正整数")
    session_value = os.getenv("PROMOTE_WEB_SESSION_ID", "")
    if not session_value:
        parser.error("请在安全环境中设置 PROMOTE_WEB_SESSION_ID（仅 Cookie 值部分）")
    client = PromoteClient(session_value)
    try:
        client.profile()
        if args.keyword:
            print(json.dumps(client.search_authors(args.keyword, page_size=args.limit), ensure_ascii=False, indent=2))
            return 0
        end = int(time.time())
        start = end - args.days * 86400
        cursor, seen, video_ids = "", set(), set()
        result = []
        stop_reason = "page_limit_reached"
        for _ in range(args.pages):
            data = client.author_videos(args.author, start=start, end=end, limit=args.limit, cursor=cursor)
            for item in data["objects"]:
                if not isinstance(item, dict):
                    raise ApiError("作品数组格式变化")
                identity = item.get("exportId")
                if not identity:
                    raise ApiError("作品缺少 exportId")
                if identity not in video_ids:
                    video_ids.add(identity)
                    result.append(item)
            if not data.get("continueFlag"):
                stop_reason = "upstream_end"
                break
            next_cursor = data.get("lastBuf")
            if not data["objects"] or not isinstance(next_cursor, str) or not next_cursor or next_cursor in seen:
                raise ApiError("分页游标缺失、重复或空页，已停止；不能判定作品已采全")
            seen.add(next_cursor)
            cursor = next_cursor
            time.sleep(1)
        output = {"source": "wechat_promote_web", "collected_at": int(time.time()), "target_finder_id": args.author,
                  "stop_reason": stop_reason, "objects": result}
        if args.share and result:
            first = result[0]
            output["share_url"] = client.share_link(first["exportId"], first.get("nonce", ""))
        print(json.dumps(output, ensure_ascii=False, indent=2))
        return 0
    except ApiError as error:
        print(str(error), file=sys.stderr)
        return 1
    finally:
        client.close()


if __name__ == "__main__":
    raise SystemExit(main())
