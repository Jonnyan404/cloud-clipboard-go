#!/usr/bin/env python3
"""Repair Cherri's missing WFWorkflowImportQuestions ActionIndex.

Stock Cherri emits WFWorkflowImportQuestions without ActionIndex and leaves the
consuming gettext action's WFTextActionText empty.  Shortcuts needs each import
question to point at the action that holds its answer (Category=Parameter +
ParameterKey), otherwise the import-time config panel is not wired up.

This post-processor (same role as the Cloudflare variant's build workflow):
  1. reads `#question id "prompt" "default"` declarations from the .cherri source,
  2. finds the gettext actions with an empty WFTextActionText in the compiled plist
     (they appear in the same order the questions were consumed),
  3. pairs them 1:1, sets each question's ActionIndex and bakes the default value
     into that gettext action, and
  4. rewrites WFWorkflowImportQuestions in declaration order.

Run:  inject_import_questions.py <Name.cherri> <Name_unsigned.shortcut>
"""

import plistlib
import re
import sys


def read_questions(cherri_path):
    questions = []
    src = open(cherri_path, encoding="utf-8").read()
    for m in re.finditer(r'^#question\s+([A-Za-z0-9_]+)\s+"(.*)"\s+"(.*)"\s*$', src, re.M):
        questions.append({"id": m.group(1), "text": m.group(2), "default": m.group(3)})
    return questions


def empty_gettext_indexes(actions):
    idx = []
    for i, a in enumerate(actions):
        if a.get("WFWorkflowActionIdentifier") != "is.workflow.actions.gettext":
            continue
        t = a.get("WFWorkflowActionParameters", {}).get("WFTextActionText")
        if t == "" or t is None:
            idx.append(i)
    return idx


def patch(cherri_path, plist_path):
    questions = read_questions(cherri_path)
    data = plistlib.load(open(plist_path, "rb"))
    actions = data["WFWorkflowActions"]

    holders = empty_gettext_indexes(actions)
    if len(holders) != len(questions):
        raise SystemExit(
            f"mismatch: {len(questions)} #question(s) vs {len(holders)} empty gettext action(s) in {plist_path}"
        )

    rebuilt = []
    for q, action_index in zip(questions, holders):
        rebuilt.append({
            "ActionIndex": action_index,
            "Category": "Parameter",
            "DefaultValue": q["default"],
            "ParameterKey": "WFTextActionText",
            "Text": q["text"],
        })
        actions[action_index]["WFWorkflowActionParameters"]["WFTextActionText"] = q["default"]

    data["WFWorkflowImportQuestions"] = rebuilt
    data["WFWorkflowActions"] = actions
    plistlib.dump(data, open(plist_path, "wb"))
    print(f"patched {plist_path}: {len(rebuilt)} question(s) -> actions {holders}")


if __name__ == "__main__":
    if len(sys.argv) != 3:
        raise SystemExit(__doc__)
    patch(sys.argv[1], sys.argv[2])