use tauri::{AppHandle, Manager, tray::{TrayIconBuilder, TrayIcon}};
use tauri::menu::{Menu, MenuItem, PredefinedMenuItem};
use log::info;

pub fn create_tray(app: &AppHandle) -> Result<TrayIcon, Box<dyn std::error::Error>> {
    info!("Creating tray icon...");

    let menu = Menu::with_items(app, & [
        &MenuItem::with_id(app, "edit_config", "编辑配置", true, None::<&str>)?,
        &PredefinedMenuItem::separator(app)?,
        &MenuItem::with_id(app, "quit_app", "退出", true, None::<&str>)?,
    ])?;

    let tray = TrayIconBuilder::new()
        .menu(&menu)
        .icon(app.default_window_icon().unwrap().clone())
        .on_menu_event(move |app, event| {
            let id = event.id().as_ref();
            info!("托盘菜单事件触发: {}", id);
            
            match id {
                "edit_config" => {
                    if let Some(window) = app.get_webview_window("config_window") {
                        let _ = window.show();
                        let _ = window.set_focus();
                    }
                }
                "quit_app" => {
                    app.exit(0);
                }
                _ => {}
            }
        })
        .on_tray_icon_event(|tray, event| {
            use tauri::tray::{TrayIconEvent, MouseButton};
            
            match event {
                TrayIconEvent::Click {
                    button: MouseButton::Left,
                    ..
                } => {
                    // 左键单击打开配置窗口
                    if let Some(window) = tray.app_handle().get_webview_window("config_window") {
                        let _ = window.show();
                        let _ = window.set_focus();
                    }
                }
                _ => {}
            }
        })
        .build(app)?;

    info!("托盘图标创建成功");
    Ok(tray)
}