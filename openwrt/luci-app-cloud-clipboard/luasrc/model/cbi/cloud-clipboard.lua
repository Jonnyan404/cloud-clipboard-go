local m, s, o

m = Map("cloud-clipboard", translate("Cloud Clipboard"),
    translate("Cloud Clipboard是一个文本和文件传输工具，可在多个设备之间共享剪贴板内容。"))

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
o.description = translate("如果设置，访问时需要输入此密码。留空表示不需要密码。启用 roomAuth 且房间密码为空字符串时，也会回退到这里。")

-- 添加配置文件路径设置
o = s:option(Value, "config", translate("配置文件路径"))
o.default = "/etc/cloud-clipboard/config.json"
o.rmempty = false
o.description = translate("高级选项：配置文件路径。房间密码、文件过期等高级参数请前往高级设置页面配置。") ..
                ' <a href="' ..
                luci.dispatcher.build_url("admin", "services", "cloud-clipboard", "advanced") ..
                '" class="cbi-button cbi-button-apply">' .. translate("打开高级设置") .. '</a>'

return m