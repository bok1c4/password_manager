use serde::{Deserialize, Serialize};
use std::path::PathBuf;
use tauri::State;

use crate::app_state::AppState;

#[derive(Debug, Serialize, Deserialize)]
pub struct Entry {
    pub id: String,
    pub site: String,
    pub username: String,
    #[serde(rename = "encrypted_password")]
    pub encrypted_password: Option<String>,
    pub notes: Option<String>,
    #[serde(rename = "created_at")]
    pub created_at: Option<String>,
    #[serde(rename = "updated_at")]
    pub updated_at: Option<String>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct Device {
    pub id: String,
    pub name: String,
    pub fingerprint: String,
    pub trusted: bool,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct SyncStatus {
    pub initialized: bool,
    #[serde(rename = "last_sync")]
    pub last_sync: Option<i64>,
    #[serde(rename = "pending_changes")]
    pub pending_changes: i32,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct VaultInfo {
    pub name: String,
    pub active: bool,
    pub initialized: bool,
}

#[derive(Debug, Serialize, Deserialize)]
struct ApiResponse {
    success: bool,
    error: Option<String>,
    data: Option<serde_json::Value>,
}

const API_BASE: &str = "http://localhost:18475/api";

fn get_vault_path() -> PathBuf {
    dirs::home_dir()
        .unwrap_or_else(|| PathBuf::from("."))
        .join(".pwman")
}

fn get_config_path() -> PathBuf {
    get_vault_path().join("config.json")
}

fn api_call(
    endpoint: &str,
    method: &str,
    body: Option<&str>,
    token: Option<&str>,
) -> Result<ApiResponse, String> {
    let client = reqwest::blocking::Client::new();

    let url = format!("{}{}", API_BASE, endpoint);

    let mut request = match method {
        "POST" => client.post(&url),
        "GET" => client.get(&url),
        _ => return Err("Invalid method".to_string()),
    };

    if let Some(body) = body {
        request = request.body(body.to_string());
    }

    request = request.header("Content-Type", "application/json");

    // Add authorization token if provided
    if let Some(token) = token {
        request = request.header("Authorization", format!("Bearer {}", token));
    }

    let response = request.send().map_err(|e| e.to_string())?;

    let result: ApiResponse = response.json().map_err(|e| e.to_string())?;

    if let Some(error) = result.error {
        return Err(error);
    }

    Ok(result)
}

#[tauri::command]
pub fn is_initialized() -> Result<bool, String> {
    let result = api_call("/is_initialized", "GET", None, None)?;
    if let Some(data) = result.data {
        if let Some(initialized) = data.get("initialized").and_then(|v| v.as_bool()) {
            return Ok(initialized);
        }
    }
    Ok(false)
}

#[tauri::command]
pub fn init_vault(name: String, password: String, vault: Option<String>) -> Result<String, String> {
    let body = if vault.is_some() {
        serde_json::json!({
            "name": name,
            "password": password,
            "vault": vault
        })
        .to_string()
    } else {
        serde_json::json!({
            "name": name,
            "password": password
        })
        .to_string()
    };
    let result = api_call("/init", "POST", Some(&body), None)?;

    if let Some(data) = result.data {
        if let Some(device_id) = data.get("device_id").and_then(|v| v.as_str()) {
            return Ok(device_id.to_string());
        }
    }

    Ok("initialized".to_string())
}

#[tauri::command]
pub fn unlock_vault(password: String, state: State<'_, AppState>) -> Result<bool, String> {
    let body = serde_json::json!({
        "password": password
    })
    .to_string();
    let result = api_call("/unlock", "POST", Some(&body), None)?;

    if result.success {
        // Store the auth token from the response
        if let Some(ref data) = result.data {
            if let Some(token) = data.get("token").and_then(|v| v.as_str()) {
                let mut token_guard = state.token.lock().map_err(|e| e.to_string())?;
                *token_guard = Some(token.to_string());
            }
        }

        let mut unlocked: std::sync::MutexGuard<bool> =
            state.unlocked.lock().map_err(|e| e.to_string())?;
        *unlocked = true;
        Ok(true)
    } else {
        Ok(false)
    }
}

#[tauri::command]
pub fn lock_vault(state: State<'_, AppState>) -> Result<bool, String> {
    // Get token and make API call to invalidate it on backend
    let token_str: Option<String> = {
        let token = state.token.lock().map_err(|e| e.to_string())?;
        token.clone()
    };

    // Make the API call - don't ignore errors
    if let Some(ref token) = token_str {
        api_call("/lock", "POST", None, Some(token))
            .map_err(|e| format!("Failed to lock vault on server: {}", e))?;
    }

    // Clear local state atomically
    let mut unlocked = state.unlocked.lock().map_err(|e| e.to_string())?;
    *unlocked = false;
    drop(unlocked);

    let mut token_guard = state.token.lock().map_err(|e| e.to_string())?;
    *token_guard = None;

    Ok(true)
}

#[tauri::command]
pub fn is_unlocked(state: State<'_, AppState>) -> Result<bool, String> {
    // First check local state
    let locally_unlocked = state.unlocked.lock().map(|u| *u).unwrap_or(false);
    if !locally_unlocked {
        return Ok(false);
    }

    // Verify with backend (requires token)
    let token = state.token.lock().map_err(|e| e.to_string())?;
    if let Some(token_str) = token.as_deref() {
        match api_call("/is_unlocked", "GET", None, Some(token_str)) {
            Ok(result) => {
                if let Some(data) = result.data {
                    if let Some(unlocked) = data.get("unlocked").and_then(|v| v.as_bool()) {
                        return Ok(unlocked);
                    }
                }
            }
            Err(_) => {
                // Backend check failed, clear local state
                drop(token);
                let mut unlocked_guard = state.unlocked.lock().map_err(|e| e.to_string())?;
                *unlocked_guard = false;
                let mut token_guard = state.token.lock().map_err(|e| e.to_string())?;
                *token_guard = None;
                return Ok(false);
            }
        }
    }

    Ok(locally_unlocked)
}

#[tauri::command]
pub fn get_entries(state: State<'_, AppState>) -> Result<Vec<Entry>, String> {
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/entries", "GET", None, token_str)?;

    if let Some(data) = result.data {
        let entries: Vec<Entry> = serde_json::from_value(data).map_err(|e| e.to_string())?;
        Ok(entries)
    } else {
        Ok(vec![])
    }
}

#[tauri::command]
pub fn add_entry(
    site: String,
    username: String,
    password: String,
    notes: String,
    state: State<'_, AppState>,
) -> Result<String, String> {
    let body = serde_json::json!({
        "site": site,
        "username": username,
        "password": password,
        "notes": notes
    })
    .to_string();

    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/entries/add", "POST", Some(&body), token_str)?;

    if let Some(data) = result.data {
        if let Some(id) = data.get("id").and_then(|v| v.as_str()) {
            return Ok(id.to_string());
        }
    }

    Ok("added".to_string())
}

#[tauri::command]
pub fn update_entry(
    id: String,
    site: String,
    username: String,
    password: String,
    notes: String,
    state: State<'_, AppState>,
) -> Result<String, String> {
    let body = serde_json::json!({
        "id": id,
        "site": site,
        "username": username,
        "password": password,
        "notes": notes
    })
    .to_string();

    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/entries/update", "POST", Some(&body), token_str)?;

    if result.success {
        Ok("updated".to_string())
    } else if let Some(error) = result.error {
        Err(error)
    } else {
        Err("unknown error".to_string())
    }
}

#[tauri::command]
pub fn delete_entry(id: String, state: State<'_, AppState>) -> Result<bool, String> {
    let body = format!(r#"{{"id":"{}"}}"#, id);
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/entries/delete", "POST", Some(&body), token_str)?;
    Ok(result.success)
}

#[tauri::command]
pub fn get_password(id: String, state: State<'_, AppState>) -> Result<String, String> {
    let body = format!(r#"{{"id":"{}"}}"#, id);
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/entries/get_password", "POST", Some(&body), token_str)?;

    if let Some(data) = result.data {
        if let Some(password) = data.get("password").and_then(|v| v.as_str()) {
            return Ok(password.to_string());
        }
    }

    Err("Failed to get password".to_string())
}

#[tauri::command]
pub fn get_devices(state: State<'_, AppState>) -> Result<Vec<Device>, String> {
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/devices", "GET", None, token_str)?;

    if let Some(data) = result.data {
        let devices: Vec<Device> = serde_json::from_value(data).map_err(|e| e.to_string())?;
        Ok(devices)
    } else {
        Ok(vec![])
    }
}

#[tauri::command]
pub fn get_sync_status(state: State<'_, AppState>) -> Result<SyncStatus, String> {
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/sync/status", "GET", None, token_str)?;

    if let Some(data) = result.data {
        let status: SyncStatus = serde_json::from_value(data).map_err(|e| e.to_string())?;
        Ok(status)
    } else {
        Ok(SyncStatus {
            initialized: false,
            last_sync: None,
            pending_changes: 0,
        })
    }
}

#[tauri::command]
pub fn init_sync(remote: String, state: State<'_, AppState>) -> Result<bool, String> {
    let body = serde_json::json!({
        "remote": remote
    })
    .to_string();
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/sync/init", "POST", Some(&body), token_str)?;
    Ok(result.success)
}

#[tauri::command]
pub fn sync_now(state: State<'_, AppState>) -> Result<bool, String> {
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    // Pull then push
    api_call("/sync/pull", "POST", None, token_str)?;
    api_call("/sync/push", "POST", None, token_str)?;
    Ok(true)
}

#[tauri::command]
pub fn sync_push(state: State<'_, AppState>) -> Result<bool, String> {
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/sync/push", "POST", None, token_str)?;
    Ok(result.success)
}

#[tauri::command]
pub fn sync_pull(state: State<'_, AppState>) -> Result<bool, String> {
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/sync/pull", "POST", None, token_str)?;
    Ok(result.success)
}

#[tauri::command]
pub fn get_vaults(state: State<'_, AppState>) -> Result<Vec<VaultInfo>, String> {
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/vaults", "GET", None, token_str)?;

    if let Some(data) = result.data {
        let vaults: Vec<VaultInfo> = serde_json::from_value(data).map_err(|e| e.to_string())?;
        Ok(vaults)
    } else {
        Ok(vec![])
    }
}

#[tauri::command]
pub fn use_vault(vault: String, state: State<'_, AppState>) -> Result<bool, String> {
    // Clear token and unlocked state BEFORE switching vaults
    // The vaults/use endpoint doesn't require auth, and we want to ensure
    // clean state even if the request fails
    {
        let mut unlocked_guard = state.unlocked.lock().map_err(|e| e.to_string())?;
        *unlocked_guard = false;
    }
    {
        let mut token_guard = state.token.lock().map_err(|e| e.to_string())?;
        *token_guard = None;
    }

    let body = serde_json::json!({
        "name": vault
    })
    .to_string();

    // No token needed for vault switch - endpoint is public
    let result = api_call("/vaults/use", "POST", Some(&body), None)?;

    Ok(result.success)
}

#[tauri::command]
pub fn create_vault(name: String, state: State<'_, AppState>) -> Result<bool, String> {
    let body = serde_json::json!({
        "name": name
    })
    .to_string();
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/vaults/create", "POST", Some(&body), token_str)?;
    Ok(result.success)
}

#[tauri::command]
pub fn p2p_status(state: State<'_, AppState>) -> Result<bool, String> {
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/p2p/status", "GET", None, token_str)?;
    Ok(result.success)
}

#[tauri::command]
pub fn p2p_start(state: State<'_, AppState>) -> Result<bool, String> {
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/p2p/start", "POST", None, token_str)?;
    Ok(result.success)
}

#[tauri::command]
pub fn p2p_stop(state: State<'_, AppState>) -> Result<bool, String> {
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/p2p/stop", "POST", None, token_str)?;
    Ok(result.success)
}

#[tauri::command]
pub fn p2p_peers(state: State<'_, AppState>) -> Result<Vec<serde_json::Value>, String> {
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/p2p/peers", "GET", None, token_str)?;

    if let Some(data) = result.data {
        let peers: Vec<serde_json::Value> =
            serde_json::from_value(data).map_err(|e| e.to_string())?;
        Ok(peers)
    } else {
        Ok(vec![])
    }
}

#[tauri::command]
pub fn p2p_connect(address: String, state: State<'_, AppState>) -> Result<bool, String> {
    let body = serde_json::json!({
        "address": address
    })
    .to_string();
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/p2p/connect", "POST", Some(&body), token_str)?;
    Ok(result.success)
}

#[tauri::command]
pub fn p2p_disconnect(peer_id: String, state: State<'_, AppState>) -> Result<bool, String> {
    let body = serde_json::json!({
        "peer_id": peer_id
    })
    .to_string();
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/p2p/disconnect", "POST", Some(&body), token_str)?;
    Ok(result.success)
}

#[tauri::command]
pub fn p2p_approvals(state: State<'_, AppState>) -> Result<Vec<serde_json::Value>, String> {
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/p2p/approvals", "GET", None, token_str)?;

    if let Some(data) = result.data {
        let approvals: Vec<serde_json::Value> =
            serde_json::from_value(data).map_err(|e| e.to_string())?;
        Ok(approvals)
    } else {
        Ok(vec![])
    }
}

#[tauri::command]
pub fn p2p_approve(device_id: String, state: State<'_, AppState>) -> Result<bool, String> {
    let body = serde_json::json!({
        "device_id": device_id
    })
    .to_string();
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/p2p/approve", "POST", Some(&body), token_str)?;
    Ok(result.success)
}

#[tauri::command]
pub fn p2p_reject(
    device_id: String,
    reason: String,
    state: State<'_, AppState>,
) -> Result<bool, String> {
    let body = serde_json::json!({
        "device_id": device_id,
        "reason": reason
    })
    .to_string();
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/p2p/reject", "POST", Some(&body), token_str)?;
    Ok(result.success)
}

#[tauri::command]
pub fn p2p_sync(full_sync: bool, state: State<'_, AppState>) -> Result<bool, String> {
    let body = serde_json::json!({
        "full_sync": full_sync
    })
    .to_string();
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/p2p/sync", "POST", Some(&body), token_str)?;
    Ok(result.success)
}

#[tauri::command]
pub fn delete_vault(
    name: String,
    password: String,
    state: State<'_, AppState>,
) -> Result<bool, String> {
    let body = serde_json::json!({
        "name": name,
        "password": password
    })
    .to_string();
    // No token needed - endpoint verifies password directly
    let result = api_call("/vaults/delete", "POST", Some(&body), None)?;

    // Clear token if deleting the currently active vault
    if result.success {
        let mut unlocked_guard = state.unlocked.lock().map_err(|e| e.to_string())?;
        *unlocked_guard = false;
        drop(unlocked_guard);
        let mut token_guard = state.token.lock().map_err(|e| e.to_string())?;
        *token_guard = None;
    }

    Ok(result.success)
}

#[tauri::command]
pub fn generate_password(length: u32, state: State<'_, AppState>) -> Result<String, String> {
    let body = serde_json::json!({
        "length": length
    })
    .to_string();
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/generate", "POST", Some(&body), token_str)?;

    if let Some(data) = result.data {
        if let Some(password) = data.get("password").and_then(|v| v.as_str()) {
            return Ok(password.to_string());
        }
    }

    Err("Failed to generate password".to_string())
}

#[tauri::command]
pub fn pairing_generate(state: State<'_, AppState>) -> Result<serde_json::Value, String> {
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/pairing/generate", "GET", None, token_str)?;

    if let Some(data) = result.data {
        Ok(data)
    } else {
        Err("Failed to generate pairing code".to_string())
    }
}

#[tauri::command]
pub fn pairing_join(
    code: String,
    device_name: String,
    password: String,
    state: State<'_, AppState>,
) -> Result<bool, String> {
    let body = serde_json::json!({
        "code": code,
        "device_name": device_name,
        "password": password
    })
    .to_string();
    // No token needed - joining is a public endpoint
    let result = api_call("/pairing/join", "POST", Some(&body), None)?;
    Ok(result.success)
}

#[tauri::command]
pub fn pairing_status(state: State<'_, AppState>) -> Result<serde_json::Value, String> {
    let token = state.token.lock().map_err(|e| e.to_string())?;
    let token_str = token.as_deref();
    let result = api_call("/pairing/status", "GET", None, token_str)?;

    if let Some(data) = result.data {
        Ok(data)
    } else {
        Err("Failed to get pairing status".to_string())
    }
}
