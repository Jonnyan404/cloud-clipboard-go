use serde::{Deserialize, Serialize};
use std::fs;
use std::path::PathBuf;
use log::warn;

#[derive(Serialize, Deserialize, Clone)]
pub struct Config {
    pub text_urls: Vec<String>,
    pub file_urls: Vec<String>,
    pub enable_text: bool,
    pub enable_file: bool,
    pub enable_monitoring: bool,
    pub max_file_size_mb: u64,
    pub log_level: String,
    pub auth_token: Option<String>,
}

impl Default for Config {
    fn default() -> Self {
        Self {
            text_urls: vec!["http://localhost:9501/text".to_string()],
            file_urls: vec!["http://localhost:9501/upload".to_string()],
            enable_text: true,
            enable_file: true,
            enable_monitoring: true,
            max_file_size_mb: 50,
            log_level: "info".to_string(),
            auth_token: None,
        }
    }
}

pub fn load_config() -> Config {
    let path = get_config_path();
    if path.exists() {
        let data = fs::read_to_string(&path).unwrap_or_default();
        serde_json::from_str(&data).unwrap_or_default()
    } else {
        let config = Config::default();
        if let Err(e) = save_config(&config) {
            warn!("Failed to save default config: {}", e);
        }
        config
    }
}

pub fn save_config(config: &Config) -> Result<(), Box<dyn std::error::Error>> {
    let config_path = get_config_path();
    let json = serde_json::to_string_pretty(config)?;
    std::fs::write(config_path, json)?;
    Ok(())
}

fn get_config_path() -> PathBuf {
    // 从项目目录加载（假设 src-tauri 目录）
    std::env::current_dir().unwrap().join("config.json")  // 修改为项目根目录
}