//! Cloak GUI — Tauri 2 entry point.
//!
//! `main.rs` is a thin wrapper around `run()` so we can be built as a library
//! crate (required for the mobile target later) and reuse the same builder
//! configuration for both desktop and mobile.

mod client;
mod commands;
mod error;
mod exec;
mod paths;
mod state;
mod tray;

use tauri::WindowEvent;
use tracing_subscriber::EnvFilter;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    init_logging();

    let app_state = state::AppState::new().expect("initialize app state");

    tauri::Builder::default()
        .manage(app_state)
        .invoke_handler(tauri::generate_handler![
            commands::daemon_ping,
            commands::daemon_info,
            commands::vault_status,
            commands::vault_init,
            commands::vault_unlock,
            commands::vault_lock,
            commands::secrets_list,
            commands::secrets_get,
            commands::secrets_create,
            commands::secrets_update,
            commands::secrets_delete,
            commands::endpoints_list,
            commands::endpoints_open,
            commands::endpoints_close,
            commands::secrets_exec,
            commands::tokens_list,
            commands::tokens_create,
            commands::tokens_revoke,
            commands::audit_tail,
            commands::gui_set_token,
            commands::gui_reload_cli_token,
        ])
        .setup(|app| {
            tray::build(app.handle())?;
            Ok(())
        })
        .on_window_event(|window, event| {
            // The close button hides to tray rather than killing the process.
            // The user truly quits via the tray menu's Quit item or cmd-Q.
            if let WindowEvent::CloseRequested { api, .. } = event {
                let _ = window.hide();
                api.prevent_close();
            }
        })
        .run(tauri::generate_context!())
        .expect("error while running cloak-gui");
}

fn init_logging() {
    let filter = EnvFilter::try_from_env("CLOAK_GUI_LOG")
        .unwrap_or_else(|_| EnvFilter::new("info,cloak_gui_lib=debug"));
    tracing_subscriber::fmt()
        .with_env_filter(filter)
        .with_target(false)
        .compact()
        .init();
}
