//! Errors surfaced over the Tauri command boundary.
//!
//! The shape is deliberately stable: anything the frontend branches on goes
//! into `code`; anything for the user to read goes into `message` (plus an
//! optional `hint`). This matches the daemon's IPC error model exactly so a
//! Postgres error flowing through us reaches the UI with the same shape it
//! would have over JSON-RPC.

use serde::Serialize;
use thiserror::Error;

use crate::client::{ClientError, RpcError};

/// Result alias used by `#[tauri::command]` handlers.
pub type Result<T> = std::result::Result<T, AppError>;

/// Error type exposed to the frontend via Tauri's serialise-on-error path.
///
/// Variants intentionally stay coarse: code branching belongs to the frontend
/// via the stable `code` string, not via a rich enum.
#[derive(Debug, Error)]
pub enum AppError {
    /// Daemon returned a structured RPC error (`vault_locked`, `unauthorized`, …).
    #[error("{0}")]
    Rpc(RpcError),

    /// Cannot reach the daemon.
    #[error("daemon unreachable: {0}")]
    Unreachable(String),

    /// Misuse — invalid command argument that the frontend should have caught.
    #[error("invalid_argument: {0}")]
    InvalidArgument(String),

    /// Any other unexpected failure (encoding, decoding, internal bug).
    #[error("{0}")]
    Internal(String),
}

impl From<ClientError> for AppError {
    fn from(e: ClientError) -> Self {
        match e {
            ClientError::Rpc(r) => Self::Rpc(r),
            ClientError::Transport(io) => Self::Unreachable(io.to_string()),
            ClientError::Closed => Self::Unreachable("connection closed".into()),
            ClientError::Encode(err) | ClientError::Decode(err) => Self::Internal(err.to_string()),
        }
    }
}

impl From<std::io::Error> for AppError {
    fn from(e: std::io::Error) -> Self {
        Self::Unreachable(e.to_string())
    }
}

/// JSON envelope sent to the frontend on every command error.
#[derive(Debug, Serialize)]
pub struct AppErrorView<'a> {
    /// Stable, machine-readable identifier. The frontend branches on this.
    code: &'a str,
    /// Human-readable message. Safe to render verbatim.
    message: String,
    /// Optional clarifying hint (e.g. "Run `cloak unlock` first.").
    #[serde(skip_serializing_if = "Option::is_none")]
    hint: Option<String>,
}

impl Serialize for AppError {
    fn serialize<S: serde::Serializer>(&self, s: S) -> std::result::Result<S::Ok, S::Error> {
        match self {
            Self::Rpc(r) => AppErrorView {
                code: r.app_code(),
                message: r.message.clone(),
                hint: r.data.as_ref().and_then(|d| d.hint.clone()),
            }
            .serialize(s),
            Self::Unreachable(msg) => AppErrorView {
                code: "daemon_unreachable",
                message: msg.clone(),
                hint: Some("Is `cloak daemon start` running?".into()),
            }
            .serialize(s),
            Self::InvalidArgument(msg) => AppErrorView {
                code: "invalid_argument",
                message: msg.clone(),
                hint: None,
            }
            .serialize(s),
            Self::Internal(msg) => AppErrorView {
                code: "internal_error",
                message: msg.clone(),
                hint: None,
            }
            .serialize(s),
        }
    }
}
