local m, s, o

m = Map("cloud-clipboard", translate("Cloud Clipboard"), translate("Cloud Clipboard是一个文本和文件传输工具，可在多个设备之间共享剪贴板内容。"))

s = m:section(TypedSection, "cloud-clipboard", translate("基本设置"))
s.anonymous = true

o = s:option(Flag, "enabled", translate("启用"))
o.rmempty = false

o = s:option(Value, "host", translate("监听地址"))
o.datatype = "ip4addr"
o.default = "0.0.0.0"
o.rmempty = false

o = s:option(Value, "port", translate("监听端口"))
o.datatype = "port"
o.default = "9501"
o.rmempty = false

o = s:option(Value, "auth", translate("访问密码"))
o.password = true
o.rmempty = true
o.description = translate("如果设置，访问时需要输入此密码。留空表示不需要密码。")

s = m:section(TypedSection, "cloud-clipboard", translate("状态"))
s.anonymous = true

o = s:option(DummyValue, "_status", translate("运行状态"))
o.template = "cloud-clipboard/status"

o = s:option(Button, "_log", translate("查看日志"))
o.template = "cloud-clipboard/log"

return m