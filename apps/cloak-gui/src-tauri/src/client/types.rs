//! Wire types mirroring `pkg/client/types.go`.
//!
//! These must stay byte-for-byte JSON-compatible with the Go definitions.
//! Drift is caught in CI by `tools/contract-tests/`. When you add a field
//! here, add it on both sides in the same PR.

// BTreeMap, not HashMap: env vars round-trip daemon → Rust → webview, and a
// HashMap would re-serialize in randomised order on every Tauri call, making
// the GUI's env-var list reshuffle each poll. BTreeMap keeps keys sorted.
use std::collections::BTreeMap;

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

// --- enums ---------------------------------------------------------------

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum SecretType {
    Ssh,
    Postgres,
    Mysql,
    Http,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum EndpointMode {
    Persistent,
    Session,
}

// --- endpoint config -----------------------------------------------------

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct EndpointConfig {
    pub mode: Option<EndpointMode>,
    #[serde(default, skip_serializing_if = "is_zero_i32")]
    pub persistent_port: i32,
    #[serde(default, skip_serializing_if = "is_zero_i32")]
    pub session_ttl_seconds: i32,
    #[serde(default)]
    pub require_local_auth: bool,
    #[serde(default, skip_serializing_if = "is_zero_i32")]
    pub max_concurrent_connections: i32,
}

#[allow(clippy::trivially_copy_pass_by_ref)] // serde's `skip_serializing_if` requires `&T`
fn is_zero_i32(n: &i32) -> bool {
    *n == 0
}

// --- secrets -------------------------------------------------------------

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Secret {
    pub id: String,
    pub name: String,
    #[serde(rename = "type")]
    pub kind: SecretType,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub description: String,
    /// Non-secret config as a free-form JSON object (host, port, ...).
    pub config: serde_json::Value,
    pub endpoint_config: EndpointConfig,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
    #[serde(default)]
    pub last_used_at: Option<DateTime<Utc>>,
}

// --- endpoints -----------------------------------------------------------

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EndpointStats {
    pub bytes_in: i64,
    pub bytes_out: i64,
    pub connections_open: i64,
    pub connections_total: i64,
    #[serde(default)]
    pub last_activity: Option<DateTime<Utc>>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Endpoint {
    pub id: String,
    pub secret_id: String,
    pub secret_name: String,
    #[serde(rename = "type")]
    pub kind: SecretType,
    pub mode: EndpointMode,
    pub local_addr: String,
    pub connection_string: String,
    #[serde(default, skip_serializing_if = "BTreeMap::is_empty")]
    pub env_vars: BTreeMap<String, String>,
    pub opened_at: DateTime<Utc>,
    #[serde(default)]
    pub expires_at: Option<DateTime<Utc>>,
    pub stats: EndpointStats,
}

// --- vault status --------------------------------------------------------

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct VaultStatus {
    pub state: String,
    pub idle_timeout_sec: i32,
    #[serde(default)]
    pub expires_at: Option<DateTime<Utc>>,
    pub endpoints_open: i32,
}

// --- tokens --------------------------------------------------------------

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Token {
    pub id: String,
    pub name: String,
    pub created_at: DateTime<Utc>,
    #[serde(default)]
    pub last_seen_at: Option<DateTime<Utc>>,
    pub revoked: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TokenInfo {
    pub id: String,
    pub name: String,
    /// Plaintext token — shown once at creation.
    pub token: String,
}

// --- audit ---------------------------------------------------------------

/// One audit-log entry. The daemon's schema is open enough that we keep this
/// as a free-form JSON object on the client side rather than over-constrain
/// the type.
pub type AuditEntry = serde_json::Map<String, serde_json::Value>;

// --- request bodies ------------------------------------------------------

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CreateSecretRequest {
    pub name: String,
    #[serde(rename = "type")]
    pub kind: SecretType,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub description: String,
    pub config: serde_json::Value,
    pub secret: serde_json::Value,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub endpoint_config: Option<EndpointConfig>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UpdateSecretRequest {
    pub id_or_name: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub config: Option<serde_json::Value>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub secret: Option<serde_json::Value>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub endpoint_config: Option<EndpointConfig>,
}
