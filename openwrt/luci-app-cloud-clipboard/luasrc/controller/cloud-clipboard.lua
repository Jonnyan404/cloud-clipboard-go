module("luci.controller.cloud-clipboard", package.seeall)

function index()
    if not nixio.fs.access("/etc/config/cloud-clipboard") then
        return
    end
    
    -- 使用acl_depends明确指定依赖的ACL权限
    entry({"admin", "services", "cloud-clipboard"}, cbi("cloud-clipboard"), _("Cloud Clipboard"), 90).acl_depends = { "luci-app-cloud-clipboard" }
    
    -- 添加状态API
    entry({"admin", "services", "cloud-clipboard", "status"}, call("act_status")).leaf = true
    
    -- 添加日志查看功能 (页面形式)
    entry({"admin", "services", "cloud-clipboard", "logview"}, template("cloud-clipboard/logview")).leaf = true

    -- 添加日志内容API
    entry({"admin", "services", "cloud-clipboard", "getlog"}, call("get_log")).leaf = true

    -- 添加日志清除API
    entry({"admin", "services", "cloud-clipboard", "clearlog"}, call("clear_log")).leaf = true
end

-- 服务状态检查函数 - 使用更精确的匹配
function act_status()
    local e = {}
    e.running = luci.sys.call("pgrep -f '^/usr/bin/cloud-clipboard' >/dev/null") == 0
    luci.http.prepare_content("application/json")
    luci.http.write_json(e)
end

-- 日志读取函数
function get_log()
    local logfile = "/var/log/cloud-clipboard.log"
    local ldata = nixio.fs.readfile(logfile) or ""
    if #ldata == 0 then
        ldata = "日志为空"
    end
    luci.http.prepare_content("text/plain; charset=utf-8")
    luci.http.write(ldata)
end

-- 日志清除函数
function clear_log()
    luci.sys.call("> /var/log/cloud-clipboard.log")
    luci.http.prepare_content("application/json")
    luci.http.write('{"result":"success"}')
end