//! In-process port of `cloak exec` — run a command with a secret's endpoint
//! environment variables injected.
//!
//! The endpoint open/close lifecycle lives in `commands::secrets_exec`; this
//! module owns only the process side: spawn the user's shell, layer the
//! injected variables on top of the inherited environment, and capture output.

use std::collections::BTreeMap;
use std::process::Stdio;

use serde::Serialize;
use tokio::process::Command;

use crate::error::{AppError, Result};

/// Cap on captured stdout/stderr. CLI output is small; this only guards
/// against a runaway command ballooning the IPC payload.
const MAX_CAPTURE: usize = 256 * 1024;

/// Outcome of a `secrets_exec` run, returned to the frontend.
#[derive(Debug, Serialize)]
pub struct ExecResult {
    pub stdout: String,
    pub stderr: String,
    pub exit_code: i32,
    /// True when stdout or stderr was clamped to `MAX_CAPTURE`.
    pub truncated: bool,
    /// Names — never values — of the variables injected into the child.
    pub env_var_names: Vec<String>,
}

/// Run `command` through the user's shell with `env` layered onto the current
/// process environment. Mirrors `cloak exec`: the child inherits the GUI's
/// environment plus the injected variables.
pub async fn run_in_shell(command: &str, env: &BTreeMap<String, String>) -> Result<ExecResult> {
    let mut cmd = shell_command(command);
    for (k, v) in env {
        cmd.env(k, v);
    }
    cmd.stdin(Stdio::null());
    cmd.stdout(Stdio::piped());
    cmd.stderr(Stdio::piped());

    let output = cmd
        .output()
        .await
        .map_err(|e| AppError::Internal(format!("could not run command: {e}")))?;

    let (stdout, t1) = clamp(&output.stdout);
    let (stderr, t2) = clamp(&output.stderr);
    Ok(ExecResult {
        stdout,
        stderr,
        exit_code: output.status.code().unwrap_or(-1),
        truncated: t1 || t2,
        env_var_names: env.keys().cloned().collect(),
    })
}

#[cfg(not(windows))]
fn shell_command(command: &str) -> Command {
    let shell = std::env::var("SHELL").unwrap_or_else(|_| "/bin/sh".to_string());
    let mut c = Command::new(shell);
    c.arg("-c").arg(command);
    c
}

#[cfg(windows)]
fn shell_command(command: &str) -> Command {
    let mut c = Command::new("cmd");
    c.arg("/C").arg(command);
    c
}

/// Truncate a captured stream to `MAX_CAPTURE` bytes and lossy-decode it.
fn clamp(raw: &[u8]) -> (String, bool) {
    let truncated = raw.len() > MAX_CAPTURE;
    let slice = if truncated { &raw[..MAX_CAPTURE] } else { raw };
    (String::from_utf8_lossy(slice).into_owned(), truncated)
}
