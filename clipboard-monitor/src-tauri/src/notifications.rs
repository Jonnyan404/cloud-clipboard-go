use notify_rust::Notification;

pub fn show_notification(title: &str, body: &str) {
    Notification::new()
        .summary(title)
        .body(body)
        .show()
        .unwrap();
}