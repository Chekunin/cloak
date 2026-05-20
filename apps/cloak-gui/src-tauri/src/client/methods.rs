//! Typed JSON-RPC method bindings.
//!
//! One function per daemon RPC, returning a strongly-typed result. Adding a
//! new method is a one-line change here plus its parameter type. The
//! transport layer below stays untouched.
//!
//! Method names match `internal/ipc/methods.go.RegisterAll` exactly. If you
//! rename one there, the contract test will catch it before this drifts.
//!
//! Some methods (e.g. `refresh_endpoint`) aren't yet exposed via a
//! `#[tauri::command]` handler. The crate-level `dead_code` allow keeps them
//! callable from future commands without scattering per-item annotations.

#![allow(dead_code)]

use serde_json::{json, Value};

use super::error::Result;
use super::transport::Transport;
use super::types::{
    AuditEntry, CreateSecretRequest, Endpoint, Secret, Token, TokenInfo, UpdateSecretRequest,
    VaultStatus,
};

/// Convenience trait so we can write `transport.list_secrets()` rather than
/// `methods::list_secrets(&transport)`.
impl Transport {
    // --- session -----------------------------------------------------

    pub async fn hello(&self, client_token: &str) -> Result<Value> {
        self.call("hello", &json!({ "client_token": client_token }))
            .await
    }

    // --- vault -------------------------------------------------------

    pub async fn vault_init(&self, password: &str) -> Result<()> {
        self.call("vault.init", &json!({ "password": password }))
            .await
            .map(|_| ())
    }

    pub async fn vault_unlock(&self, password: &str) -> Result<()> {
        self.call(
            "vault.unlock",
            &json!({ "password": password, "unlock_method": "password" }),
        )
        .await
        .map(|_| ())
    }

    pub async fn vault_lock(&self) -> Result<()> {
        self.call("vault.lock", &json!({})).await.map(|_| ())
    }

    pub async fn vault_status(&self) -> Result<VaultStatus> {
        let v = self.call("vault.status", &json!({})).await?;
        decode(v)
    }

    // --- secrets -----------------------------------------------------

    pub async fn list_secrets(&self) -> Result<Vec<Secret>> {
        #[derive(serde::Deserialize)]
        struct Wrap {
            // `Option<Vec<_>>` tolerates both a missing key and an explicit
            // `null` from the daemon (older builds returned `null` for empty
            // lists). `unwrap_or_default()` collapses both to `vec![]`.
            secrets: Option<Vec<Secret>>,
        }
        let v = self.call("secrets.list", &json!({})).await?;
        let wrap: Wrap = decode(v)?;
        Ok(wrap.secrets.unwrap_or_default())
    }

    pub async fn get_secret(&self, id_or_name: &str) -> Result<Secret> {
        let v = self
            .call("secrets.get", &json!({ "id_or_name": id_or_name }))
            .await?;
        decode(v)
    }

    pub async fn create_secret(&self, req: &CreateSecretRequest) -> Result<Secret> {
        let v = self.call("secrets.create", req).await?;
        decode(v)
    }

    pub async fn update_secret(&self, req: &UpdateSecretRequest) -> Result<Secret> {
        let v = self.call("secrets.update", req).await?;
        decode(v)
    }

    pub async fn delete_secret(&self, id_or_name: &str) -> Result<()> {
        self.call("secrets.delete", &json!({ "id_or_name": id_or_name }))
            .await
            .map(|_| ())
    }

    // --- endpoints ---------------------------------------------------

    pub async fn list_endpoints(&self) -> Result<Vec<Endpoint>> {
        #[derive(serde::Deserialize)]
        struct Wrap {
            endpoints: Option<Vec<Endpoint>>,
        }
        let v = self.call("endpoints.list", &json!({})).await?;
        let wrap: Wrap = decode(v)?;
        Ok(wrap.endpoints.unwrap_or_default())
    }

    pub async fn open_endpoint(&self, secret: &str, ttl_seconds: i32) -> Result<Endpoint> {
        let v = self
            .call(
                "endpoints.open",
                &json!({ "secret_id": secret, "ttl_seconds": ttl_seconds }),
            )
            .await?;
        decode(v)
    }

    pub async fn close_endpoint(&self, endpoint_id: &str) -> Result<()> {
        self.call(
            "endpoints.close",
            &json!({ "endpoint_id": endpoint_id }),
        )
        .await
        .map(|_| ())
    }

    pub async fn refresh_endpoint(&self, endpoint_id: &str, ttl_seconds: i32) -> Result<Endpoint> {
        let v = self
            .call(
                "endpoints.refresh",
                &json!({ "endpoint_id": endpoint_id, "ttl_seconds": ttl_seconds }),
            )
            .await?;
        decode(v)
    }

    // --- tokens ------------------------------------------------------

    pub async fn create_token(&self, name: &str) -> Result<TokenInfo> {
        let v = self.call("tokens.create", &json!({ "name": name })).await?;
        decode(v)
    }

    pub async fn list_tokens(&self) -> Result<Vec<Token>> {
        #[derive(serde::Deserialize)]
        struct Wrap {
            tokens: Option<Vec<Token>>,
        }
        let v = self.call("tokens.list", &json!({})).await?;
        let wrap: Wrap = decode(v)?;
        Ok(wrap.tokens.unwrap_or_default())
    }

    pub async fn revoke_token(&self, id: &str) -> Result<()> {
        self.call("tokens.revoke", &json!({ "id": id }))
            .await
            .map(|_| ())
    }

    // --- audit -------------------------------------------------------

    pub async fn audit_tail(&self, limit: i32) -> Result<Vec<AuditEntry>> {
        #[derive(serde::Deserialize)]
        struct Wrap {
            entries: Option<Vec<AuditEntry>>,
        }
        let v = self.call("audit.tail", &json!({ "limit": limit })).await?;
        let wrap: Wrap = decode(v)?;
        Ok(wrap.entries.unwrap_or_default())
    }
}

/// Centralised decode that funnels `serde_json` errors into `ClientError::Decode`.
fn decode<T: serde::de::DeserializeOwned>(v: Value) -> Result<T> {
    serde_json::from_value(v).map_err(super::error::ClientError::Decode)
}
