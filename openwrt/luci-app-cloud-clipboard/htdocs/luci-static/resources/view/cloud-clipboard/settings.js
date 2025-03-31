'use strict';
'require view';
'require form';
'require uci';
'require fs';
'require ui';

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

        // 添加状态节和控制按钮
        s = m.section(form.NamedSection, '_status', 'status', _('服务状态'));
        s.anonymous = true;

        o = s.option(form.DummyValue, 'status', _('运行状态'));
        o.render = function() {
            return fs.exec('/etc/init.d/cloud-clipboard', ['status'])
                .then(function(res) {
                    var running = res.stdout.indexOf('running') > -1;
                    return E('span', { 'class': running ? 'label success' : 'label danger' },
                           [ _(running ? '运行中' : '未运行') ]);
                })
                .catch(function() {
                    return E('span', { 'class': 'label danger' }, [ _('状态未知') ]);
                });
        };

        // 添加服务控制按钮
        o = s.option(form.Button, 'restart', _('服务控制'));
        o.inputtitle = _('重启服务');
        o.inputstyle = 'apply';
        o.onclick = function() {
            return fs.exec('/bin/sh', ['-c', '/etc/init.d/cloud-clipboard restart'])
                .then(function() {
                    ui.addNotification(null, E('p', _('已重启Cloud Clipboard服务')));
                    window.setTimeout(function() {
                        location.reload();
                    }, 1000);
                })
                .catch(function(error) {
                    ui.addNotification(null, E('p', _('重启服务失败: ') + error));
                });
        };
        
        // 添加停止服务按钮
        o = s.option(form.Button, 'stop', _('停止服务'));
        o.inputtitle = _('停止服务');
        o.inputstyle = 'reset';
        o.onclick = function() {
            return fs.exec('/bin/sh', ['-c', '/etc/init.d/cloud-clipboard stop'])
                .then(function() {
                    ui.addNotification(null, E('p', _('已停止Cloud Clipboard服务')));
                    window.setTimeout(function() {
                        location.reload();
                    }, 1000);
                })
                .catch(function(error) {
                    ui.addNotification(null, E('p', _('停止服务失败: ') + error));
                });
        };

        return m.render();
    }
});