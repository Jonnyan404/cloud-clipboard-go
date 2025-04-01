module("luci.controller.cloud-clipboard", package.seeall)

function index()
    if not nixio.fs.access("/etc/config/cloud-clipboard") then
        return
    end
    
    -- 注册主菜单和多个子页面
    entry({"admin", "services", "cloud-clipboard"}, firstchild(), _("Cloud Clipboard"), 90).dependent = true
    
    -- 设置页面
    entry({"admin", "services", "cloud-clipboard", "settings"}, cbi("cloud-clipboard"), _("设置"), 10).leaf = true
    
    -- 日志页面
    entry({"admin", "services", "cloud-clipboard", "log"}, template("cloud-clipboard/log"), _("日志"), 20).leaf = true
    
    -- API接口
    entry({"admin", "services", "cloud-clipboard", "status"}, call("act_status")).leaf = true
    entry({"admin", "services", "cloud-clipboard", "getlog"}, call("get_log")).leaf = true
    entry({"admin", "services", "cloud-clipboard", "clearlog"}, call("clear_log")).leaf = true
end

-- 服务状态检查函数
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