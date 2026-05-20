//! Low-level newline-delimited JSON-RPC 2.0 framing over a Unix socket.
//!
//! Mirrors the wire contract in `internal/ipc/server.go`:
//!
//! - one JSON object per line (`\n`-terminated)
//! - request ↔ response correlated by `id`
//! - newline framing only; no length prefix
//!
//! Cloak's IPC is local-only, single-connection, sequential, so this
//! transport is intentionally minimal: one mutex, one outstanding call at
//! a time per `Transport`. Higher-level concurrency lives on the
//! `Client` layer.

use std::path::Path;

use serde::Serialize;
use serde_json::{json, Value};
use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};
use tokio::net::UnixStream;
use tokio::sync::Mutex;

use super::error::{ClientError, Result, RpcError};

/// One framed connection to `cloakd`. Safe to share across tasks via `Arc`;
/// the internal mutex serialises in-flight calls.
pub struct Transport {
    inner: Mutex<Inner>,
    next_id: std::sync::atomic::AtomicI64,
}

struct Inner {
    reader: BufReader<tokio::net::unix::OwnedReadHalf>,
    writer: tokio::net::unix::OwnedWriteHalf,
}

impl Transport {
    /// Open a fresh connection to the daemon socket at `path`.
    pub async fn connect(path: impl AsRef<Path>) -> Result<Self> {
        let stream = UnixStream::connect(path.as_ref()).await?;
        let (r, w) = stream.into_split();
        Ok(Self {
            inner: Mutex::new(Inner {
                reader: BufReader::with_capacity(64 * 1024, r),
                writer: w,
            }),
            next_id: std::sync::atomic::AtomicI64::new(0),
        })
    }

    /// Issue one JSON-RPC call.
    ///
    /// On `Ok`, returns the `result` field as raw JSON; the caller is
    /// responsible for `serde_json::from_value` into a concrete type. We
    /// stop one step short of generic deserialisation here so that
    /// transport errors are kept distinct from decode errors at the
    /// caller's site.
    pub async fn call<P: Serialize>(&self, method: &str, params: &P) -> Result<Value> {
        let id = self
            .next_id
            .fetch_add(1, std::sync::atomic::Ordering::Relaxed)
            + 1;

        let req = json!({
            "jsonrpc": "2.0",
            "id":      id,
            "method":  method,
            "params":  params,
        });

        // Encode separately so a JSON error surfaces as `Encode` rather than
        // `Transport` after we've already touched the socket.
        let mut bytes = serde_json::to_vec(&req).map_err(ClientError::Encode)?;
        bytes.push(b'\n');

        let mut inner = self.inner.lock().await;

        inner.writer.write_all(&bytes).await?;
        inner.writer.flush().await?;

        // Read exactly one frame.
        let mut line = Vec::with_capacity(1024);
        let n = inner.reader.read_until(b'\n', &mut line).await?;
        if n == 0 {
            return Err(ClientError::Closed);
        }
        // Trim the trailing newline (if present).
        if line.last() == Some(&b'\n') {
            line.pop();
        }

        let response: WireResponse =
            serde_json::from_slice(&line).map_err(ClientError::Decode)?;

        if let Some(err) = response.error {
            return Err(ClientError::Rpc(err));
        }
        Ok(response.result.unwrap_or(Value::Null))
    }
}

#[derive(serde::Deserialize)]
struct WireResponse {
    #[allow(dead_code)]
    jsonrpc: Option<String>,
    #[allow(dead_code)]
    id: Option<Value>,
    result: Option<Value>,
    error: Option<RpcError>,
}
