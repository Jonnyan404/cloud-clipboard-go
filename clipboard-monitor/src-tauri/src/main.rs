#![cfg_attr(all(not(debug_assertions), target_os = "windows"), windows_subsystem = "windows")]

mod clipboard;
mod config;
mod tray;
mod commands;
mod uploader;
mod notifications;

use tauri::{Builder, Manager};
use log::info;

fn main() {
    env_logger::Builder::from_default_env()
        .filter_level(log::LevelFilter::Info)
        .init();

    info!("应用启动...");

    let _config = config::load_config();

    Builder::default()
        .invoke_handler(tauri::generate_handler![
            commands::get_config,
            commands::save_config
        ])
        .setup(|app| {
            info!("Setup 开始");
            
            let tray = tray::create_tray(&app.handle())?;
            app.manage(tray);
            info!("托盘图标已初始化");

            let app_handle = app.handle().clone();
            clipboard::start_monitoring(app_handle);
            info!("剪贴板监控已启动");

            info!("Setup 完成");
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}