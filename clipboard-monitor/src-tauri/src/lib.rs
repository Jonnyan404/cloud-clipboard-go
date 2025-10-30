pub mod clipboard;
pub mod config;
pub mod notifications;
pub mod tray;
pub mod uploader;

use std::sync::Arc;
use tokio::sync::Mutex;
use crate::config::Config;

#[derive(Clone)]
pub struct AppState {
    pub config: Arc<Mutex<Config>>,
}
