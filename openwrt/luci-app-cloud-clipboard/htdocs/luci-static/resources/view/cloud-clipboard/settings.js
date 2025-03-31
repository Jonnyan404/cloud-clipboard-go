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

        // 修改服务状态显示部分，使用 TypedSection 代替 NamedSection
        s = m.section(form.TypedSection, 'cloud-clipboard', _('服务状态'));
        s.anonymous = true;

        // 修改运行状态检测
        o = s.option(form.DummyValue, '_status', _('运行状态'));
        o.rawhtml = true;  // 确保HTML内容能正确渲染
        o.cfgvalue = function() {
            return fs.exec('/bin/sh', ['-c', 'pgrep -f cloud-clipboard || echo "not running"'])
                .then(function(res) {
                    var running = res.stdout.trim() !== 'not running';
                    
                    return fs.exec('/etc/init.d/cloud-clipboard', ['enabled'])
                        .then(function(res2) {
                            var enabled = res2.code === 0;
                            
                            return E('div', [
                                E('span', { 'class': running ? 'label success' : 'label danger' }, 
                                  [ _(running ? '运行中' : '未运行') ]),
                                ' ',
                                E('span', { 'class': enabled ? 'label notice' : 'label warning' },
                                  [ _(enabled ? '已启用' : '已禁用') ])
                            ]);
                        });
                })
                .catch(function(err) {
                    console.error('Status check error:', err);
                    return E('span', { 'class': 'label danger' }, [ _('获取状态失败') ]);
                });
        };

        // 添加服务控制按钮组
        o = s.option(form.DummyValue, '_buttons', _('服务控制'));
        o.rawhtml = true;
        o.cfgvalue = function() {
            return E('div', { 'class': 'cbi-value-field' }, [
                E('button', {
                    'class': 'btn cbi-button cbi-button-apply',
                    'click': ui.createHandlerFn(this, function() {
                        return fs.exec('/etc/init.d/cloud-clipboard', ['restart']).then(function() {
                            ui.addNotification(null, E('p', _('已重启服务')));
                            window.setTimeout(function() { location.reload() }, 1000);
                        });
                    })
                }, [ _('重启') ]),
                ' ',
                E('button', {
                    'class': 'btn cbi-button cbi-button-remove',
                    'click': ui.createHandlerFn(this, function() {
                        return fs.exec('/etc/init.d/cloud-clipboard', ['stop']).then(function() {
                            ui.addNotification(null, E('p', _('已停止服务')));
                            window.setTimeout(function() { location.reload() }, 1000);
                        });
                    })
                }, [ _('停止') ])
            ]);
        };

        return m.render();
    }
});