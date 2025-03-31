module("luci.controller.cloud-clipboard", package.seeall)

function index()
    if not nixio.fs.access("/etc/config/cloud-clipboard") then
        return
    end
    
    entry({"admin", "services", "cloud-clipboard"}, cbi("cloud-clipboard"), _("Cloud Clipboard"), 90).dependent = true
    entry({"admin", "services", "cloud-clipboard", "status"}, call("act_status")).leaf = true
end

function act_status()
    local e = {}
    e.running = luci.sys.call("pgrep -f 'cloud-clipboard' >/dev/null") == 0
    luci.http.prepare_content("application/json")
    luci.http.write_json(e)
end