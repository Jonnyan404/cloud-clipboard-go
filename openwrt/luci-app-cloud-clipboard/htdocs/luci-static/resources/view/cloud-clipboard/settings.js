'use strict';
'require view';
'require form';
'require uci';
'require fs';
'require ui';
'require rpc';

var serviceRPC = rpc.declare({
    object: 'service',
    method: 'list',
    params: ['name'],
    expect: { '': {} }
});

return view.extend({
    load: function() {
        return Promise.all([
            uci.load('cloud-clipboard'),
            serviceRPC('cloud-clipboard')
        ]);
    },
    
    render: function(data) {
        let m, s, o;
        
        m = new form.Map('cloud-clipboard', _('Cloud Clipboard'), 
            _('云剪贴板工具，支持跨设备共享文本和文件'));

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

        // 服务状态部分
        s = m.section(form.TypedSection, 'cloud-clipboard', _('服务状态'));
        s.anonymous = true;
        
        o = s.option(form.DummyValue, '_status', _('状态'));
        o.cfgvalue = function() {
            return Promise.resolve(data[1]).then(function(svc) {
                const running = svc['cloud-clipboard']?.instances?.instance?.running || false;
                const enabled = uci.get('cloud-clipboard', 'main', 'enabled') === '1';
                
                return E('div', { 'class': 'cbi-value-field' }, [
                    E('span', { 
                        'class': running ? 'label-success' : 'label-danger',
                        'style': 'padding: 2px 5px; border-radius: 3px;' 
                    }, [ running ? _('运行中') : _('已停止') ]),
                    E('span', { 
                        'class': enabled ? 'label-success' : 'label-warning',
                        'style': 'margin-left: 10px; padding: 2px 5px; border-radius: 3px;' 
                    }, [ enabled ? _('已启用') : _('已禁用') ])
                ]);
            }).catch(function(err) {
                ui.addNotification(null, E('p', _('状态获取失败: ') + err.message));
                return E('em', _('状态未知'));
            });
        };

        // 服务控制按钮（修复重点）
        o = s.option(form.Button, '_control', _('服务操作'));
        o.inputtitle = _('重启服务');
        o.inputstyle = 'apply';
        o.onclick = function() {
            return rpc.call('service', 'list', { name: 'cloud-clipboard' })
                .then(function(res) {
                    const running = res['cloud-clipboard']?.instances?.instance?.running;
                    const action = running ? 'restart' : 'start';
                    
                    // 修复点：移除多余的单引号
                    return rpc.call('service', action, { name: 'cloud-clipboard' });
                })
                .then(function(res) {
                    if (res.code !== 0) throw new Error(res.stderr || '操作失败');
                    ui.addNotification(null, E('p', _('操作成功，1秒后刷新页面')));
                    setTimeout(window.location.reload.bind(window.location), 1000);
                })
                .catch(function(err) {
                    ui.addNotification(null, E('p', _('操作失败: ') + err.message));
                });
        };

        // 访问链接
        s = m.section(form.TypedSection, 'cloud-clipboard', _('访问入口'));
        s.anonymous = true;
        
        o = s.option(form.DummyValue, '_access', _('Web界面'));
        o.cfgvalue = function() {
            const host = uci.get('cloud-clipboard', 'main', 'host') || window.location.hostname;
            const port = uci.get('cloud-clipboard', 'main', 'port') || '9501';
            
            return E('a', {
                'href': `http://${host}:${port}`,
                'target': '_blank',
                'class': 'cbi-button cbi-button-action',
                'style': 'text-decoration: none; padding: 5px 15px;'
            }, [ _('打开控制面板') ]);
        };

        return m.render();
    }
});
