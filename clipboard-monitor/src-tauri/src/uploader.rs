use reqwest::{Client, multipart};
use crate::config::Config;
use log::{info, error};

pub async fn upload_content(config: &Config, content_type: &str, data: &[u8], filename: &str) -> Result<(), Box<dyn std::error::Error>> {
    // 全局文件大小限制检查
    let size_mb = data.len() as f64 / (1024.0 * 1024.0);
    if size_mb > config.max_file_size_mb as f64 {
        return Err(format!("File too large: {:.2} MB exceeds limit of {} MB", size_mb, config.max_file_size_mb).into());
    }

    let (urls, is_file_upload) = match content_type {
        "text" | "text_url" | "text_email" | "text_color" => {
            if !config.enable_text { return Ok(()); }
            (&config.text_urls, false)
        }
        _ => { // "file", "image", etc.
            if !config.enable_file { return Ok(()); }
            (&config.file_urls, true)
        }
    };

    let client = Client::new();
    for url in urls {
        let mut request_builder = client.post(url);

        // 如果有认证 token，添加 Authorization 头
        if let Some(token) = &config.auth_token {
            request_builder = request_builder.header("Authorization", format!("Bearer {}", token));
        }

        let response = if is_file_upload {
            // --- 文件上传逻辑 ---
            let part = multipart::Part::bytes(data.to_vec())
                .file_name(filename.to_string());

            let form = multipart::Form::new().part("file", part);

            info!("Sending POST (multipart/form-data) to: {}", url);
            request_builder.multipart(form).send().await?
        } else {
            // --- 文本上传逻辑 ---
            info!("Sending POST (text/plain) to: {}", url);
            request_builder
                .header("Content-Type", "text/plain")
                .body(data.to_vec())
                .send()
                .await?
        };

        let status = response.status();
        let body = response.text().await?;

        if status.is_success() {
            info!("Uploaded {} to {}. Response: {}", content_type, url, body);
            return Ok(());
        } else {
            error!("Upload to {} failed with status: {}", url, status);
            error!("Response body: {}", body);
        }
    }
    Err("All upload URLs failed".into())
}