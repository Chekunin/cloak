//! Errors surfaced by the JSON-RPC client.
//!
//! `ClientError` distinguishes three kinds of failure so callers (and the
//! Tauri command layer) can react appropriately:
//!
//! - `Transport` — the connection itself broke (daemon down, EOF, malformed
//!   frame). Recoverable by reconnecting.
//! - `Rpc` — the daemon returned an `error` object. The `code` field carries
//!   the stable string identifier from `internal/errs` (`vault_locked`,
//!   `unauthorized`, …); the optional `hint` is human-readable guidance.
//! - `Encode` / `Decode` — local serde failures. These indicate a bug; the
//!   surface should be small.

use serde::{Deserialize, Serialize};
use thiserror::Error;

/// Result alias used throughout the client module.
pub type Result<T> = std::result::Result<T, ClientError>;

#[derive(Debug, Error)]
pub enum ClientError {
    #[error("transport: {0}")]
    Transport(#[from] std::io::Error),

    #[error("encode: {0}")]
    Encode(serde_json::Error),

    #[error("decode: {0}")]
    Decode(serde_json::Error),

    #[error("rpc: {0}")]
    Rpc(RpcError),

    #[error("daemon closed the connection")]
    Closed,
}

/// Mirrors `internal/ipc.RPCError`. The application code lives in `message`
/// (e.g. `"vault_locked"`); `data.hint` is a human-readable string the UI
/// can show.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RpcError {
    pub code: i32,
    pub message: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub data: Option<RpcErrorData>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct RpcErrorData {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub hint: Option<String>,
}

impl std::fmt::Display for RpcError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self.data.as_ref().and_then(|d| d.hint.as_deref()) {
            Some(hint) => write!(f, "{}: {}", self.message, hint),
            None => write!(f, "{}", self.message),
        }
    }
}

impl RpcError {
    /// Stable application code (`vault_locked`, `unauthorized`, …) for
    /// branching in the UI / Rust command layer.
    pub fn app_code(&self) -> &str {
        &self.message
    }
}
