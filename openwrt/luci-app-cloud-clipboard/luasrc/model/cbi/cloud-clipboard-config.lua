local m, s, o

m = Map("cloud-clipboard", translate("Cloud Clipboard") .. " - " .. translate("配置文件"),
    translate("编辑 Cloud Clipboard 的高级配置文件"))

-- 配置文件编辑部分
s = m:section(TypedSection, "cloud-clipboard", translate("配置文件编辑"))
s.anonymous = true

-- 创建临时变量存储当前配置路径
local config_path = luci.model.uci.cursor():get("cloud-clipboard", "main", "config") or ""

-- 如果配置文件为空，不显示编辑器
if config_path == "" then
    local no_path = s:option(DummyValue, "_no_path", "")
    no_path.rawhtml = true
    no_path.value = '<span class="alert-message warning">' .. 
                    translate("请先在基本设置中设置配置文件路径，然后保存并应用。") .. 
                    '</span><br/><a href="' .. 
                    luci.dispatcher.build_url("admin", "services", "cloud-clipboard", "settings") .. 
                    '" class="cbi-button">' .. translate("返回基本设置") .. '</a>'
else
    -- 检查目录可写性
    local config_dir = config_path:match("(.+)/[^/]+")
    local writable = true
    local file_exists = nixio.fs.access(config_path)
    
    if config_dir and not nixio.fs.access(config_dir) then
        -- 尝试创建目录
        nixio.fs.mkdir(config_dir)
        if not nixio.fs.access(config_dir) then
            writable = false
        end
    end
    
    if not writable then
        local not_writable = s:option(DummyValue, "_not_writable", "")
        not_writable.rawhtml = true
        not_writable.value = '<span class="alert-message warning">' .. 
                            translate("无法写入配置目录，请检查权限或选择其他路径。") .. 
                            '</span>'
    else
        -- 显示当前配置路径
        local path_info = s:option(DummyValue, "_path_info", translate("当前配置文件"))
        path_info.rawhtml = true
        path_info.value = '<code>' .. config_path .. '</code>' .. 
                          (file_exists and ' <span class="alert-message success">' .. 
                           translate("(文件已存在)") .. '</span>' or 
                           ' <span class="alert-message notice">' .. 
                           translate("(文件不存在，将被创建)") .. '</span>')
        
        -- 添加JSON内容编辑器
        local json_edit = s:option(TextValue, "_json_content", translate("编辑配置文件"))
        json_edit.wrap = "off"
        json_edit.rows = 20
        json_edit.template = "cloud-clipboard/json_editor"
        
        -- 获取当前文件内容
        function json_edit.cfgvalue(self, section)
            if nixio.fs.access(config_path) then
                return nixio.fs.readfile(config_path) or "{}"
            else
                -- 如果文件不存在，返回默认的JSON格式
                local auth = luci.model.uci.cursor():get("cloud-clipboard", "main", "auth") or ""
                local default_config = {
                    server = {
                        host = "",
                        port = 9501,
                        prefix = "",
                        history = 100,
                        historyFile = "",
                        storageDir = "",
                        auth = auth ~= "",
                        roomList = false,
                        roomCleanup = 3600
                    },
                    text = {
                        limit = 4096
                    },
                    file = {
                        expire = 3600,
                        chunk = 2097152,
                        limit = 268435456
                    }
                }
                -- 如果设置了密码，添加到配置中
                if auth ~= "" then
                    default_config.server.auth = true
                end
                
                -- 转换为JSON字符串
                local json = require "luci.jsonc" or require "luci.json"
                if json.stringify then
                    return json.stringify(default_config, true)
                else
                    -- 如果没有内置的JSON序列化器，返回固定格式
                    return [[{
        "server": {
                "host": "",
                "port": 9501,
                "prefix": "",
                "history": 100,
                "historyFile": "",
                "storageDir": "",
                "auth": ]] .. (auth ~= "" and "true" or "false") .. [[,
                "roomList": false,
                "roomCleanup": 3600
        },
        "text": {
                "limit": 4096
        },
        "file": {
                "expire": 3600,
                "chunk": 2097152,
                "limit": 268435456
        }
}]]
                end
            end
        end
        
        -- 保存修改的内容
        function json_edit.write(self, section, value)
            if value then
                -- 验证JSON格式
                local valid_json = false
                local json = require "luci.jsonc" or require "luci.json"
                local parsed
                
                if json.parse then
                    parsed = json.parse(value)
                    valid_json = (parsed ~= nil)
                else
                    -- 如果没有内置的JSON解析器，尝试使用基本的验证
                    -- 检查基本的JSON语法（这是一个简单的检查，可能不完全准确）
                    valid_json = value:match("^%s*{.+}%s*$")
                end
                
                if not valid_json then
                    return nil, "无效的JSON格式"
                end
                
                -- 保存到配置文件
                nixio.fs.writefile(config_path, value)
                
                -- 如果在配置中启用了auth但UCI中没有设置密码，显示警告
                local auth = luci.model.uci.cursor():get("cloud-clipboard", "main", "auth")
                if (parsed and parsed.server and parsed.server.auth == true) and (not auth or #auth == 0) then
                    return nil, "警告：配置文件中启用了认证，但在基本设置中未设置密码，这可能导致访问问题"
                end
            end
            return value
        end
    end
end

-- 添加JSON内容提示
local json_help = s:option(DummyValue, "_json_help", "")
json_help.rawhtml = true
json_help.value = [[
<div class="alert-message info">
    <p><strong>]] .. translate("JSON配置文件说明") .. [[</strong></p>
    <p>]] .. translate("配置文件应为有效的JSON格式，以下是可用的配置项：") .. [[</p>
    <pre>
{
    "server": {
        "host": "",                // 服务器绑定地址，留空使用UCI设置的host
        "port": 9501,              // 服务器端口，留空使用UCI设置的port
        "prefix": "",              // Web界面URL前缀，例如"/clipboard"
        "history": 100,            // 历史记录数量
        "historyFile": "",         // 历史记录文件路径
        "storageDir": "",          // 文件存储目录
        "auth": false,              // 是否启用认证，true时需在UCI设置中配置密码
        "roomList": false,          // 是否启用房间列表功能
        "roomCleanup": 3600        // 房间清理周期(秒)，清理消息数为0的房间
    },
    "text": {
        "limit": 4096              // 文本大小限制(字节)
    },
    "file": {
        "expire": 3600,            // 文件过期时间(秒)
        "chunk": 2097152,          // 文件上传分片大小(字节)，2MB
        "limit": 268435456         // 文件大小限制(字节)，256MB
    }
}
</pre>
</div>
]]

return m