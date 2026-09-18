#!/usr/bin/env python3
# 从 shortcuts.json 生成可导入、可分享的 shortcuts.zip。
#
# 为什么以 shortcuts.json 为准：它就是 HTTP Shortcuts 真正读写的东西。改捷径的流程是
# 「在应用里改 → 导出成 shortcuts.json 丢回来」，**不存在第二份要跟它对齐的源**。
# （曾经由这个脚本把一份 shortcuts.js 注入 JSON。两份同内容的东西必然会漂，而且要去改
#   JSON 里那段转义过的 JS 几乎不可能改对 —— 那条路已废弃，shortcuts.js 不再存在。）
#
# 本脚本只做四件事：自检 → 导出可读镜像 → 抹掉凭据 → 打包。
# **不生成、也不改写 shortcuts.json。**
#
# 用法: python3 build.py
import io
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import zipfile

HERE = os.path.dirname(os.path.abspath(__file__))
JSON_PATH = os.path.join(HERE, "shortcuts.json")
ZIP_PATH = os.path.join(HERE, "shortcuts.zip")
MIRROR_PATH = os.path.join(HERE, "shortcuts.js")
VERIFY = os.path.join(HERE, "verify.mjs")

# 可读镜像。shortcuts.json 带着房间密码、不入库，那段 JS 就只剩「压成一行 + 转义过」的
# 形态（& 写成 \u0026、换行写成 \n），git 里既读不了也 diff 不出。所以把它原样导出成
# shortcuts.js —— **只写不读**，因此它不是第二个源，也不可能与 JSON 漂。
MIRROR_HEADER = """// ⚠️ 本文件由 build.py 从 shortcuts.json 自动生成，**别手改**。
// 要改这段逻辑：在 App 里改捷径 → 重新导出 shortcuts.json → 再跑一次 build.py。
//
// 下面这段与 shortcuts.json 里「接收最新」「接收指定ID」两处的 codeOnSuccess 逐字相同
// （verify.mjs 会断言那两处必须一致，所以只需要留一份）。

"""

# 抹掉 auth 变量的 value（用户自己的房间密码）。
#
# 用正则在**原始文本**上改，不重新序列化整个 JSON：这样 zip 里那份与你导出的文件逐字一致，
# 只少了密码那一个字符串 —— 「我导入的和导出的是同一份」这句话最好解释，也最不容易出岔子。
# （重序列化会把导出器写的 \u0026 变回 &、把缩进换掉，虽然语义等价，但 diff 起来就说不清了。）
AUTH_VALUE = re.compile(r'("key"\s*:\s*"auth"\s*,[^{}]*?"value"\s*:\s*)"(?:[^"\\]|\\.)*"')


def strip_auth(raw: str) -> tuple[str, bool]:
    """把 auth 变量的 value 置空，返回 (新文本, 是否真的改过)。"""
    data = json.loads(raw)
    auth = [v for v in data.get("variables", []) if v.get("key") == "auth"]
    if len(auth) != 1:
        raise SystemExit(f"!! variables 里应当有且只有一个 key=auth，实际 {len(auth)} 个")
    if not auth[0].get("value"):
        return raw, False

    new_raw, hits = AUTH_VALUE.subn(r'\1""', raw)
    if hits != 1:
        raise SystemExit(
            f"!! 没能定位 auth 的 value（正则匹配 {hits} 次）。\n"
            "   导出的 JSON 结构可能变了 —— 看 variables 里 auth 那项的字段顺序。"
        )

    # 反向校验：除了 auth 的 value，别的必须逐字段不变。
    # 没有这一步，一个写歪的正则会静默改掉别的东西。
    after = json.loads(new_raw)
    if data["categories"] != after["categories"]:
        raise SystemExit("!! 抹凭据时动到了捷径内容")
    for a, b in zip(data["variables"], after["variables"]):
        if a.get("key") == "auth":
            if b.get("value") != "":
                raise SystemExit("!! 抹掉后 auth 的 value 不是空串")
        elif a != b:
            raise SystemExit(f"!! 抹凭据时动到了别的变量: {a.get('key')}")
    return new_raw, True


def download_codes(raw: str) -> list[str]:
    """取出带下载逻辑的那几段 codeOnSuccess（正常是 2 段：接收最新 / 接收指定ID）。"""
    data = json.loads(raw)
    return [
        sc["codeOnSuccess"]
        for cat in data["categories"]
        for sc in cat["shortcuts"]
        if sc.get("codeOnSuccess") and "downloadUrl" in sc["codeOnSuccess"]
    ]


def main() -> int:
    if not os.path.exists(JSON_PATH):
        print(f"!! 找不到 {os.path.basename(JSON_PATH)}")
        print("   它是 HTTP Shortcuts 的原始导出：在应用里「导出」会得到 shortcuts.zip，")
        print("   解出里面的 shortcuts.json 放到本目录即可。")
        return 1

    raw = io.open(JSON_PATH, encoding="utf-8").read()

    # 1) 行为自检。那段 JS 是字符串存进 JSON 的，转义错了要导进手机才发现。
    if shutil.which("node"):
        proc = subprocess.run(
            ["node", "--no-warnings", VERIFY, JSON_PATH], capture_output=True, text=True,
        )
        print((proc.stdout or proc.stderr).rstrip())
        if proc.returncode != 0:
            print("!! 自检没过，别把这份 JSON 导进手机")
            return 1
    else:
        print("!! 找不到 node，跳过下载 URL 自检（建议装上再跑一次）")

    # 2) 导出可读镜像（只写不读）
    codes = download_codes(raw)
    io.open(MIRROR_PATH, "w", encoding="utf-8").write(MIRROR_HEADER + codes[0])
    print(f"✓ 已导出可读镜像 {os.path.basename(MIRROR_PATH)}（{len(codes[0])} 字符，别手改）")

    # 3) 抹掉凭据
    portable, changed = strip_auth(raw)
    if not changed:
        print("· auth 的 value 本来就是空的，无需抹除")

    # 4) 打包。临时文件写系统临时目录，别在工作区里留东西。
    fd, tmp = tempfile.mkstemp(suffix=".json")
    os.close(fd)
    try:
        io.open(tmp, "w", encoding="utf-8").write(portable)
        with zipfile.ZipFile(ZIP_PATH, "w", zipfile.ZIP_DEFLATED) as zf:
            zf.write(tmp, "shortcuts.json")
    finally:
        os.remove(tmp)

    names = [sc["name"] for cat in json.loads(raw)["categories"] for sc in cat["shortcuts"]]
    print(f"✓ 已生成 {os.path.basename(ZIP_PATH)}（{len(names)} 条捷径，auth 的 value 已抹空）")
    print("  内含:", ", ".join(names))
    print("  导入后在应用里把 auth（房间密码）填上。")

    # url 变量是导出时的服务器地址，会跟着进仓库产物。这里每次都显式打出来，
    # 免得「仓库里的包带着我家的局域网地址」这种事悄悄发生。
    urls = [v.get("value") for v in json.loads(raw)["variables"] if v.get("key") == "url"]
    if urls and urls[0]:
        print(f"  ⚠ url 变量仍是导出时的地址：{urls[0]}（要对外发就把这里换掉再导出）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
