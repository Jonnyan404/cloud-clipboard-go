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
        let m, s, o;
        
        m = new form.Map('cloud-clipboard', _('Cloud Clipboard'), 
            _('Cloud Clipboard是一个文本和文件传输工具，可在多个设备之间共享剪贴板内容。'));

        // 基本设置部分
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

        // 服务状态部分
        s = m.section(form.TypedSection, 'cloud-clipboard', _('服务状态'));
        s.anonymous = true;
        
        o = s.option(form.DummyValue, '_status', _('状态'));
        o.cfgvalue = function() {
            return fs.exec('/bin/busybox', ['ps']).then(function(res) {
                var running = (res.stdout || '').indexOf('cloud-clipboard') > -1;
                return fs.exec('/etc/init.d/cloud-clipboard', ['enabled']).then(function(res) {
                    var enabled = res.code === 0;
                    
                    return E('div', [
                        E('span', { 'class': running ? 'label success' : 'label danger' }, 
                            [ _(running ? '运行中' : '未运行') ]),
                        ' ',
                        E('span', { 'class': enabled ? 'label notice' : 'label warning' },
                            [ _(enabled ? '已启用' : '已禁用') ])
                    ]);
                });
            }).catch(function(err) {
                ui.addNotification(null, E('p', _('获取服务状态失败: ') + err.message));
                return E('em', _('获取状态失败'));
            });
        };

        // 服务控制
        o = s.option(form.Button, '_restart', _('重启服务'));
        o.inputtitle = _('重启');
        o.inputstyle = 'apply';
        o.onclick = function() {
            return fs.exec('/etc/init.d/cloud-clipboard', ['restart']).then(function(res) {
                if (res.code === 0) {
                    ui.addNotification(null, E('p', _('服务已重启')));
                } else {
                    ui.addNotification(null, E('p', _('重启失败: ') + res.stderr));
                }
                
                window.setTimeout(function() {
                    location.reload();
                }, 1000);
            });
        };
        
        o = s.option(form.Button, '_stop', _('停止服务'));
        o.inputtitle = _('停止');
        o.inputstyle = 'reset';
        o.onclick = function() {
            return fs.exec('/etc/init.d/cloud-clipboard', ['stop']).then(function(res) {
                if (res.code === 0) {
                    ui.addNotification(null, E('p', _('服务已停止')));
                } else {
                    ui.addNotification(null, E('p', _('停止失败: ') + res.stderr));
                }
                
                window.setTimeout(function() {
                    location.reload();
                }, 1000);
            });
        };

        // 服务访问链接
        s = m.section(form.TypedSection, 'cloud-clipboard', _('服务访问'));
        s.anonymous = true;
        
        o = s.option(form.DummyValue, '_access', _('Web界面'));
        o.cfgvalue = function() {
            var host = uci.get('cloud-clipboard', 'main', 'host') || '0.0.0.0';
            var port = uci.get('cloud-clipboard', 'main', 'port') || '9501';
            
            // 如果监听地址是0.0.0.0，使用当前访问的地址
            if (host === '0.0.0.0') {
                host = window.location.hostname;
            }
            
            return E('a', {
                'href': 'http://' + host + ':' + port,
                'target': '_blank',
                'class': 'btn cbi-button cbi-button-neutral'
            }, [ _('打开 Cloud Clipboard') ]);
        };

        return m.render();
    }
});