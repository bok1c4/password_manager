#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod app_state;
mod commands;

use app_state::AppState;

fn main() {
    env_logger::init();
    log::info!("Starting Password Manager...");

    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_clipboard_manager::init())
        .manage(AppState::default())
        .invoke_handler(tauri::generate_handler![
            commands::is_initialized,
            commands::init_vault,
            commands::unlock_vault,
            commands::lock_vault,
            commands::is_unlocked,
            commands::get_entries,
            commands::add_entry,
            commands::update_entry,
            commands::delete_entry,
            commands::get_password,
            commands::get_devices,
            commands::get_sync_status,
            commands::init_sync,
            commands::sync_now,
            commands::get_vaults,
            commands::use_vault,
            commands::create_vault,
            commands::delete_vault,
            commands::p2p_status,
            commands::p2p_start,
            commands::p2p_stop,
            commands::p2p_peers,
            commands::p2p_connect,
            commands::p2p_disconnect,
            commands::p2p_approvals,
            commands::p2p_approve,
            commands::p2p_reject,
            commands::p2p_sync,
            commands::generate_password,
            commands::pairing_generate,
            commands::pairing_join,
            commands::pairing_status,
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
