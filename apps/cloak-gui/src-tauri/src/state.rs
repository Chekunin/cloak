//! Tauri-managed application state.
//!
//! Holds the one daemon connection plus the bearer token. Lazy-connects on
//! first use, auto-reconnects if the connection drops, and serialises
//! reconnects so two concurrent commands don't both try to dial.

use std::path::PathBuf;
use std::sync::Arc;

use tokio::sync::Mutex;

use crate::client::Client;
use crate::error::{AppError, Result};
use crate::paths;

/// Top-level managed state. Cheap to clone (just an `Arc` under the hood).
#[derive(Clone)]
pub struct AppState(Arc<Mutex<Inner>>);

struct Inner {
    socket_path: PathBuf,
    token: Option<String>,
    client: Option<Client>,
}

impl AppState {
    /// Construct from environment + filesystem. Does not dial — that
    /// happens on the first command.
    pub fn new() -> Result<Self> {
        let socket_path = paths::socket_path()?;
        let token = load_token_best_effort();
        Ok(Self(Arc::new(Mutex::new(Inner {
            socket_path,
            token,
            client: None,
        }))))
    }

    /// Get a usable, authenticated client.
    ///
    /// Reconnects + re-authenticates transparently after a daemon restart.
    /// Callers should be tolerant of `daemon_unreachable` errors.
    pub async fn client(&self) -> Result<Client> {
        let mut guard = self.0.lock().await;

        // Fast path: existing client (we'll detect a broken pipe on the
        // next call and reconnect then; cheaper than probing here).
        if let Some(c) = guard.client.clone() {
            return Ok(c);
        }

        // Slow path: connect, optionally authenticate.
        let client = Client::connect(&guard.socket_path)
            .await
            .map_err(AppError::from)?;

        if let Some(token) = guard.token.clone() {
            // Authentication failure shouldn't propagate as "unreachable" —
            // it's an actionable RPC error.
            if let Err(e) = client.authenticate(&token).await {
                return Err(AppError::from(e));
            }
        }

        guard.client = Some(client.clone());
        Ok(client)
    }

    /// Drop the cached client so the next `client()` call dials afresh.
    /// Used after we observe a transport error.
    #[allow(dead_code)]
    pub async fn invalidate(&self) {
        let mut guard = self.0.lock().await;
        guard.client = None;
    }

    /// Replace the in-memory bearer token. Used when the GUI bootstraps
    /// its own token via `tokens.create`.
    pub async fn set_token(&self, token: Option<String>) {
        let mut guard = self.0.lock().await;
        guard.token = token;
        guard.client = None; // force re-authentication
    }

    /// Returns the resolved socket path. Used by the connection banner.
    pub async fn socket_path(&self) -> PathBuf {
        self.0.lock().await.socket_path.clone()
    }

    /// Does this process have *any* bearer token wired up?
    pub async fn has_token(&self) -> bool {
        self.0.lock().await.token.is_some()
    }
}

/// Read `$CLOAK_TOKEN` first, then fall back to `~/.cloak/cli_token` if
/// present. Failures are silent — the GUI handles the no-token case by
/// guiding the user through bootstrap.
fn load_token_best_effort() -> Option<String> {
    if let Ok(v) = std::env::var("CLOAK_TOKEN") {
        let trimmed = v.trim();
        if !trimmed.is_empty() {
            return Some(trimmed.to_string());
        }
    }
    let path = paths::cli_token_path().ok()?;
    let data = std::fs::read_to_string(path).ok()?;
    let trimmed = data.trim();
    if trimmed.is_empty() {
        None
    } else {
        Some(trimmed.to_string())
    }
}
