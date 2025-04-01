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
o.description = translate("如果设置，访问时需要输入此密码。留空表示不需要密码。")

-- 服务状态和控制
s = m:section(TypedSection, "cloud-clipboard", translate("服务控制"))
s.anonymous = true

-- 使用更精确的命令行检测服务状态
local running = (luci.sys.call("pgrep -f '^/usr/bin/cloud-clipboard' >/dev/null") == 0)
local status_text = running and translate("运行中") or translate("未运行")
local status_code = running and "running" or "stopped"  -- 用代码而非文本判断状态

o = s:option(DummyValue, "_status", translate("运行状态"))
o.value = status_text
o.rawhtml = true
o.template = "cloud-clipboard/status"

-- 启动按钮：只在停止状态显示
o = s:option(Button, "_start", translate("启动"))
o:depends("_status", translate("未运行"))  -- 这行可能不生效
if not running then  -- 添加额外判断，确保按钮显示
    o.inputtitle = translate("启动服务")
    o.inputstyle = "apply"
    o.write = function()
        luci.sys.call("/etc/init.d/cloud-clipboard start >/dev/null")
        luci.http.redirect(luci.dispatcher.build_url("admin", "services", "cloud-clipboard"))
    end
end

-- 停止按钮：只在运行状态显示
o = s:option(Button, "_stop", translate("停止"))
o:depends("_status", translate("运行中"))  -- 这行可能不生效
if running then  -- 添加额外判断，确保按钮显示
    o.inputtitle = translate("停止服务")
    o.inputstyle = "reset"
    o.write = function()
        luci.sys.call("/etc/init.d/cloud-clipboard stop >/dev/null") 
        luci.http.redirect(luci.dispatcher.build_url("admin", "services", "cloud-clipboard"))
    end
end

-- 重启按钮：只在运行状态显示
o = s:option(Button, "_restart", translate("重启"))
o:depends("_status", translate("运行中"))  -- 这行可能不生效
if running then  -- 添加额外判断，确保按钮显示
    o.inputtitle = translate("重启服务")
    o.inputstyle = "reload"
    o.write = function()
        luci.sys.call("/etc/init.d/cloud-clipboard restart >/dev/null")
        luci.http.redirect(luci.dispatcher.build_url("admin", "services", "cloud-clipboard"))
    end
end

-- 访问服务
s = m:section(TypedSection, "cloud-clipboard", translate("服务访问"))
s.anonymous = true

o = s:option(DummyValue, "_access", translate("Web界面"))
o.template = "cloud-clipboard/access"

return m