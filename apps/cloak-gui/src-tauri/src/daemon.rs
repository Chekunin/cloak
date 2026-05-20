//! Daemon supervision — make the bundled `cloakd` "just work" for GUI users.
//!
//! A GUI-first user never touches a terminal, so the GUI is responsible for
//! the daemon. On launch it checks whether a `cloakd` is already reachable on
//! the Unix socket:
//!
//! - **Reachable** — adopt it. Someone (the CLI, a previous GUI run, a
//!   developer) already started it; leave its lifetime alone.
//! - **Not reachable** — spawn the `cloakd` binary shipped alongside the app.
//!   A daemon the GUI started is stopped again when the GUI quits.
//!
//! This keeps the common case ("double-click Cloak, it works") zero-config
//! while never trampling a daemon the user is managing themselves.

use std::path::{Path, PathBuf};
use std::process::{Child, Command, Stdio};
use std::sync::Mutex;
use std::time::{Duration, Instant};

/// How long to wait for a freshly spawned daemon to start accepting.
const READY_TIMEOUT: Duration = Duration::from_secs(10);

/// Supervises an optional `cloakd` child process.
///
/// Registered as Tauri managed state; `Mutex<Option<Child>>` is `Send + Sync`
/// so it is reachable from the setup thread and the exit handler alike.
#[derive(Default)]
pub struct DaemonSupervisor {
    /// `Some` only when *this* process spawned the daemon.
    owned: Mutex<Option<Child>>,
}

impl DaemonSupervisor {
    pub fn new() -> Self {
        Self::default()
    }

    /// Ensure a daemon is reachable at `socket_path`, spawning the bundled
    /// `cloakd` if necessary. Blocking — intended to run on a background
    /// thread so the window can open immediately.
    pub fn ensure_running(&self, socket_path: &Path) -> Result<(), String> {
        if daemon_reachable(socket_path) {
            tracing::info!("daemon already running — adopting it");
            return Ok(());
        }

        let bin = locate_cloakd();
        tracing::info!(cloakd = %bin.display(), "no daemon found — starting bundled cloakd");

        let mut cmd = Command::new(&bin);
        cmd.arg("-foreground").stdin(Stdio::null());
        match open_daemon_log() {
            Some((out, err)) => {
                cmd.stdout(out).stderr(err);
            }
            None => {
                cmd.stdout(Stdio::null()).stderr(Stdio::null());
            }
        }
        let child = cmd
            .spawn()
            .map_err(|e| format!("could not start cloakd ({}): {e}", bin.display()))?;
        *self.owned.lock().unwrap() = Some(child);

        wait_for_socket(socket_path)
    }

    /// Stop the daemon this process started. No-op when the GUI adopted an
    /// already-running daemon — that one is not ours to stop.
    pub fn shutdown(&self) {
        let Some(mut child) = self.owned.lock().unwrap().take() else {
            return;
        };
        tracing::info!("stopping GUI-managed daemon");

        // Prefer a graceful stop: SIGTERM lets cloakd lock the vault, close
        // endpoints and remove its socket. Shelling out to `kill` keeps this
        // free of `unsafe` libc calls (the crate forbids `unsafe_code`).
        #[cfg(unix)]
        {
            let _ = Command::new("kill")
                .arg("-TERM")
                .arg(child.id().to_string())
                .status();
            for _ in 0..20 {
                match child.try_wait() {
                    Ok(Some(_)) => return,
                    Ok(None) => std::thread::sleep(Duration::from_millis(100)),
                    Err(_) => break,
                }
            }
        }
        // Fallback (or non-unix): hard kill.
        let _ = child.kill();
        let _ = child.wait();
    }
}

/// True when something is accepting connections on the daemon socket.
#[cfg(unix)]
fn daemon_reachable(socket_path: &Path) -> bool {
    std::os::unix::net::UnixStream::connect(socket_path).is_ok()
}

#[cfg(not(unix))]
fn daemon_reachable(_socket_path: &Path) -> bool {
    // Windows named-pipe probing is a v1.x task; assume an external daemon
    // so the GUI never tries to spawn one it can't yet manage.
    true
}

/// Poll until the daemon socket accepts a connection or the timeout elapses.
fn wait_for_socket(socket_path: &Path) -> Result<(), String> {
    let deadline = Instant::now() + READY_TIMEOUT;
    while Instant::now() < deadline {
        if daemon_reachable(socket_path) {
            tracing::info!("daemon is ready");
            return Ok(());
        }
        std::thread::sleep(Duration::from_millis(100));
    }
    Err(format!(
        "daemon did not start accepting within {}s",
        READY_TIMEOUT.as_secs()
    ))
}

#[cfg(windows)]
const CLOAKD_NAME: &str = "cloakd.exe";
#[cfg(not(windows))]
const CLOAKD_NAME: &str = "cloakd";

/// Locate the `cloakd` binary. Resolution order:
///  1. `$CLOAKD_BIN` — explicit override, handy during development.
///  2. Next to the GUI executable — where Tauri places the bundled sidecar
///     (`externalBin`): inside `Cloak.app/Contents/MacOS/` for a release
///     build, or `target/<profile>/` under `tauri dev`.
///  3. `cloakd` on `$PATH` — last resort.
fn locate_cloakd() -> PathBuf {
    if let Ok(p) = std::env::var("CLOAKD_BIN") {
        if !p.is_empty() {
            return PathBuf::from(p);
        }
    }
    if let Ok(exe) = std::env::current_exe() {
        if let Some(candidate) = exe.parent().map(|d| d.join(CLOAKD_NAME)) {
            if candidate.exists() {
                return candidate;
            }
        }
    }
    // Rely on PATH. If cloakd isn't there either, the spawn fails and the
    // caller logs it — the UI falls back to its "daemon unreachable" banner.
    PathBuf::from(CLOAKD_NAME)
}

/// Open `~/.cloak/cloakd.log` (append) for the spawned daemon's stdout and
/// stderr, so its operational logs are inspectable. Best-effort.
fn open_daemon_log() -> Option<(Stdio, Stdio)> {
    let home = crate::paths::home().ok()?;
    let _ = std::fs::create_dir_all(&home);
    let file = std::fs::OpenOptions::new()
        .create(true)
        .append(true)
        .open(home.join("cloakd.log"))
        .ok()?;
    let clone = file.try_clone().ok()?;
    Some((Stdio::from(file), Stdio::from(clone)))
}
