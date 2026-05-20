//! Cloak GUI — Tauri 2 entry point.
//!
//! `main.rs` is a thin wrapper around `run()` so we can be built as a library
//! crate (required for the mobile target later) and reuse the same builder
//! configuration for both desktop and mobile.

mod client;
mod commands;
mod daemon;
mod error;
mod exec;
mod paths;
mod state;
mod tray;

use tauri::{Manager, RunEvent, WindowEvent};
use tracing_subscriber::EnvFilter;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    init_logging();

    let app_state = state::AppState::new().expect("initialize app state");

    let app = tauri::Builder::default()
        .manage(app_state)
        .manage(daemon::DaemonSupervisor::new())
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

            // Make the daemon "just work" for GUI-first users: if no `cloakd`
            // is running, start the one bundled with the app. Done off the
            // main thread so the window opens immediately — the connection
            // banner reflects progress as the daemon comes up.
            let handle = app.handle().clone();
            std::thread::spawn(move || match paths::socket_path() {
                Ok(socket) => {
                    if let Err(e) = handle
                        .state::<daemon::DaemonSupervisor>()
                        .ensure_running(&socket)
                    {
                        tracing::warn!(error = %e, "daemon autostart failed");
                    }
                }
                Err(e) => tracing::warn!(error = %e, "cannot resolve daemon socket path"),
            });
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
        .build(tauri::generate_context!())
        .expect("error while building cloak-gui");

    // On a real quit (tray Quit / cmd-Q), stop the daemon if we started it.
    app.run(|app_handle, event| {
        if let RunEvent::Exit = event {
            app_handle.state::<daemon::DaemonSupervisor>().shutdown();
        }
    });
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
