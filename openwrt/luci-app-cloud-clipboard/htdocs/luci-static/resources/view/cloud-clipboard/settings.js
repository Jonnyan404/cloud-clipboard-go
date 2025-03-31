'use strict';
'require view';
'require form';
'require uci';
'require fs';
'require ui';
'require tools.widgets as widgets';
'require rpc';

return view.extend({
    render: function() {
        var m, s, o;

        m = new form.Map('cloud-clipboard', _('Cloud Clipboard'), _('Cloud Clipboard是一个文本和文件传输工具，可在多个设备之间共享剪贴板内容。'));

        s = m.section(form.TypedSection, 'cloud-clipboard', _('基本设置'));
        s.anonymous = true;

        o = s.option(form.Flag, 'enabled', _('启用'));
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
        
        // 添加状态节
        s = m.section(form.TypedSection, 'cloud-clipboard', _('服务状态'));
        s.anonymous = true;
        
        o = s.option(form.DummyValue, '_status', _('运行状态'));
        o.rawhtml = true;
        o.load = function() {
            return Promise.resolve();
        };
        o.render = function() {
            return rpc.delegated('luci.cloud-clipboard', 'status').then(function(result) {
                if (result && result.result === "running")
                    return E('span', { 'class': 'label success' }, [ _('运行中') ]);
                else
                    return E('span', { 'class': 'label danger' }, [ _('未运行') ]);
            }).catch(function() {
                return E('span', { 'class': 'label danger' }, [ _('状态未知') ]);
            });
        };
        
        o = s.option(form.Button, '_restart', _('重启服务'));
        o.inputtitle = _('重启服务');
        o.inputstyle = 'apply';
        o.onclick = function() {
            return fs.exec('/etc/init.d/cloud-clipboard', ['restart']).then(function() {
                ui.addNotification(null, E('p', _('已重启Cloud Clipboard服务')));
                window.setTimeout(function() {
                    location.reload();
                }, 1000);
            });
        };

        return m.render();
    }
});