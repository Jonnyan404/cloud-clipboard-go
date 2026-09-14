module("luci.controller.cloud-clipboard", package.seeall)

function index()
    if not nixio.fs.access("/etc/config/cloud-clipboard") then
        return
    end

    -- 注册主菜单和多个子页面
    entry({"admin", "services", "cloud-clipboard"}, firstchild(), _("Cloud Clipboard"), 90).dependent = true

    -- 总览页面
    entry({"admin", "services", "cloud-clipboard", "overview"}, template("cloud-clipboard/overview"), _("总览"), 10).leaf = true

    -- 基本设置页面
    entry({"admin", "services", "cloud-clipboard", "settings"}, template("cloud-clipboard/basic"), _("基本设置"), 20).leaf = true

    -- 高级设置页面
    entry({"admin", "services", "cloud-clipboard", "advanced"}, template("cloud-clipboard/advanced"), _("高级设置"), 30).leaf = true

    -- 日志页面
    entry({"admin", "services", "cloud-clipboard", "log"}, template("cloud-clipboard/log"), _("日志"), 50).leaf = true

    -- API接口
    entry({"admin", "services", "cloud-clipboard", "status"}, call("act_status")).leaf = true
    entry({"admin", "services", "cloud-clipboard", "version"}, call("act_version")).leaf = true
    entry({"admin", "services", "cloud-clipboard", "action"}, call("act_action")).leaf = true
    entry({"admin", "services", "cloud-clipboard", "getconfig"}, call("act_getconfig")).leaf = true
    entry({"admin", "services", "cloud-clipboard", "saveconfig"}, call("act_saveconfig")).leaf = true
    entry({"admin", "services", "cloud-clipboard", "getbasic"}, call("act_getbasic")).leaf = true
    entry({"admin", "services", "cloud-clipboard", "savebasic"}, call("act_savebasic")).leaf = true
    entry({"admin", "services", "cloud-clipboard", "getlog"}, call("get_log")).leaf = true
    entry({"admin", "services", "cloud-clipboard", "clearlog"}, call("clear_log")).leaf = true
end

local CONFIG_DIR = "/etc/cloud-clipboard"
local DEFAULT_PATHS = {
    history = CONFIG_DIR .. "/data/history.json",
    storage = CONFIG_DIR .. "/data/upload"
}

local function conf_path()
    local uci = luci.model.uci.cursor()
    return uci:get("cloud-clipboard", "main", "config") or CONFIG_DIR .. "/config.json"
end

-- 解析 JSON（兼容 luci.jsonc / luci.json）
local function parse_json(text)
    local json = nil
    local ok1, j1 = pcall(require, "luci.jsonc")
    if ok1 and j1 then json = j1 else
        local ok2, j2 = pcall(require, "luci.json")
        if ok2 and j2 then json = j2 end
    end
    if not json then return nil end
    local ok, obj = pcall(function()
        if json.parse then return json.parse(text) end
        if json.decode then return json.decode(text) end
    end)
    if not ok then return nil end
    return obj
end

local function default_config()
    return {
        server = {
            historyFile = DEFAULT_PATHS.history,
            storageDir = DEFAULT_PATHS.storage
        },
        text = {},
        file = {}
    }
end

local function read_config()
    local p = conf_path()
    if not nixio.fs.access(p) then return default_config() end
    local ok, text = pcall(nixio.fs.readfile, p)
    if not ok or not text or text == "" then return default_config() end
    local obj = parse_json(text)
    if not obj then return default_config() end
    return obj
end

-- 目录占用的 KB（du -sk）
local function du_kb(path)
    if not nixio.fs.access(path) then return 0 end
    local out = luci.sys.exec("du -sk '" .. path .. "' 2>/dev/null")
    local kb = out:match("(%d+)[%s]+")
    return tonumber(kb) or 0
end

local function get_pid()
    local out = luci.sys.exec("pgrep -f '^/usr/bin/cloud-clipboard' | head -n1")
    local pid = out:match("%d+")
    return pid and tonumber(pid) or nil
end

local function get_installed_version()
    local out = luci.sys.exec("/usr/bin/cloud-clipboard -v 2>/dev/null")
    local ver = out:match("[vV]?([%d%.]+[%w%._%-]*)")
    if ver then
        return "v" .. ver
    end
    local uci = luci.model.uci.cursor()
    local uv = uci:get("cloud-clipboard", "main", "version") or ""
    if uv ~= "" then return "v" .. uv end
    return ""
end

local function get_update_repo()
    local uci = luci.model.uci.cursor()
    return uci:get("cloud-clipboard", "main", "update_repo") or "Jonnyan404/cloud-clipboard-go"
end

-- 从 GitHub Releases 获取最新版本（带本地缓存，10 分钟过期）
local function fetch_latest(repo, force)
    local cache = "/tmp/cloud-clipboard/version.json"
    local json = nil
    local ok1, j1 = pcall(require, "luci.jsonc")
    if ok1 and j1 then json = j1 else
        local ok2, j2 = pcall(require, "luci.json")
        if ok2 and j2 then json = j2 end
    end

    local function read_cache()
        if not nixio.fs.access(cache) then return nil end
        local st = nixio.fs.stat(cache)
        if st and st.mtime and os.time() - st.mtime > 600 then return nil end
        local ok, text = pcall(nixio.fs.readfile, cache)
        if not ok or not text then return nil end
        local obj = json and parse_json(text) or nil
        return obj
    end

    local cached = read_cache()
    if cached and cached.tag_name then
        return cached.tag_name, cached.html_url, false
    end

    if not force then
        -- 有旧缓存但已过期：继续用旧缓存作为兜底，避免离线时失败
        if nixio.fs.access(cache) then
            local ok, text = pcall(nixio.fs.readfile, cache)
            if ok and text then
                local obj = json and parse_json(text) or nil
                if obj and obj.tag_name then
                    return obj.tag_name, obj.html_url, true
                end
            end
        end
    end

    local api_url = string.format("https://api.github.com/repos/%s/releases/latest", repo)
    local data = ""
    data = luci.sys.exec(string.format("curl -fsSL --max-time 8 '%s' 2>/dev/null", api_url))
    if not data or data == "" then
        data = luci.sys.exec(string.format("wget -qO- --timeout=8 '%s' 2>/dev/null", api_url))
    end
    data = data or ""

    if data ~= "" and json and json.parse then
        local obj = json.parse(data)
        if obj and obj.tag_name then
            nixio.fs.mkdir("/tmp/cloud-clipboard")
            local cache_json = data:gsub("^%s+", "")
            pcall(nixio.fs.writefile, cache, cache_json)
            return obj.tag_name, obj.html_url, false
        end
    end
    return nil, nil, "无法从 GitHub 获取最新版本"
end

-- 服务状态检查
function act_status()
    local running = (luci.sys.call("pgrep -f '^/usr/bin/cloud-clipboard' >/dev/null") == 0)
    local uci = luci.model.uci.cursor()
    local conf = read_config()
    local srv = conf.server or {}
    local txt = conf.text or {}
    local fl = conf.file or {}

    local host = uci:get("cloud-clipboard", "main", "host") or ""
    local port = uci:get("cloud-clipboard", "main", "port") or ""
    local auth = uci:get("cloud-clipboard", "main", "auth") or ""
    if host == "" then
        host = (srv.host ~= "" and tostring(srv.host)) or "0.0.0.0"
    end
    local cport = tonumber(srv.port) or 0
    if port == "" or tonumber(port) == nil then port = tostring(cport) end

    local summary = {
        roomList = srv.roomList == true,
        history = tonumber(srv.history) or 100,
        textLimit = tonumber(txt.limit) or 4096,
        fileLimit = tonumber(fl.limit) or 268435456
    }

    local e = {
        running = running,
        pid = get_pid(),
        host = host,
        port = port,
        auth = (auth ~= "" or srv.auth == true),
        version = get_installed_version(),
        usage = {
            total = du_kb(CONFIG_DIR) * 1024,
            upload = du_kb(CONFIG_DIR .. "/data/upload") * 1024,
            history = du_kb(CONFIG_DIR .. "/data") * 1024
        },
        summary = summary
    }
    luci.http.prepare_content("application/json")
    luci.http.write_json(e)
end

-- 版本检查
function act_version()
    local force = (luci.http.formvalue("force") == "1")
    local installed = get_installed_version()
    local repo = get_update_repo()
    local latest, release_url, err = fetch_latest(repo, force)

    local e = {
        installed = installed,
        latest = latest or "",
        release_url = release_url or "",
        update = (latest ~= nil and latest ~= "" and latest ~= installed),
        repo = repo,
        error = err
    }
    luci.http.prepare_content("application/json")
    luci.http.write_json(e)
end

-- 服务动作: start / stop / restart / reload
function act_action()
    local a = luci.http.formvalue("act") or ""
    if a ~= "" then
        a = a:match("^%w+$") or ""
    end
    local ok = false
    if a == "start" then
        ok = (luci.sys.call("/etc/init.d/cloud-clipboard start >/dev/null 2>&1") == 0)
    elseif a == "stop" then
        ok = (luci.sys.call("/etc/init.d/cloud-clipboard stop >/dev/null 2>&1") == 0)
    elseif a == "restart" then
        ok = (luci.sys.call("/etc/init.d/cloud-clipboard restart >/dev/null 2>&1") == 0)
    elseif a == "reload" then
        ok = (luci.sys.call("/etc/init.d/cloud-clipboard reload >/dev/null 2>&1") == 0)
    elseif a == "clearhistory" then
        local conf = read_config()
        local hf = (conf.server and conf.server.historyFile) or DEFAULT_PATHS.history
        local dir = hf:match("(.+)/[^/]+")
        if dir and not nixio.fs.access(dir) then nixio.fs.mkdir(dir) end
        local wok = pcall(nixio.fs.writefile, hf, "[]\n")
        ok = wok
    end
    luci.http.prepare_content("application/json")
    luci.http.write_json({ ok = ok, act = a })
end

-- 读取当前配置（供高级设置页使用）
function act_getconfig()
    local e = {
        path = conf_path(),
        config = read_config()
    }
    luci.http.prepare_content("application/json")
    luci.http.write_json(e)
end

-- 保存完整配置 JSON
function act_saveconfig()
    local p = conf_path()
    local dir = p:match("(.+)/[^/]+")
    if dir and not nixio.fs.access(dir) then
        nixio.fs.mkdir(dir)
    end

    local body = luci.http.content() or ""
    if body == "" then body = luci.http.formvalue("json") or "" end

    local obj = parse_json(body)
    if not obj or type(obj) ~= "table" then
        luci.http.prepare_content("application/json")
        luci.http.write_json({ ok = false, error = "无效的 JSON 格式" })
        return
    end

    local j = nil
    local ok1, j1 = pcall(require, "luci.jsonc")
    if ok1 and j1 then j = j1 else
        local ok2, j2 = pcall(require, "luci.json")
        if ok2 and j2 then j = j2 end
    end
    local text = body
    if j and j.stringify then
        text = j.stringify(obj, true)
    end

    local wok, werr = pcall(nixio.fs.writefile, p, text)
    if not wok then
        luci.http.prepare_content("application/json")
        luci.http.write_json({ ok = false, error = "写入失败: " .. tostring(werr) })
        return
    end
    luci.http.prepare_content("application/json")
    luci.http.write_json({ ok = true, path = p })
end

-- 读取基本设置（UCI）
function act_getbasic()
    local uci = luci.model.uci.cursor()
    local e = {
        enabled = (uci:get("cloud-clipboard", "main", "enabled") or "1"),
        host = uci:get("cloud-clipboard", "main", "host") or "0.0.0.0",
        port = uci:get("cloud-clipboard", "main", "port") or "9501",
        auth = uci:get("cloud-clipboard", "main", "auth") or "",
        config = uci:get("cloud-clipboard", "main", "config") or (CONFIG_DIR .. "/config.json")
    }
    luci.http.prepare_content("application/json")
    luci.http.write_json(e)
end

-- 保存基本设置（UCI）并启停服务
function act_savebasic()
    local body = luci.http.content() or ""
    if body == "" then body = luci.http.formvalue("json") or "" end
    local obj = parse_json(body)
    if not obj or type(obj) ~= "table" then
        luci.http.prepare_content("application/json")
        luci.http.write_json({ ok = false, error = "无效的 JSON 格式" })
        return
    end

    local enabled = tostring(obj.enabled)
    if enabled ~= "0" and enabled ~= "1" then enabled = "1" end
    local host = tostring(obj.host or "0.0.0.0")
    if host == "" then host = "0.0.0.0" end
    local port = tostring(tonumber(obj.port) or 9501)
    local auth = tostring(obj.auth or "")
    local config = tostring(obj.config or (CONFIG_DIR .. "/config.json"))

    local uci = luci.model.uci.cursor()
    uci:set("cloud-clipboard", "main", "enabled", enabled)
    uci:set("cloud-clipboard", "main", "host", host)
    uci:set("cloud-clipboard", "main", "port", port)
    uci:set("cloud-clipboard", "main", "auth", auth)
    uci:set("cloud-clipboard", "main", "config", config)
    local committed = uci:commit("cloud-clipboard")

    if enabled == "1" then
        luci.sys.call("/etc/init.d/cloud-clipboard enable >/dev/null 2>&1; /etc/init.d/cloud-clipboard restart >/dev/null 2>&1")
    else
        luci.sys.call("/etc/init.d/cloud-clipboard stop >/dev/null 2>&1; /etc/init.d/cloud-clipboard disable >/dev/null 2>&1")
    end

    luci.http.prepare_content("application/json")
    luci.http.write_json({ ok = (committed == true), enabled = (enabled == "1"), host = host, port = port })
end

-- 日志读取函数
function get_log()
    local uci = luci.model.uci.cursor()
    local uselog = uci:get("cloud-clipboard", "main", "use_logread")
    local logtext = ""
    if uselog ~= "0" then
        logtext = luci.sys.exec("logread | grep 'cloud-clipboard'")
    else
        logtext = luci.sys.exec("cat /var/log/cloud-clipboard.log 2>/dev/null")
    end

    if logtext == "" then
        logtext = _("No related logs found")
    end

    luci.http.prepare_content("text/plain")
    luci.http.write(logtext)
end

-- 日志清除函数
function clear_log()
    luci.sys.call("> /var/log/cloud-clipboard.log")
    luci.http.prepare_content("application/json")
    luci.http.write('{"result":"success"}')
end