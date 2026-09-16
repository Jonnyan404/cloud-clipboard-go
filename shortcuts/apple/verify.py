#!/usr/bin/env python3
"""构建后校验：把已知的坑变成断言，避免"产物看着像对的、其实少了东西"。

用法： verify.py <源码.cherri> <编译产物_unsigned.shortcut>

硬失败（退出码 1）：
  - 产物无法解析 / 没有 WFWorkflowActions
  - 导入问答缺失、数量与 #question 不符、或缺 ActionIndex
  - 条件里出现本地化类型名比较（原版 29 处那种写法，非中英文系统会静默走错分支）

提示（不失败）：
  - 用了 typeOf(getitemtype) / setName(setitemname)
  - WFWorkflowTypes 未包含 macOS 的 QuickActions / MenuBar 入口
"""

import collections
import plistlib
import re
import sys

# 条件里出现这些字符串，说明又在用"本地化类型名"判断类型了。
# 只收 CJK 词，避免把 `@room == "default"` 之类的正常比较误判。
LOCALIZED_TYPE_NAMES = [
    "图像", "图片", "照片", "文件", "文件夹", "视频", "影片", "音频", "压缩", "文本", "字符串",
]

# 一个条件里比较字符串 ≥ 此数量，基本就是"类型清单"链式比较。
TYPE_LIST_THRESHOLD = 3

MAC_ENTRY_TYPES = {"QuickActions", "MenuBar"}


def collect_compared_strings(obj, out):
    if isinstance(obj, dict):
        if "WFConditionalActionString" in obj and isinstance(obj["WFConditionalActionString"], str):
            out.append(obj["WFConditionalActionString"])
        for v in obj.values():
            collect_compared_strings(v, out)
    elif isinstance(obj, list):
        for v in obj:
            collect_compared_strings(v, out)


def compared_strings_by_conditional(actions):
    """按「单个条件动作」分组收集比较字符串。

    必须分组统计：若在整个动作列表上累加，几处正常的少量比较（如 `== "image"`、
    `== "Mac"`）会被凑成大数字，误报成类型清单链式比较。
    """
    groups = []
    for a in actions:
        if a.get("WFWorkflowActionIdentifier") != "is.workflow.actions.conditional":
            continue
        found = []
        collect_compared_strings(a.get("WFWorkflowActionParameters", {}), found)
        if found:
            groups.append(found)
    return groups


def main():
    if len(sys.argv) != 3:
        print(__doc__)
        return 2
    src_path, plist_path = sys.argv[1], sys.argv[2]

    failures, notes = [], []

    src = open(src_path, encoding="utf-8").read()
    declared = re.findall(r'^#question\s+([A-Za-z0-9_]+)', src, re.M)

    try:
        data = plistlib.load(open(plist_path, "rb"))
    except Exception as exc:
        print(f"✗ 无法解析 {plist_path}: {exc}")
        return 1

    actions = data.get("WFWorkflowActions")
    if not actions:
        print(f"✗ {plist_path} 没有 WFWorkflowActions")
        return 1

    print(f"── 校验 {plist_path}")
    print(f"   动作数: {len(actions)}")

    # 1. 导入问答
    questions = data.get("WFWorkflowImportQuestions", [])
    if len(questions) != len(declared):
        failures.append(
            f"导入问答数量不符：源码声明 {len(declared)} 个 #question，产物里有 {len(questions)} 个"
        )
    missing_index = [i for i, q in enumerate(questions) if not isinstance(q.get("ActionIndex"), int)]
    if missing_index:
        failures.append(
            f"导入问答缺 ActionIndex（第 {missing_index} 项）—— 说明 patcher 没跑或跑错了，"
            "导入时配置面板不会生效"
        )
    if questions:
        print(f"   导入问答: {len(questions)} 个，ActionIndex={[q.get('ActionIndex') for q in questions]}")

    # 2. 本地化类型名比较
    groups = compared_strings_by_conditional(actions)
    flat = [s for g in groups for s in g]
    bad_names = sorted({s for s in flat if s in LOCALIZED_TYPE_NAMES})
    if bad_names:
        failures.append(f"条件里出现本地化类型名比较：{bad_names}（非中英文系统会静默走错分支）")
    heavy = sorted((g for g in groups if len(g) >= TYPE_LIST_THRESHOLD), key=len, reverse=True)
    if heavy:
        failures.append(
            f"某个条件里比较了 {len(heavy[0])} 个字符串，疑似「类型清单」链式比较：{heavy[0][:8]}"
        )
    print(f"   条件比较: 共 {len(groups)} 处条件含比较，单处最多 {max((len(g) for g in groups), default=0)} 个字符串")

    # 3. 已知会出问题的动作
    ids = collections.Counter(a.get("WFWorkflowActionIdentifier") for a in actions)
    if ids.get("is.workflow.actions.getitemtype"):
        notes.append(f"使用了 typeOf（{ids['is.workflow.actions.getitemtype']} 次）—— 本机实测会闪变，别拿它做唯一判据")
    if ids.get("is.workflow.actions.setitemname"):
        notes.append(f"使用了 setName（{ids['is.workflow.actions.setitemname']} 次）—— 确认是有意为之（本项目里用于给下载文件恢复原始文件名）")

    # 4. 平台入口
    types = data.get("WFWorkflowTypes", [])
    print(f"   WFWorkflowTypes: {types}")
    print(f"   WFQuickActionSurfaces: {data.get('WFQuickActionSurfaces', [])}")
    print(f"   最低客户端版本: {data.get('WFWorkflowMinimumClientVersionString', '?')}")
    if not (MAC_ENTRY_TYPES & set(types)):
        notes.append(
            "WFWorkflowTypes 未包含 QuickActions / MenuBar —— macOS 的 Finder 快速操作与菜单栏里不会出现这个捷径。"
            "若需要，在源码里写 #define from sharesheet,quickactions,menubar"
        )

    # 5. 动作构成
    print("   动作构成:")
    for ident, count in ids.most_common():
        print(f"     {count:>4}  {ident}")

    for n in notes:
        print(f"   ! {n}")
    for f in failures:
        print(f"   ✗ {f}")

    print("   结果:", "失败" if failures else "通过")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
