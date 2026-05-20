//! Tray icon — Cloak's always-on presence in the menu bar / system tray.
//!
//! Three menu items in Phase 1: Show window / Lock vault / Quit. The window's
//! close button hides to tray rather than terminating the process; the user
//! truly quits via the Quit menu item or `cmd-Q` on macOS.

use tauri::menu::{Menu, MenuEvent, MenuItem, PredefinedMenuItem};
use tauri::tray::{TrayIconBuilder, TrayIconEvent};
use tauri::{AppHandle, Emitter, Manager, Runtime};

use crate::state::AppState;

const ID_SHOW: &str = "show_window";
const ID_LOCK: &str = "lock_vault";
const ID_UPDATE: &str = "check_update";
const ID_QUIT: &str = "quit";

/// Event the tray emits to ask the frontend to run an update check. The
/// frontend listens for it and drives the update dialog.
pub const EVENT_CHECK_UPDATE: &str = "menu://check-update";

/// Build the tray icon and register its menu/event handlers.
///
/// Called once during `tauri::Builder::setup`.
pub fn build<R: Runtime>(app: &AppHandle<R>) -> tauri::Result<()> {
    let menu = build_menu(app)?;

    TrayIconBuilder::with_id("main")
        .tooltip("Cloak")
        .icon(app.default_window_icon().cloned().ok_or_else(|| {
            tauri::Error::AssetNotFound("tray icon missing — check tauri.conf.json".into())
        })?)
        .menu(&menu)
        .show_menu_on_left_click(false)
        .on_menu_event(on_menu_event)
        .on_tray_icon_event(on_tray_icon_event)
        .build(app)?;

    Ok(())
}

fn build_menu<R: Runtime>(app: &AppHandle<R>) -> tauri::Result<Menu<R>> {
    let show = MenuItem::with_id(app, ID_SHOW, "Show Cloak", true, None::<&str>)?;
    let lock = MenuItem::with_id(app, ID_LOCK, "Lock vault", true, Some("CmdOrCtrl+L"))?;
    let update = MenuItem::with_id(app, ID_UPDATE, "Check for Updates…", true, None::<&str>)?;
    let sep = PredefinedMenuItem::separator(app)?;
    let quit = MenuItem::with_id(app, ID_QUIT, "Quit", true, Some("CmdOrCtrl+Q"))?;
    Menu::with_items(app, &[&show, &lock, &update, &sep, &quit])
}

// Signatures are fixed by Tauri's tray callback contracts — these args are
// owned values even when we only read them.
#[allow(clippy::needless_pass_by_value)]
fn on_menu_event<R: Runtime>(app: &AppHandle<R>, event: MenuEvent) {
    match event.id().as_ref() {
        ID_SHOW => show_main_window(app),
        ID_LOCK => spawn_lock(app.clone()),
        ID_UPDATE => {
            // Surface the window so the update dialog is visible, then ask the
            // frontend to run the check.
            show_main_window(app);
            if let Err(err) = app.emit(EVENT_CHECK_UPDATE, ()) {
                tracing::warn!(error = %err, "tray: could not emit update-check event");
            }
        }
        ID_QUIT => app.exit(0),
        _ => {}
    }
}

#[allow(clippy::needless_pass_by_value)]
fn on_tray_icon_event<R: Runtime>(tray: &tauri::tray::TrayIcon<R>, event: TrayIconEvent) {
    // Left-click toggles the main window. Right-click opens the menu (handled
    // by Tauri automatically when `show_menu_on_left_click(false)`).
    if let TrayIconEvent::Click {
        button: tauri::tray::MouseButton::Left,
        button_state: tauri::tray::MouseButtonState::Up,
        ..
    } = event
    {
        let app = tray.app_handle().clone();
        toggle_main_window(&app);
    }
}

fn show_main_window<R: Runtime>(app: &AppHandle<R>) {
    if let Some(win) = app.get_webview_window("main") {
        let _ = win.show();
        let _ = win.set_focus();
        let _ = win.unminimize();
    }
}

fn toggle_main_window<R: Runtime>(app: &AppHandle<R>) {
    let Some(win) = app.get_webview_window("main") else { return };
    let visible = win.is_visible().unwrap_or(false);
    let focused = win.is_focused().unwrap_or(false);
    if visible && focused {
        let _ = win.hide();
    } else {
        let _ = win.show();
        let _ = win.set_focus();
    }
}

/// Lock the vault from the tray menu. Runs on the Tauri async runtime so we
/// don't block the menu event loop.
fn spawn_lock<R: Runtime>(app: AppHandle<R>) {
    tauri::async_runtime::spawn(async move {
        let Some(state) = app.try_state::<AppState>() else { return };
        match state.client().await {
            Ok(client) => {
                if let Err(err) = client.transport().vault_lock().await {
                    tracing::warn!(error = %err, "tray: vault.lock failed");
                }
            }
            Err(err) => {
                tracing::warn!(error = %err, "tray: cannot reach daemon");
            }
        }
    });
}
