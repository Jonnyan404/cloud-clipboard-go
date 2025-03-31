module("luci.controller.cloud-clipboard", package.seeall)

function index()
    if not nixio.fs.access("/etc/config/cloud-clipboard") then
        return
    end

    local page = entry({"admin", "services", "cloud-clipboard"}, cbi("cloud-clipboard"), _("Cloud Clipboard"), 100)
    page.dependent = true
    page.acl_depends = { "luci-app-cloud-clipboard" }
    
    entry({"admin", "services", "cloud-clipboard", "status"}, call("act_status")).leaf = true
    entry({"admin", "services", "cloud-clipboard", "log"}, call("act_log")).leaf = true
end

function act_status()
    local e = {}
    e.running = luci.sys.call("pgrep -f cloud-clipboard >/dev/null") == 0
    luci.http.prepare_content("application/json")
    luci.http.write_json(e)
end

function act_log()
    local logfile = "/var/log/cloud-clipboard.log"
    if nixio.fs.access(logfile) then
        local content = nixio.fs.readfile(logfile) or ""
        luci.http.prepare_content("text/plain")
        luci.http.write(content)
    else
        luci.http.prepare_content("text/plain")
        luci.http.write("日志文件不存在")
    end
end