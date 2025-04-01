module("luci.controller.cloud-clipboard", package.seeall)

function index()
    if not nixio.fs.access("/etc/config/cloud-clipboard") then
        return
    end
    
    -- 使用acl_depends明确指定依赖的ACL权限
    entry({"admin", "services", "cloud-clipboard"}, cbi("cloud-clipboard"), _("Cloud Clipboard"), 90).acl_depends = { "luci-app-cloud-clipboard" }
    
    -- 添加状态API
    entry({"admin", "services", "cloud-clipboard", "status"}, call("act_status")).leaf = true
end

-- 服务状态检查函数
function act_status()
    local e = {}
    e.running = luci.sys.call("pgrep -f 'cloud-clipboard' >/dev/null") == 0
    luci.http.prepare_content("application/json")
    luci.http.write_json(e)
end