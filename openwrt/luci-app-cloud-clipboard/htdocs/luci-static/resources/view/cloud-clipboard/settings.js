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
        const [configData, serviceData] = data;
        const configSection = Object.keys(configData.sections)[0];
        const enabled = configData.sections[configSection].enabled === '1';
        const serviceInfo = serviceData['cloud-clipboard']?.instances?.instance || {};
        const isRunning = serviceInfo.running === true;

        const m = new form.Map('cloud-clipboard', _('Cloud Clipboard'), 
            _('云剪贴板工具，支持跨设备共享文本和文件'));

        // 自动提交配置变更
        m.on('apply', function() {
            return Promise.all([
                uci.commit(),
                uci.apply(['cloud-clipboard'])
            ]).catch(err => {
                ui.addNotification(null, E('p', _('配置应用失败: ') + err.message));
            });
        });

        // 基本设置部分
        const basicSection = m.section(form.TypedSection, 'cloud-clipboard', _('基本设置'));
        basicSection.anonymous = true;

        let o = basicSection.option(form.Flag, 'enabled', _('启用'));
        o.rmempty = false;
        o.default = '0';
        o.onchange = function(ev, section, value) {
            if(value === '1') return uci.set('cloud-clipboard', section, 'enabled', '1');
            return uci.unset('cloud-clipboard', section, 'enabled');
        };

        o = basicSection.option(form.Value, 'host', _('监听地址'));
        o.datatype = 'ip4addr';
        o.default = '0.0.0.0';
        o.rmempty = false;

        o = basicSection.option(form.Value, 'port', _('监听端口'));
        o.datatype = 'port';
        o.default = '9501';
        o.rmempty = false;

        o = basicSection.option(form.Value, 'auth', _('访问密码'));
        o.password = true;
        o.rmempty = true;

        // 服务状态显示
        const statusSection = m.section(form.TypedSection, 'cloud-clipboard', _('服务状态'));
        statusSection.anonymous = true;

        o = statusSection.option(form.DummyValue, '_status', _('当前状态'));
        o.cfgvalue = function() {
            return E('div', { 'class': 'service-status-container' }, [
                E('div', { 
                    'class': `status-indicator ${isRunning ? 'running' : 'stopped'}`,
                    'title': isRunning ? _('服务正在运行') : _('服务已停止')
                }, [
                    E('span', { 'class': 'status-dot' }),
                    E('span', isRunning ? _('运行中') : _('已停止'))
                ]),
                E('div', { 
                    'class': `config-state ${enabled ? 'enabled' : 'disabled'}`,
                    'title': enabled ? _('配置已启用') : _('配置已禁用')
                }, enabled ? _('已启用') : _('已禁用'))
            ]);
        };

        // 服务控制按钮
        o = statusSection.option(form.Button, '_control', _('服务操作'));
        o.inputtitle = function() {
            return enabled ? (isRunning ? _('重启服务') : _('启动服务')) : _('强制操作');
        };
        o.onclick = function(ev) {
            return Promise.resolve()
                .then(() => {
                    if (!enabled) {
                        return rpc.call('service', 'stop', { name: 'cloud-clipboard' });
                    }
                    const action = isRunning ? 'restart' : 'start';
                    return rpc.call('service', action, { name: 'cloud-clipboard' });
                })
                .then(res => {
                    if (res !== 0) throw new Error(res.stderr || _('操作失败'));
                    ui.showModal(_('操作成功'), [
                        E('p', _('服务状态已更新，页面即将刷新...')),
                        E('div', { 'class': 'progress', 'style': 'margin:15px 0' }, [
                            E('div', { 'class': 'progress-bar', 'style': 'width:100%' })
                        ])
                    ]);
                    setTimeout(() => window.location.reload(true), 2000);
                })
                .catch(err => {
                    ui.addNotification(null, E('p', [
                        E('strong', _('操作失败: ')),
                        err.message
                    ]));
                });
        };

        // 访问入口
        const accessSection = m.section(form.TypedSection, 'cloud-clipboard', _('访问入口'));
        accessSection.anonymous = true;

        o = accessSection.option(form.DummyValue, '_access', _('控制面板'));
        o.cfgvalue = function() {
            const host = uci.get('cloud-clipboard', configSection, 'host') || window.location.hostname;
            const port = uci.get('cloud-clipboard', configSection, 'port') || '9501';
            const auth = uci.get('cloud-clipboard', configSection, 'auth') || '';
            
            const url = `http://${host}:${port}${auth ? `#auth=${auth}` : ''}`;
            
            return E('a', {
                'href': url,
                'target': '_blank',
                'class': 'cbi-button cbi-button-action access-link',
                'click': ui.createHandlerFn(this, function() {
                    if (auth) {
                        localStorage.setItem('clipboard-auth-token', auth);
                    }
                })
            }, [
                E('span', { 'class': 'icon icon-external-link' }),
                E('span', _('打开控制台'))
            ]);
        };

        return m.render();
    }
});
