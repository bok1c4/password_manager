use std::sync::Mutex;

pub struct AppState {
    pub unlocked: Mutex<bool>,
    pub token: Mutex<Option<String>>,
}

impl Default for AppState {
    fn default() -> Self {
        Self {
            unlocked: Mutex::new(false),
            token: Mutex::new(None),
        }
    }
}
