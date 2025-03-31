-- 修改 /luasrc/controller/cloud-clipboard.lua
module("luci.controller.cloud-clipboard", package.seeall)

function index()
    if not nixio.fs.access("/etc/config/cloud-clipboard") then
        return
    end
    
    -- 修改为明确指定节点类型和ACL依赖
    entry({"admin", "services", "cloud-clipboard"}, firstchild(), _("Cloud Clipboard"), 90)
    entry({"admin", "services", "cloud-clipboard", "settings"}, cbi("cloud-clipboard"), _("Settings"), 1)
    entry({"admin", "services", "cloud-clipboard", "status"}, call("act_status")).leaf = true
end

function act_status()
    local e = {}
    e.running = luci.sys.call("pgrep -f cloud-clipboard >/dev/null") == 0
    luci.http.prepare_content("application/json")
    luci.http.write_json(e)
end