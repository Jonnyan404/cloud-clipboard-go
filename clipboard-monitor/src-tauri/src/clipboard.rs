use std::path::Path;
use std::sync::{Arc, Mutex};
use regex::Regex;
use tauri::AppHandle;
use log::{info, error, debug};
use std::thread;

use clipboard_rs::{
    common::RustImage, Clipboard, ClipboardContext, ClipboardHandler, ClipboardWatcher,
    ClipboardWatcherContext, ContentFormat,
};

use crate::uploader::upload_content;
use crate::notifications::show_notification;

/// 使用一个简单的哈希值来防止在短时间内重复处理相同的内容。
#[derive(Clone, Default)]
struct DebounceState {
    last_text_hash: u64,
    last_image_hash: u64,
    last_files_hash: u64,
}

/// 简单的 DJB2 哈希函数
fn hash_bytes(data: &[u8]) -> u64 {
    let mut hash: u64 = 5381;
    for byte in data {
        hash = ((hash << 5).wrapping_add(hash)).wrapping_add(*byte as u64);
    }
    hash
}

fn hash_string_vec(vec: &[String]) -> u64 {
    let combined = vec.join("");
    hash_bytes(combined.as_bytes())
}

/// Determines the subtype of text content (e.g., URL, email, color).
fn get_text_subtype(value: &str) -> Option<String> {
    if let Ok(url_re) = Regex::new(r"https?://[^\s/$.?#].[^\s]*") {
        if url_re.is_match(value) { return Some("url".to_string()); }
    }
    if let Ok(email_re) = Regex::new(r"(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b") {
        if email_re.is_match(value) { return Some("email".to_string()); }
    }
    if let Ok(color_re) = Regex::new(r"^#([A-Fa-f0-9]{6}|[A-Fa-f0-9]{3})$") {
        if color_re.is_match(value) { return Some("color".to_string()); }
    }
    None
}

/// 实现了 ClipboardHandler trait 的结构体
struct ClipboardHandlerImpl {
    app_handle: AppHandle,
    debounce_state: Arc<Mutex<DebounceState>>,
}

impl ClipboardHandler for ClipboardHandlerImpl {
    /// 当剪贴板内容发生变化时，此方法被调用
    fn on_clipboard_change(&mut self) {
        debug!("Clipboard change detected by watcher.");
        
        let _app = self.app_handle.clone();  // <-- 改为 _app 或者直接移除这一行
        let state = self.debounce_state.clone();

        tauri::async_runtime::spawn(async move {
            // 在异步块内部创建新的 ClipboardContext
            let Ok(ctx) = ClipboardContext::new() else {
                error!("Failed to create clipboard context in async task.");
                return;
            };

            // 获取 config
            let config = crate::config::load_config();

            if !config.enable_monitoring {
                return;
            }

            // 设定优先级: 文件 > 图片 > 文本
            if ctx.has(ContentFormat::Files) {
                if let Ok(files) = ctx.get_files() {
                    let should_process = {
                        if files.is_empty() { return; }
                        let files_hash = hash_string_vec(&files);
                        let mut state_lock = state.lock().unwrap();
                        if files_hash == state_lock.last_files_hash {
                            false
                        } else {
                            state_lock.last_files_hash = files_hash;
                            true
                        }
                    };

                    if should_process {
                        info!("Processing copied files: {:?}", files);
                        for file_path_str in &files {
                            let clean_path = file_path_str.strip_prefix("file://").unwrap_or(file_path_str);
                            match std::fs::read(clean_path) {
                                Ok(data) => {
                                    let p = Path::new(clean_path);
                                    let filename = p.file_name().unwrap_or_default().to_str().unwrap_or("file");
                                    upload_and_notify(&config, "file", None, &data, filename).await;
                                }
                                Err(e) => error!("Failed to read file {}: {}", clean_path, e),
                            }
                        }
                    }
                }
            } else if ctx.has(ContentFormat::Image) {
                if let Ok(image) = ctx.get_image() {
                    let image_bytes = image.to_png().map_or_else(|_| Vec::new(), |img| img.get_bytes().to_vec());
                    let should_process = {
                        if image_bytes.is_empty() { return; }
                        let image_hash = hash_bytes(&image_bytes);
                        let mut state_lock = state.lock().unwrap();
                        if image_hash == state_lock.last_image_hash {
                            false
                        } else {
                            state_lock.last_image_hash = image_hash;
                            state_lock.last_text_hash = 0;
                            true
                        }
                    };

                    if should_process {
                        info!("Processing copied image, size: {} bytes", image_bytes.len());
                        upload_and_notify(&config, "image", None, &image_bytes, "Image").await;
                    }
                }
            } else if ctx.has(ContentFormat::Text) {
                if let Ok(text) = ctx.get_text() {
                    let should_process = {
                        if text.is_empty() { return; }
                        let text_hash = hash_bytes(text.as_bytes());
                        let mut state_lock = state.lock().unwrap();
                        if text_hash == state_lock.last_text_hash {
                            false
                        } else {
                            state_lock.last_text_hash = text_hash;
                            true
                        }
                    };

                    if should_process {
                        info!("Processing copied text: {}", &text[..text.len().min(80)]);
                        let subtype = get_text_subtype(&text);
                        upload_and_notify(&config, "text", subtype.as_deref(), text.as_bytes(), "Text").await;
                    }
                }
            }
        });
    }
}

/// 启动剪贴板监控
pub fn start_monitoring(app: AppHandle) {
    info!("Initializing clipboard watcher...");

    thread::spawn(move || {
        let handler = ClipboardHandlerImpl {
            app_handle: app,
            debounce_state: Arc::new(Mutex::new(DebounceState::default())),
        };

        let Ok(mut watcher) = ClipboardWatcherContext::new() else {
            error!("Failed to create clipboard watcher context.");
            return;
        };
        
        let _shutdown_channel = watcher.add_handler(handler).get_shutdown_channel();
        
        info!("Clipboard watcher started.");
        watcher.start_watch();

        info!("Clipboard watcher stopped.");
    });
}

/// Handles uploading content and showing notifications in a unified way.
async fn upload_and_notify(config: &crate::config::Config, content_type: &str, subtype: Option<&str>, data: &[u8], description: &str) {
    let full_type = subtype.map_or_else(
        || content_type.to_string(),
        |sub| format!("{}_{}", content_type, sub),
    );
    
    match upload_content(config, &full_type, data, description).await {
        Ok(_) => {
            info!("'{}' ({}) uploaded successfully", description, full_type);
            show_notification("Upload Success", &format!("'{}' uploaded", description));
        }
        Err(e) => {
            error!("'{}' ({}) upload failed: {}", description, full_type, e);
            show_notification("Upload Failed", &format!("'{}' upload error: {}", description, e));
        }
    }
}