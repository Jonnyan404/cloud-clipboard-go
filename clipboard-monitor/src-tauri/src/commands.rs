use tauri::command;
use crate::config::{self, Config};
use log::info;

#[command]
pub async fn get_config() -> Result<Config, String> {
    Ok(config::load_config())
}

#[command]
pub async fn save_config(config: Config) -> Result<(), String> {
    info!("正在保存新配置");
    config::save_config(&config).map_err(|e| e.to_string())
}