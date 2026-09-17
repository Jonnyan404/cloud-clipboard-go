#!/usr/bin/env python3
"""把 multipart 表单里的字段改成「文件」类型，并指向指定的文件变量。

为什么需要它（长期必需，不是临时补丁）：
    Send 发送「真文件」时走 multipart 表单上传，让 part 自带真实文件名（含扩展名）——
    因为 getName 会把扩展名剥掉，而客户端没有任何动作能读到扩展名，服务端按内容嗅探
    也只能给 txt（.md/.sh/.txt 内容都是合法 UTF-8）。

    但 cherri 表达不了这件事：
      · formRequest 的 body / headers 声明为 dictionary!（只收内联字面量），传变量会报
        "Shortcuts does not allow variable values for this argument"
      · 它的字典只能生成 text token，做不出「文件」类型的字段
    所以源码里先用字面量 "PLACEHOLDER" 占位，构建后由本脚本改造。

结构来自 2026-09-17 在捷径 App 里手工建的文件字段样本（CC-Form-Sample）：
    "WFItemType": 5,                       # 5 = 文件（文本字段没有这个键）
    "WFValue": {
        "Value": {
            "Value": {"Type": "Variable", "VariableName": "item"},
            "WFSerializationType": "WFTextTokenAttachment"
        },
        "WFSerializationType": "WFTokenAttachmentParameterState"
    }
注意 WFValue 外层是 WFTokenAttachmentParameterState，不是文本字段那种 WFTextTokenString。
漏掉 WFItemType=5 会让服务端报「无法解析表单数据」（body 不是合法 multipart）。

用法：patch_form_field.py <unsigned.shortcut> [变量名，默认 item] [字段名，默认 file]
     没有占位符时静默跳过（对不含表单上传的源码安全）。
"""
import plistlib
import sys

PLACEHOLDER = "PLACEHOLDER"
FILE_ITEM_TYPE = 5


def patch(path: str, variable: str = "item", field: str = "file") -> int:
    with open(path, "rb") as f:
        workflow = plistlib.load(f)

    patched = 0
    for action in workflow.get("WFWorkflowActions", []):
        params = action.get("WFWorkflowActionParameters") or {}
        if params.get("WFHTTPBodyType") != "Form":
            continue
        form = params.get("WFFormValues")
        if not isinstance(form, dict):
            continue
        items = (form.get("Value") or {}).get("WFDictionaryFieldValueItems") or []
        for item in items:
            value = item.get("WFValue") or {}
            inner = value.get("Value") or {}
            if inner.get("string") != PLACEHOLDER:
                continue

            # 字段名（服务端 r.FormFile("file") / formData.get("file") 认这个名字）
            item["WFKey"] = {
                "Value": {"string": field},
                "WFSerializationType": "WFTextTokenString",
            }
            # 类型标记：5 = 文件
            item["WFItemType"] = FILE_ITEM_TYPE
            # 值：指向文件变量的 attachment
            item["WFValue"] = {
                "Value": {
                    "Value": {"Type": "Variable", "VariableName": variable},
                    "WFSerializationType": "WFTextTokenAttachment",
                },
                "WFSerializationType": "WFTokenAttachmentParameterState",
            }
            patched += 1

    if patched == 0:
        return 0

    with open(path, "wb") as f:
        plistlib.dump(workflow, f)
    return patched


if __name__ == "__main__":
    if len(sys.argv) < 2:
        raise SystemExit(__doc__)
    n = patch(
        sys.argv[1],
        sys.argv[2] if len(sys.argv) > 2 else "item",
        sys.argv[3] if len(sys.argv) > 3 else "file",
    )
    if n:
        print(f"   表单字段已改造: {n} 个 -> 名 {sys.argv[3] if len(sys.argv) > 3 else 'file'!r} / "
              f"变量 {sys.argv[2] if len(sys.argv) > 2 else 'item'!r} / WFItemType={FILE_ITEM_TYPE}")
