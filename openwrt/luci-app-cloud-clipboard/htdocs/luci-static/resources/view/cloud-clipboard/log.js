'use strict';
'require view';
'require fs';
'require ui';

return view.extend({
    render: function() {
        var logfile = '/var/log/cloud-clipboard.log';
        
        return E('div', { 'class': 'cbi-map' }, [
            E('h2', {}, [ _('Cloud Clipboard - 日志') ]),
            
            E('div', { 'class': 'cbi-section' }, [
                E('div', { 'class': 'cbi-section-descr' }, _('显示Cloud Clipboard服务的日志文件内容')),
                
                E('div', { 'class': 'cbi-section-node' }, [
                    E('div', { 'class': 'cbi-value' }, [
                        E('div', { 'class': 'cbi-value-title' }, _('日志文件')),
                        E('div', { 'class': 'cbi-value-field' }, [
                            E('button', {
                                'class': 'btn cbi-button cbi-button-apply',
                                'click': ui.createHandlerFn(this, function() {
                                    location.reload();
                                })
                            }, [ _('刷新') ]),
                            ' ',
                            E('button', {
                                'class': 'btn cbi-button cbi-button-remove',
                                'click': ui.createHandlerFn(this, function() {
                                    return fs.write(logfile, '').then(function() {
                                        ui.addNotification(null, E('p', _('日志已清空')));
                                        location.reload();
                                    });
                                })
                            }, [ _('清空') ])
                        ])
                    ])
                ]),
                
                E('pre', { 'id': 'syslog' })
            ])
        ]);
    },
    
    handleSaveApply: null,
    handleSave: null,
    handleReset: null,
    
    load: function() {
        return fs.read('/var/log/cloud-clipboard.log').catch(function(err) {
            if (err.name === 'NotFoundError')
                return '';
            throw err;
        });
    },
    
    render_content: function(logdata) {
        var syslog = document.getElementById('syslog');
        if (syslog) {
            syslog.textContent = logdata || _('日志为空');
        }
    }
});