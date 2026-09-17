#!/usr/bin/env python3
"""补齐 cherri 表达不了的 plist 结构。**长期必需，不是临时补丁。**

cherri 有两类东西做不到，都靠本脚本在构建后补齐：

一、扩展名动作的显式输入
    `is.workflow.actions.properties.files`（"获取文件详情 → 扩展名"）没有输入参数，
    吃的是「上一个动作的输出」。实测：紧跟条目之后仍拿不到扩展名（恒为空），
    **必须显式给 WFInput**。而 cherri 的 rawAction 参数字典只收内联字面量，
    表达不了「指向变量的输入」，所以构建后补：
        "WFInput": {"Value": {"Type": "Variable", "VariableName": "item"},
                    "WFSerializationType": "WFTextTokenAttachment"}

二、multipart 表单里的「文件」字段
    Send 发真文件时走表单上传（让 part 自带真实文件名）。但 cherri 的字典只能生成
    text token，做不出文件类型的字段，所以源码里用字面量 "PLACEHOLDER" 占位：
        "WFItemType": 5,                      # 5 = 文件（文本字段没有这个键）
        "WFValue": {"Value": {"Value": {"Type": "Variable", "VariableName": "item"},
                              "WFSerializationType": "WFTextTokenAttachment"},
                    "WFSerializationType": "WFTokenAttachmentParameterState"}
    注意 WFValue 外层是 WFTokenAttachmentParameterState，不是文本字段那种 WFTextTokenString。
    漏掉 WFItemType=5 会让服务端报「无法解析表单数据」（body 不是合法 multipart）。

结构均取自 2026-09-17 在捷径 App 里手工建的样本（CC-Form-Sample / CC-Type-Sample）。

用法：patch_shortcut.py <unsigned.shortcut> [变量名，默认 item] [字段名，默认 file]
     没有可补的结构时静默跳过（对所有源码安全）。
"""
import plistlib
import sys

PLACEHOLDER = "PLACEHOLDER"
FILE_ITEM_TYPE = 5
EXT_ACTION = "is.workflow.actions.properties.files"


def _variable_attachment(variable: str) -> dict:
    return {"Type": "Variable", "VariableName": variable}


def patch(path: str, variable: str = "item", field: str = "file") -> dict:
    with open(path, "rb") as f:
        workflow = plistlib.load(f)

    ext_inputs = 0
    form_fields = 0
    fixed_idents = 0

    for action in workflow.get("WFWorkflowActions", []):
        ident = action.get("WFWorkflowActionIdentifier")
        params = action.get("WFWorkflowActionParameters") or {}

        # 零、修 rawAction 没覆盖成功的标识符
        # cherri 的 rawAction 会用第一个参数覆盖动作标识符（action.go 里的 overrideIdentifier），
        # 但实测不稳定：同一个写法在诊断 v1 生效、v2/v3 没生效，产物里留下占位标识符
        # is.workflow.actions.rawaction。这里按参数特征还原成真正的标识符。
        if ident == "is.workflow.actions.rawaction" and "WFContentItemPropertyName" in params:
            action["WFWorkflowActionIdentifier"] = EXT_ACTION
            ident = EXT_ACTION
            fixed_idents += 1

        # 一、扩展名动作的显式输入
        if ident == EXT_ACTION and "WFInput" not in params:
            params["WFInput"] = {
                "Value": _variable_attachment(variable),
                "WFSerializationType": "WFTextTokenAttachment",
            }
            action["WFWorkflowActionParameters"] = params
            ext_inputs += 1

        # 二、表单里的文件字段
        if params.get("WFHTTPBodyType") != "Form":
            continue
        form = params.get("WFFormValues")
        if not isinstance(form, dict):
            continue
        items = (form.get("Value") or {}).get("WFDictionaryFieldValueItems") or []
        for item in items:
            inner = (item.get("WFValue") or {}).get("Value") or {}
            if inner.get("string") != PLACEHOLDER:
                continue
            item["WFKey"] = {"Value": {"string": field}, "WFSerializationType": "WFTextTokenString"}
            item["WFItemType"] = FILE_ITEM_TYPE
            item["WFValue"] = {
                "Value": {
                    "Value": _variable_attachment(variable),
                    "WFSerializationType": "WFTextTokenAttachment",
                },
                "WFSerializationType": "WFTokenAttachmentParameterState",
            }
            form_fields += 1

    if ext_inputs == 0 and form_fields == 0 and fixed_idents == 0:
        return {"ext_inputs": 0, "form_fields": 0, "fixed_idents": 0}

    with open(path, "wb") as f:
        plistlib.dump(workflow, f)
    return {"ext_inputs": ext_inputs, "form_fields": form_fields, "fixed_idents": fixed_idents}


if __name__ == "__main__":
    if len(sys.argv) < 2:
        raise SystemExit(__doc__)
    result = patch(
        sys.argv[1],
        sys.argv[2] if len(sys.argv) > 2 else "item",
        sys.argv[3] if len(sys.argv) > 3 else "file",
    )
    if result["ext_inputs"] or result["form_fields"] or result["fixed_idents"]:
        print(f"   补 plist 结构: 修标识符 {result['fixed_idents']} 处、"
              f"扩展名动作输入 {result['ext_inputs']} 处、表单文件字段 {result['form_fields']} 个")
