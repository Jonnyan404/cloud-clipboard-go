#!/usr/bin/env python3
# 从 shortcuts.json 生成可导入、可分享的 shortcuts.zip。
#
# 为什么以 shortcuts.json 为准：它就是 HTTP Shortcuts 真正读写的东西。改捷径的流程是
# 「在应用里改 → 导出成 shortcuts.json 丢回来」，**不存在第二份要跟它对齐的源**。
# （试过两条「多一份」的路：先把 shortcuts.js 注入 JSON，后来改成反向导出可读镜像 ——
#   两条都被否掉了。两份同内容的东西必然会漂，哪怕其中一份是生成的。）
#
# 本脚本只做三件事：自检 → 换掉属于导出者的值 → 打包。
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
import time
import zipfile

HERE = os.path.dirname(os.path.abspath(__file__))
JSON_PATH = os.path.join(HERE, "shortcuts.json")
ZIP_PATH = os.path.join(HERE, "shortcuts.zip")
VERIFY = os.path.join(HERE, "verify.mjs")

# 入库的 zip 里要替换掉的、属于导出者自己的值。
#
# 用正则在**原始文本**上改，不重新序列化整个 JSON：这样 zip 里那份与你导出的文件逐字
# 一致，只有这几个字符串不同 —— 「导入的和导出的是同一份」最好解释，也最不容易出岔子。
# （重序列化会把导出器写的 \u0026 变回 &、把缩进换掉，语义等价，但 diff 起来就说不清了。）
SANITIZE = {
    "auth": "",                        # 房间密码：你自己的
    "url": "http://your-server:9501",  # 服务器地址：导出时是你自己的地址
}
# 只匹配「同一个对象里 key 紧接着 value」这种形状；跨对象（[^{}]*?）是为了容忍中间字段
VALUE_OF = {
    key: re.compile(r'("key"\s*:\s*"' + re.escape(key) + r'"\s*,[^{}]*?"value"\s*:\s*)"(?:[^"\\]|\\.)*"')
    for key in SANITIZE
}


def sanitize(raw: str) -> tuple[str, list[str]]:
    """换掉属于导出者的值，返回 (新文本, 实际改过的 key)。"""
    before = json.loads(raw)
    text = raw
    changed = []

    for key, replacement in SANITIZE.items():
        var = [v for v in before.get("variables", []) if v.get("key") == key]
        if len(var) != 1:
            raise SystemExit(f"!! variables 里应当有且只有一个 key={key}，实际 {len(var)} 个")
        if str(var[0].get("value", "")) == replacement:
            continue
        text, hits = VALUE_OF[key].subn(lambda m: m.group(1) + json.dumps(replacement), text)
        if hits != 1:
            raise SystemExit(
                f"!! 没能定位 {key} 的 value（正则匹配 {hits} 次）。\n"
                f"   导出的 JSON 结构可能变了 —— 看 variables 里 {key} 那项的字段顺序。"
            )
        changed.append(key)

    # 反向校验：除了上面那几个值，别的必须逐字段不变。
    # 没有这一步，一个写歪的正则会静默改掉别的东西。
    after = json.loads(text)
    if before["categories"] != after["categories"]:
        raise SystemExit("!! 替换时动到了捷径内容")
    for a, b in zip(before["variables"], after["variables"]):
        if a.get("key") in SANITIZE:
            if b.get("value") != SANITIZE[a["key"]]:
                raise SystemExit(f"!! {a['key']} 替换后不是预期值")
        elif a != b:
            raise SystemExit(f"!! 替换时动到了别的变量: {a.get('key')}")
    return text, changed


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

    # 2) 换掉属于导出者的值（只影响 zip，不动 shortcuts.json）
    portable, changed = sanitize(raw)
    if changed:
        print("✓ 已替换: " + ", ".join(f"{k} → {SANITIZE[k]!r}" for k in changed))
    else:
        print("· 没有需要替换的值")

    # 3) 打包。
    #
    # 时间戳用**当前时间**，故意不锁死：要能一眼看出产物有没有被重新生成
    # （2026-09-18 Jonny 明确要求）。代价是同一份 JSON 跑两次字节不同、git 里每次都是
    # modified —— 这是他要的，别「优化」回去。
    info = zipfile.ZipInfo("shortcuts.json", date_time=time.localtime()[:6])
    info.compress_type = zipfile.ZIP_DEFLATED
    info.external_attr = 0o644 << 16
    with zipfile.ZipFile(ZIP_PATH, "w", zipfile.ZIP_DEFLATED) as zf:
        zf.writestr(info, portable.encode("utf-8"))

    names = [sc["name"] for cat in json.loads(raw)["categories"] for sc in cat["shortcuts"]]
    export_time = time.strftime("%Y-%m-%d %H:%M", time.localtime(os.path.getmtime(JSON_PATH)))
    print(
        f"✓ 已生成 {os.path.basename(ZIP_PATH)}"
        f"（{len(names)} 条捷径，{os.path.getsize(ZIP_PATH)} 字节，"
        f"构建于 {time.strftime('%H:%M:%S')}）"
    )
    print(f"  源导出时间: {export_time}")   # 「zip 是不是跟着最新导出走的」看这一行
    print("  内含:", ", ".join(names))
    print("  导入后在应用里把 auth（房间密码）和 url（服务器地址）填上。")
    return 0


if __name__ == "__main__":
    sys.exit(main())
