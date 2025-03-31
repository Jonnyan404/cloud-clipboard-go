'use strict';
'require view';
'require form';
'require uci';
'require fs';
'require ui';
'require rpc';

return view.extend({
    render: function() {
        var m, s, o;
        m = new form.Map('cloud-clipboard', _('Cloud Clipboard'), _('Cloud Clipboard是一个文本和文件传输工具，可在多个设备之间共享剪贴板内容。'));

        // 基本设置
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

        // 服务状态与重启服务放在一个单独的 section，下方状态显示在重启按钮正上方
        s = m.section(form.TypedSection, 'cloud-clipboard-status', _('服务状态'));
        s.anonymous = true;

        // 运行状态显示
        o = s.option(form.DummyValue, '_status', _('运行状态'));
        o.rawhtml = true;
        o.load = function() {
            return Promise.resolve();
        };
        o.render = function() {
            return rpc.call('luci.cloud-clipboard', 'status', []).then(function(result) {
                if (result && result.result === "running")
                    return E('span', { 'class': 'label success' }, [ _('运行中') ]);
                else
                    return E('span', { 'class': 'label danger' }, [ _('未运行') ]);
            }).catch(function() {
                return E('span', { 'class': 'label danger' }, [ _('状态未知') ]);
            });
        };

        // 重启服务按钮
        o = s.option(form.Button, '_restart', _('重启服务'));
        o.inputtitle = _('重启服务');
        o.inputstyle = 'apply';
        o.onclick = function() {
            return fs.exec('/etc/init.d/cloud-clipboard', ['restart']).then(function() {
                ui.addNotification(null, E('p', _('已重启 Cloud Clipboard 服务')));
                window.setTimeout(function() {
                    location.reload();
                }, 1000);
            });
        };

        return m.render();
    }
});