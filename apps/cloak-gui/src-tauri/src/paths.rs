//! Resolve well-known Cloak file paths.
//!
//! Mirrors `internal/paths` in the Go codebase. Kept tiny on purpose — the
//! GUI only needs the socket path today; CLI-token path and config path are
//! added as the corresponding features land.
//!
//! Honors the same `$CLOAK_HOME` override the daemon and CLI honor.

use std::path::PathBuf;

/// Returns the path of the `cloakd` Unix socket.
///
/// Resolution order:
/// 1. `$CLOAK_HOME/cloakd.sock`
/// 2. `~/.cloak/cloakd.sock` on Unix
/// 3. `%APPDATA%\Cloak\cloakd.sock` on Windows (placeholder; full Windows
///    support is a v1.x task).
pub fn socket_path() -> std::io::Result<PathBuf> {
    Ok(home()?.join("cloakd.sock"))
}

/// Returns the path of the CLI's saved client token (if any).
///
/// The GUI reads this only as a *bootstrap* convenience for development;
/// production builds will store the GUI's own token in the OS keychain.
pub fn cli_token_path() -> std::io::Result<PathBuf> {
    Ok(home()?.join("cli_token"))
}

/// Returns the Cloak home directory.
pub fn home() -> std::io::Result<PathBuf> {
    if let Ok(custom) = std::env::var("CLOAK_HOME") {
        if !custom.is_empty() {
            return Ok(PathBuf::from(custom));
        }
    }
    let base = dirs_home()?;
    if cfg!(target_os = "windows") {
        Ok(base.join("Cloak"))
    } else {
        Ok(base.join(".cloak"))
    }
}

fn dirs_home() -> std::io::Result<PathBuf> {
    if cfg!(target_os = "windows") {
        std::env::var("APPDATA")
            .map(PathBuf::from)
            .map_err(|_| std::io::Error::new(std::io::ErrorKind::NotFound, "%APPDATA% not set"))
    } else {
        std::env::var("HOME")
            .map(PathBuf::from)
            .map_err(|_| std::io::Error::new(std::io::ErrorKind::NotFound, "$HOME not set"))
    }
}
