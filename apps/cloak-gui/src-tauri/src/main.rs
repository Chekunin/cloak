// Prevents an extra console window on Windows in release builds. Does not
// affect dev (`cargo tauri dev` keeps stdout/stderr).
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    cloak_gui_lib::run();
}
