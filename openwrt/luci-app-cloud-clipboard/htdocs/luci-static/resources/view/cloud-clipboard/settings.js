'use strict';
'require view';
'require form';
'require uci';
'require fs';
'require ui';
'require sys';  // 添加系统调用模块

return view.extend({
    load: function() {
        return Promise.all([uci.load('cloud-clipboard')]);
    },
    
    render: function() {
        var m = new form.Map('cloud-clipboard', _('Cloud Clipboard'), _('Cloud Clipboard是一个文本和文件传输工具，可在多个设备之间共享剪贴板内容。'));

        var s = m.section(form.TypedSection, 'cloud-clipboard', _('基本设置'));
        s.anonymous = true;

        var o = s.option(form.Flag, 'enabled', _('启用'));
        o.rmempty = false;

        o = s.option(form.Value, 'host', _('监听地址'));
        o.datatype = 'ip4addr';
        o.default = '0.0.0.0';
        o.rmempty = false;

        o = s.option(form.Value, 'port', _('监听端口'));
        o.datatype = 'port';
        o.default = '9501';
        o.rmempty = false;

        o = s.option(form.Value, 'auth', _('访问密码'));
        o.password = true;
        o.rmempty = true;
        o.description = _('如果设置，访问时需要输入此密码。留空表示不需要密码。');

        // 修改服务状态显示部分，使用 TypedSection 代替 NamedSection
        s = m.section(form.TypedSection, 'cloud-clipboard', _('服务状态'));
        s.anonymous = true;

        // 修改运行状态检测，使用 sys.call 代替 fs.exec
        o = s.option(form.DummyValue, '_status', _('运行状态'));
        o.rawhtml = true;  // 确保HTML内容能正确渲染
        o.cfgvalue = function() {
            // 使用 sys.call 检查进程是否运行
            var running = sys.call("pgrep -f 'cloud-clipboard' >/dev/null") === 0;
            
            // 使用 sys.call 检查服务是否启用
            var enabled = sys.call("/etc/init.d/cloud-clipboard enabled") === 0;
            
            return E('div', [
                E('span', { 'class': running ? 'label success' : 'label danger' },
                  [ _(running ? '运行中' : '未运行') ]),
                ' ',
                E('span', { 'class': enabled ? 'label notice' : 'label warning' },
                  [ _(enabled ? '已启用' : '已禁用') ])
            ]);
        };

        // 添加服务控制按钮组，使用 sys.call 代替 fs.exec
        o = s.option(form.DummyValue, '_buttons', _('服务控制'));
        o.rawhtml = true;
        o.cfgvalue = function() {
            return E('div', { 'class': 'cbi-value-field' }, [
                E('button', {
                    'class': 'btn cbi-button cbi-button-apply',
                    'click': ui.createHandlerFn(this, function() {
                        sys.call("/etc/init.d/cloud-clipboard restart");
                        ui.addNotification(null, E('p', _('已重启服务')));
                        window.setTimeout(function() { location.reload() }, 1000);
                    })
                }, [ _('重启') ]),
                ' ',
                E('button', {
                    'class': 'btn cbi-button cbi-button-remove',
                    'click': ui.createHandlerFn(this, function() {
                        sys.call("/etc/init.d/cloud-clipboard stop");
                        ui.addNotification(null, E('p', _('已停止服务')));
                        window.setTimeout(function() { location.reload() }, 1000);
                    })
                }, [ _('停止') ])
            ]);
        };
        
        // 添加服务快速访问链接
        s = m.section(form.TypedSection, 'cloud-clipboard', _('服务访问'));
        s.anonymous = true;
        
        o = s.option(form.DummyValue, '_access', _('Web界面'));
        o.rawhtml = true;
        o.cfgvalue = function() {
            // 获取当前主机地址和端口
            var host = uci.get('cloud-clipboard', 'main', 'host') || '0.0.0.0';
            var port = uci.get('cloud-clipboard', 'main', 'port') || '9501';
            
            // 如果监听地址是0.0.0.0，使用当前访问的地址
            if (host === '0.0.0.0') {
                host = window.location.hostname;
            }
            
            return E('div', { 'class': 'cbi-value-field' }, [
                E('a', {
                    'href': 'http://' + host + ':' + port,
                    'target': '_blank',
                    'class': 'btn cbi-button cbi-button-neutral'
                }, [ _('打开 Cloud Clipboard') ])
            ]);
        };

        return m.render();
    }
});