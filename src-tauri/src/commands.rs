use serde::{Deserialize, Serialize};
use std::fs;
use std::path::PathBuf;
use std::sync::Mutex;
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

fn api_call(endpoint: &str, method: &str, body: Option<&str>) -> Result<ApiResponse, String> {
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

    let response = request
        .header("Content-Type", "application/json")
        .send()
        .map_err(|e| e.to_string())?;

    let result: ApiResponse = response.json().map_err(|e| e.to_string())?;

    if let Some(error) = result.error {
        return Err(error);
    }

    Ok(result)
}

#[tauri::command]
pub fn is_initialized() -> Result<bool, String> {
    let result = api_call("/is_initialized", "GET", None)?;
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
    let result = api_call("/init", "POST", Some(&body))?;

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
    let result = api_call("/unlock", "POST", Some(&body))?;

    if result.success {
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
    let _ = api_call("/lock", "POST", None);

    let mut unlocked: std::sync::MutexGuard<bool> =
        state.unlocked.lock().map_err(|e| e.to_string())?;
    *unlocked = false;
    Ok(true)
}

#[tauri::command]
pub fn is_unlocked(state: State<'_, AppState>) -> bool {
    state.unlocked.lock().map(|u| *u).unwrap_or(false)
}

#[tauri::command]
pub fn get_entries() -> Result<Vec<Entry>, String> {
    let result = api_call("/entries", "GET", None)?;

    if let Some(data) = result.data {
        let entries: Vec<Entry> = serde_json::from_value(data).map_err(|e| e.to_string())?;
        Ok(entries)
    } else {
        Ok(vec![])
    }
}

#[tauri::command]
pub fn get_entry(id: String) -> Result<Entry, String> {
    Err("Not implemented".to_string())
}

#[tauri::command]
pub fn add_entry(
    site: String,
    username: String,
    password: String,
    notes: String,
) -> Result<String, String> {
    let body = serde_json::json!({
        "site": site,
        "username": username,
        "password": password,
        "notes": notes
    })
    .to_string();

    let result = api_call("/entries/add", "POST", Some(&body))?;

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
) -> Result<String, String> {
    let body = serde_json::json!({
        "id": id,
        "site": site,
        "username": username,
        "password": password,
        "notes": notes
    })
    .to_string();

    let result = api_call("/entries/update", "POST", Some(&body))?;

    if result.success {
        Ok("updated".to_string())
    } else if let Some(error) = result.error {
        Err(error)
    } else {
        Err("unknown error".to_string())
    }
}

#[tauri::command]
pub fn delete_entry(id: String) -> Result<bool, String> {
    let body = format!(r#"{{"id":"{}"}}"#, id);
    let result = api_call("/entries/delete", "POST", Some(&body))?;
    Ok(result.success)
}

#[tauri::command]
pub fn get_password(id: String) -> Result<String, String> {
    let body = format!(r#"{{"id":"{}"}}"#, id);
    let result = api_call("/entries/get_password", "POST", Some(&body))?;

    if let Some(data) = result.data {
        if let Some(password) = data.get("password").and_then(|v| v.as_str()) {
            return Ok(password.to_string());
        }
    }

    Err("Failed to get password".to_string())
}

#[tauri::command]
pub fn get_devices() -> Result<Vec<Device>, String> {
    let result = api_call("/devices", "GET", None)?;

    if let Some(data) = result.data {
        let devices: Vec<Device> = serde_json::from_value(data).map_err(|e| e.to_string())?;
        Ok(devices)
    } else {
        Ok(vec![])
    }
}

#[tauri::command]
pub fn get_sync_status() -> Result<SyncStatus, String> {
    let result = api_call("/sync/status", "GET", None)?;

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
pub fn init_sync(remote: String) -> Result<bool, String> {
    let body = serde_json::json!({
        "remote": remote
    })
    .to_string();
    let result = api_call("/sync/init", "POST", Some(&body))?;
    Ok(result.success)
}

#[tauri::command]
pub fn sync_now() -> Result<bool, String> {
    // Pull then push
    api_call("/sync/pull", "POST", None)?;
    api_call("/sync/push", "POST", None)?;
    Ok(true)
}

#[tauri::command]
pub fn sync_push() -> Result<bool, String> {
    let result = api_call("/sync/push", "POST", None)?;
    Ok(result.success)
}

#[tauri::command]
pub fn sync_pull() -> Result<bool, String> {
    let result = api_call("/sync/pull", "POST", None)?;
    Ok(result.success)
}

#[tauri::command]
pub fn get_vaults() -> Result<Vec<VaultInfo>, String> {
    let result = api_call("/vaults", "GET", None)?;

    if let Some(data) = result.data {
        let vaults: Vec<VaultInfo> = serde_json::from_value(data).map_err(|e| e.to_string())?;
        Ok(vaults)
    } else {
        Ok(vec![])
    }
}

#[tauri::command]
pub fn use_vault(vault: String) -> Result<bool, String> {
    let body = serde_json::json!({
        "vault": vault
    })
    .to_string();
    let result = api_call("/vaults/use", "POST", Some(&body))?;
    Ok(result.success)
}

#[tauri::command]
pub fn create_vault(name: String) -> Result<bool, String> {
    let body = serde_json::json!({
        "name": name
    })
    .to_string();
    let result = api_call("/vaults/create", "POST", Some(&body))?;
    Ok(result.success)
}

#[tauri::command]
pub fn p2p_status() -> Result<bool, String> {
    let result = api_call("/p2p/status", "GET", None)?;
    Ok(result.success)
}

#[tauri::command]
pub fn p2p_start() -> Result<bool, String> {
    let result = api_call("/p2p/start", "POST", None)?;
    Ok(result.success)
}

#[tauri::command]
pub fn p2p_stop() -> Result<bool, String> {
    let result = api_call("/p2p/stop", "POST", None)?;
    Ok(result.success)
}

#[tauri::command]
pub fn p2p_peers() -> Result<Vec<serde_json::Value>, String> {
    let result = api_call("/p2p/peers", "GET", None)?;

    if let Some(data) = result.data {
        let peers: Vec<serde_json::Value> =
            serde_json::from_value(data).map_err(|e| e.to_string())?;
        Ok(peers)
    } else {
        Ok(vec![])
    }
}

#[tauri::command]
pub fn p2p_connect(address: String) -> Result<bool, String> {
    let body = serde_json::json!({
        "address": address
    })
    .to_string();
    let result = api_call("/p2p/connect", "POST", Some(&body))?;
    Ok(result.success)
}

#[tauri::command]
pub fn p2p_disconnect(peer_id: String) -> Result<bool, String> {
    let body = serde_json::json!({
        "peer_id": peer_id
    })
    .to_string();
    let result = api_call("/p2p/disconnect", "POST", Some(&body))?;
    Ok(result.success)
}

#[tauri::command]
pub fn p2p_approvals() -> Result<Vec<serde_json::Value>, String> {
    let result = api_call("/p2p/approvals", "GET", None)?;

    if let Some(data) = result.data {
        let approvals: Vec<serde_json::Value> =
            serde_json::from_value(data).map_err(|e| e.to_string())?;
        Ok(approvals)
    } else {
        Ok(vec![])
    }
}

#[tauri::command]
pub fn p2p_approve(device_id: String) -> Result<bool, String> {
    let body = serde_json::json!({
        "device_id": device_id
    })
    .to_string();
    let result = api_call("/p2p/approve", "POST", Some(&body))?;
    Ok(result.success)
}

#[tauri::command]
pub fn p2p_reject(device_id: String, reason: String) -> Result<bool, String> {
    let body = serde_json::json!({
        "device_id": device_id,
        "reason": reason
    })
    .to_string();
    let result = api_call("/p2p/reject", "POST", Some(&body))?;
    Ok(result.success)
}
