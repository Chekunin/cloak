//! Rust port of `pkg/client` — a small, typed JSON-RPC 2.0 client for
//! `cloakd` over its Unix domain socket.
//!
//! Layout mirrors the Go side:
//! - [`transport`] — newline-delimited JSON framing
//! - [`types`]     — wire types (must stay in lockstep with `pkg/client`)
//! - [`methods`]   — one async fn per RPC method
//! - [`error`]     — typed errors with a stable application code
//!
//! The public surface is just [`Client`], a thin wrapper around an `Arc<Transport>`
//! plus an authentication helper. Construct with [`Client::connect`].

mod error;
mod methods;
mod transport;
mod types;

use std::path::{Path, PathBuf};
use std::sync::Arc;

pub use error::{ClientError, Result, RpcError};
// Re-export only what callers in this crate use today; growing this list is a
// one-line change when a new command needs another type.
pub use types::{
    AuditEntry, CreateSecretRequest, Endpoint, Secret, Token, TokenInfo, UpdateSecretRequest,
    VaultStatus,
};

use transport::Transport;

/// A connected, optionally authenticated JSON-RPC client.
///
/// Cheap to clone — internally an `Arc` — so commands can hold their own
/// handle without locking concerns.
#[derive(Clone)]
pub struct Client {
    transport: Arc<Transport>,
    #[allow(dead_code)] // exposed via `socket_path()` — used by Phase 1 diagnostics UI
    socket_path: PathBuf,
}

impl Client {
    /// Dial the daemon at `socket_path`. Does not authenticate.
    pub async fn connect(socket_path: impl AsRef<Path>) -> Result<Self> {
        let path = socket_path.as_ref().to_path_buf();
        let transport = Transport::connect(&path).await?;
        Ok(Self {
            transport: Arc::new(transport),
            socket_path: path,
        })
    }

    /// Authenticate via the `hello` RPC. The connection's session state is
    /// remembered server-side, so subsequent calls inherit the identity.
    pub async fn authenticate(&self, client_token: &str) -> Result<()> {
        self.transport.hello(client_token).await.map(|_| ())
    }

    /// Path of the Unix socket this client is connected to. Useful for
    /// diagnostics and the UI's status banner.
    #[allow(dead_code)] // used by Phase 1 diagnostics UI
    pub fn socket_path(&self) -> &Path {
        &self.socket_path
    }

    /// Direct access to the typed RPC surface. Every `pub async fn` on
    /// `Transport` (e.g. `vault_status`, `list_secrets`) is reachable
    /// via this borrow.
    pub fn transport(&self) -> &Transport {
        &self.transport
    }
}
