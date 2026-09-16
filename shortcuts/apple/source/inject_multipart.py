#!/usr/bin/env python3
import plistlib, sys, copy, json

TEMPLATE = {
    "Value": {
        "WFDictionaryFieldValueItems": [
            {
                "WFItemType": 1,
                "WFKey": {"Value": {"string": "Value"}, "WFSerializationType": "WFTextTokenString"},
                "WFValue": {
                    "Value": {
                        "WFDictionaryFieldValueItems": [
                            {
                                "WFItemType": 2,
                                "WFKey": {"Value": {"string": "WFDictionaryFieldValueItems"}, "WFSerializationType": "WFTextTokenString"},
                                "WFValue": {
                                    "Value": [
                                        {
                                            "WFItemType": 1,
                                            "WFValue": {
                                                "Value": {
                                                    "WFDictionaryFieldValueItems": [
                                                        {
                                                            "WFItemType": 1,
                                                            "WFKey": {"Value": {"string": "WFKey"}, "WFSerializationType": "WFTextTokenString"},
                                                            "WFValue": {
                                                                "Value": {
                                                                    "WFDictionaryFieldValueItems": [
                                                                        {"WFKey": {"Value": {"string": "Value"}, "WFSerializationType": "WFTextTokenString"},
                                                                         "WFValue": {"Value": {"string": "file"}, "WFSerializationType": "WFTextTokenString"}},
                                                                        {"WFKey": {"Value": {"string": "WFSerializationType"}, "WFSerializationType": "WFTextTokenString"},
                                                                         "WFValue": {"Value": {"string": "WFTextTokenString"}, "WFSerializationType": "WFTextTokenString"}}
                                                                    ]
                                                                },
                                                                "WFSerializationType": "WFDictionaryFieldValue"
                                                            }
                                                        },
                                                        {
                                                            "WFItemType": 3,
                                                            "WFKey": {"Value": {"string": "WFItemType"}, "WFSerializationType": "WFTextTokenString"},
                                                            "WFValue": {"Value": {"string": "5"}, "WFSerializationType": "WFTextTokenString"}
                                                        },
                                                        {
                                                            "WFItemType": 1,
                                                            "WFKey": {"Value": {"string": "WFValue"}, "WFSerializationType": "WFTextTokenString"},
                                                            "WFValue": {
                                                                "Value": {
                                                                    "WFDictionaryFieldValueItems": [
                                                                        {"WFKey": {"Value": {"string": "WFSerializationType"}, "WFSerializationType": "WFTextTokenString"},
                                                                         "WFValue": {"Value": {"string": "WFTokenAttachmentParameterState"}, "WFSerializationType": "WFTextTokenString"}},
                                                                        {
                                                                            "WFItemType": 1,
                                                                            "WFKey": {"Value": {"string": "Value"}, "WFSerializationType": "WFTextTokenString"},
                                                                            "WFValue": {
                                                                                "Value": {
                                                                                    "WFDictionaryFieldValueItems": [
                                                                                        {"WFKey": {"Value": {"string": "Value"}, "WFSerializationType": "WFTextTokenString"},
                                                                                         "WFValue": {"Value": {"VariableName": "fileToUpload", "Type": "Variable"}, "WFSerializationType": "WFTextTokenAttachment"}},
                                                                                        {"WFKey": {"Value": {"string": "WFSerializationType"}, "WFSerializationType": "WFTextTokenString"},
                                                                                         "WFValue": {"Value": {"string": "WFTextTokenAttachment"}, "WFSerializationType": "WFTextTokenString"}}
                                                                                    ]
                                                                                },
                                                                                "WFSerializationType": "WFDictionaryFieldValue"
                                                                            }
                                                                        }
                                                                    ]
                                                                },
                                                                "WFSerializationType": "WFDictionaryFieldValue"
                                                            }
                                                        }
                                                    ]
                                                },
                                                "WFSerializationType": "WFDictionaryFieldValue"
                                            }
                                        }
                                    ],
                                    "WFSerializationType": "WFArrayParameterState"
                                }
                            }
                        ]
                    },
                    "WFSerializationType": "WFDictionaryFieldValue"
                }
            },
            {
                "WFKey": {"Value": {"string": "WFSerializationType"}, "WFSerializationType": "WFTextTokenString"},
                "WFValue": {"Value": {"string": "WFDictionaryFieldValue"}, "WFSerializationType": "WFTextTokenString"}
            }
        ]
    },
    "WFSerializationType": "WFDictionaryFieldValue"
}

def main(path):
    with open(path, 'rb') as f:
        d = plistlib.load(f)
    injected = 0
    for a in d['WFWorkflowActions']:
        p = a.get('WFWorkflowActionParameters', {})
        if (a.get('WFWorkflowActionIdentifier') == 'is.workflow.actions.downloadurl'
                and p.get('WFHTTPMethod') == 'POST' and p.get('WFHTTPBodyType') == 'Form'):
            p['WFFormValues'] = copy.deepcopy(TEMPLATE)
            injected += 1
    with open(path, 'wb') as f:
        plistlib.dump(d, f)
    print(f'{path}: injected {injected} multipart action(s)')

if __name__ == '__main__':
    main(sys.argv[1])