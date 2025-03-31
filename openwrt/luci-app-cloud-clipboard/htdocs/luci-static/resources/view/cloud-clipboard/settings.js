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

        // 修改服务状态显示部分
        s = m.section(form.TypedSection, 'cloud-clipboard', _('服务状态'));
        s.anonymous = true;

        // 添加状态显示（仅显示UCI中的启用状态）
        o = s.option(form.DummyValue, '_status', _('运行状态'));
        o.rawhtml = true;
        o.cfgvalue = function() {
            var enabled = (uci.get('cloud-clipboard', 'main', 'enabled') == '1');
            
            return E('div', [
                E('span', { 'class': enabled ? 'label notice' : 'label warning' },
                  [ _(enabled ? '已启用' : '已禁用') ])
            ]);
        };

        // 添加服务控制链接
        o = s.option(form.DummyValue, '_buttons', _('服务控制'));
        o.rawhtml = true;
        o.cfgvalue = function() {
            return E('div', { 'class': 'cbi-value-field' }, [
                E('a', {
                    'class': 'btn cbi-button cbi-button-apply',
                    'href': '/cgi-bin/luci/admin/system/startup/restart/cloud-clipboard',
                    'target': '_blank'
                }, [ _('重启服务') ]),
                ' ',
                E('a', {
                    'class': 'btn cbi-button cbi-button-reset',
                    'href': '/cgi-bin/luci/admin/system/startup/stop/cloud-clipboard',
                    'target': '_blank'
                }, [ _('停止服务') ])
            ]);
        };

        return m.render();
    }
});