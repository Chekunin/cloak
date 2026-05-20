//! `#[tauri::command]` handlers exposed to the frontend.
//!
//! Each handler is a thin pass-through: it pulls a `Client` from `AppState`,
//! invokes one typed method on it, and converts errors into the frontend's
//! wire shape. New RPC bindings should follow this pattern exactly.
//!
//! Naming convention: `snake_case` matching the Tauri JS / TS side. Cobra-style
//! groupings (`vault_*`, `secrets_*`, …) keep the surface easy to grep.

use tauri::State;

use crate::client::{
    AuditEntry, CreateSecretRequest, Endpoint, RevealedSecret, Secret, Token, TokenInfo,
    UpdateSecretRequest, VaultStatus,
};
use crate::error::{AppError, Result};
use crate::state::AppState;

// --- daemon / vault ------------------------------------------------------

/// Quick liveness probe. Returns `true` when we can dial the socket
/// *and* (if a token is configured) authenticate.
#[tauri::command]
pub async fn daemon_ping(state: State<'_, AppState>) -> Result<bool> {
    match state.client().await {
        Ok(_) => Ok(true),
        Err(AppError::Unreachable(_)) => Ok(false),
        Err(e) => Err(e),
    }
}

#[tauri::command]
pub async fn vault_status(state: State<'_, AppState>) -> Result<VaultStatus> {
    Ok(state.client().await?.transport().vault_status().await?)
}

#[tauri::command]
pub async fn vault_init(state: State<'_, AppState>, password: String) -> Result<()> {
    if password.is_empty() {
        return Err(AppError::InvalidArgument("password is required".into()));
    }
    state.client().await?.transport().vault_init(&password).await?;
    Ok(())
}

#[tauri::command]
pub async fn vault_unlock(state: State<'_, AppState>, password: String) -> Result<()> {
    if password.is_empty() {
        return Err(AppError::InvalidArgument("password is required".into()));
    }
    state.client().await?.transport().vault_unlock(&password).await?;

    // Auto-bootstrap a GUI token on the first unlock after fresh install.
    // The daemon's `tokens.create` allows unauthenticated calls only when no
    // active tokens exist, so this is naturally a one-shot; subsequent
    // unlocks (or when the user already has tokens) get an `unauthorized`
    // error that we swallow — the connection banner will guide them.
    if !state.has_token().await {
        if let Ok(client) = state.client().await {
            if let Ok(info) = client.transport().create_token("cloak-gui").await {
                tracing::info!(token_id = %info.id, "auto-bootstrapped GUI token");
                state.set_token(Some(info.token)).await;
            } else {
                tracing::debug!("auto-bootstrap skipped — tokens already exist or vault locked");
            }
        }
    }
    Ok(())
}

#[tauri::command]
pub async fn vault_lock(state: State<'_, AppState>) -> Result<()> {
    state.client().await?.transport().vault_lock().await?;
    Ok(())
}

// --- secrets -------------------------------------------------------------

#[tauri::command]
pub async fn secrets_list(state: State<'_, AppState>) -> Result<Vec<Secret>> {
    Ok(state.client().await?.transport().list_secrets().await?)
}

#[tauri::command]
pub async fn secrets_get(state: State<'_, AppState>, id_or_name: String) -> Result<Secret> {
    Ok(state
        .client()
        .await?
        .transport()
        .get_secret(&id_or_name)
        .await?)
}

/// Decrypt and return the secret material for one secret. Requires the vault
/// master password as a re-authentication gate — a configured client token is
/// deliberately not enough. The daemon audit-logs every call.
#[tauri::command]
pub async fn secrets_reveal(
    state: State<'_, AppState>,
    id_or_name: String,
    password: String,
) -> Result<RevealedSecret> {
    if id_or_name.is_empty() {
        return Err(AppError::InvalidArgument("secret is required".into()));
    }
    if password.is_empty() {
        return Err(AppError::InvalidArgument(
            "master password is required".into(),
        ));
    }
    Ok(state
        .client()
        .await?
        .transport()
        .reveal_secret(&id_or_name, &password)
        .await?)
}

#[tauri::command]
pub async fn secrets_create(
    state: State<'_, AppState>,
    request: CreateSecretRequest,
) -> Result<Secret> {
    Ok(state
        .client()
        .await?
        .transport()
        .create_secret(&request)
        .await?)
}

#[tauri::command]
pub async fn secrets_update(
    state: State<'_, AppState>,
    request: UpdateSecretRequest,
) -> Result<Secret> {
    Ok(state
        .client()
        .await?
        .transport()
        .update_secret(&request)
        .await?)
}

#[tauri::command]
pub async fn secrets_delete(state: State<'_, AppState>, id_or_name: String) -> Result<()> {
    state
        .client()
        .await?
        .transport()
        .delete_secret(&id_or_name)
        .await?;
    Ok(())
}

// --- endpoints -----------------------------------------------------------

#[tauri::command]
pub async fn endpoints_list(state: State<'_, AppState>) -> Result<Vec<Endpoint>> {
    Ok(state.client().await?.transport().list_endpoints().await?)
}

#[tauri::command]
pub async fn endpoints_open(
    state: State<'_, AppState>,
    secret: String,
    ttl_seconds: i32,
) -> Result<Endpoint> {
    Ok(state
        .client()
        .await?
        .transport()
        .open_endpoint(&secret, ttl_seconds)
        .await?)
}

#[tauri::command]
pub async fn endpoints_close(state: State<'_, AppState>, endpoint_id: String) -> Result<()> {
    state
        .client()
        .await?
        .transport()
        .close_endpoint(&endpoint_id)
        .await?;
    Ok(())
}

// --- exec ----------------------------------------------------------------

/// Run a command with a secret's endpoint environment variables injected —
/// the GUI analogue of `cloak exec`. Opens an endpoint for `secret`, runs the
/// command through the user's shell with the env vars layered on, and closes
/// the endpoint again, even if the command fails.
///
/// Most useful for `env` secrets, where the injected variables are the real
/// credentials a CLI tool (the AWS CLI, `gcloud`, …) needs. It also works for
/// proxied secrets, injecting their `127.0.0.1` connection variables.
#[tauri::command]
pub async fn secrets_exec(
    state: State<'_, AppState>,
    secret: String,
    command: String,
) -> Result<crate::exec::ExecResult> {
    if secret.is_empty() {
        return Err(AppError::InvalidArgument("secret is required".into()));
    }
    if command.trim().is_empty() {
        return Err(AppError::InvalidArgument("command is required".into()));
    }
    let client = state.client().await?;
    let endpoint = client.transport().open_endpoint(&secret, 0).await?;

    let result = crate::exec::run_in_shell(&command, &endpoint.env_vars).await;

    // Always close the endpoint — success or failure — so a materialized
    // secret's rendered files are shredded promptly.
    if let Err(e) = client.transport().close_endpoint(&endpoint.id).await {
        tracing::warn!(endpoint = %endpoint.id, error = %e, "could not close endpoint after exec");
    }
    result
}

// --- updates -------------------------------------------------------------

/// A newer release offered by the configured update endpoint.
#[derive(serde::Serialize)]
pub struct UpdateInfo {
    /// The version the update would install.
    pub version: String,
    /// The version currently running.
    pub current_version: String,
    /// Release notes, when the update manifest carries them.
    pub notes: Option<String>,
    /// Publish date, when present.
    pub date: Option<String>,
}

/// Check the update endpoint for a newer release. `Ok(None)` means the app is
/// already up to date.
#[tauri::command]
pub async fn check_for_update(app: tauri::AppHandle) -> Result<Option<UpdateInfo>> {
    use tauri_plugin_updater::UpdaterExt;
    let updater = app
        .updater()
        .map_err(|e| AppError::Internal(format!("updates are not configured: {e}")))?;
    match updater.check().await {
        Ok(Some(u)) => Ok(Some(UpdateInfo {
            version: u.version.clone(),
            current_version: u.current_version.clone(),
            notes: u.body.clone(),
            date: u.date.map(|d| d.to_string()),
        })),
        Ok(None) => Ok(None),
        Err(e) => Err(AppError::Internal(format!("update check failed: {e}"))),
    }
}

/// Download and install the available update, then relaunch. On success the
/// process is replaced and this never returns.
#[tauri::command]
pub async fn install_update(app: tauri::AppHandle) -> Result<()> {
    use tauri_plugin_updater::UpdaterExt;
    let updater = app
        .updater()
        .map_err(|e| AppError::Internal(format!("updates are not configured: {e}")))?;
    let update = updater
        .check()
        .await
        .map_err(|e| AppError::Internal(format!("update check failed: {e}")))?
        .ok_or_else(|| AppError::Internal("no update is available".into()))?;
    update
        .download_and_install(|_chunk, _total| {}, || {})
        .await
        .map_err(|e| AppError::Internal(format!("update install failed: {e}")))?;
    app.restart();
}

// --- tokens --------------------------------------------------------------

#[tauri::command]
pub async fn tokens_list(state: State<'_, AppState>) -> Result<Vec<Token>> {
    Ok(state.client().await?.transport().list_tokens().await?)
}

/// Issue a new token. If `persist` is `true`, store it in the `AppState` so
/// subsequent dials authenticate automatically. Plaintext is also returned
/// for the caller to display once.
#[tauri::command]
pub async fn tokens_create(
    state: State<'_, AppState>,
    name: String,
    persist: bool,
) -> Result<TokenInfo> {
    if name.is_empty() {
        return Err(AppError::InvalidArgument("name is required".into()));
    }
    let info = state.client().await?.transport().create_token(&name).await?;
    if persist {
        state.set_token(Some(info.token.clone())).await;
    }
    Ok(info)
}

#[tauri::command]
pub async fn tokens_revoke(state: State<'_, AppState>, id: String) -> Result<()> {
    state.client().await?.transport().revoke_token(&id).await?;
    Ok(())
}

// --- audit ---------------------------------------------------------------

#[tauri::command]
pub async fn audit_tail(state: State<'_, AppState>, limit: i32) -> Result<Vec<AuditEntry>> {
    Ok(state.client().await?.transport().audit_tail(limit).await?)
}

// --- diagnostics ---------------------------------------------------------

/// Information about how this process is wired up. Useful for the
/// connection-status banner in the UI.
#[derive(serde::Serialize)]
pub struct DaemonInfo {
    pub socket_path: String,
    pub has_token: bool,
}

#[tauri::command]
pub async fn daemon_info(state: State<'_, AppState>) -> Result<DaemonInfo> {
    Ok(DaemonInfo {
        socket_path: state.socket_path().await.to_string_lossy().into_owned(),
        has_token: state.has_token().await,
    })
}

// --- token import (GUI-side wiring) --------------------------------------

/// Set the GUI's bearer token to `token` and validate it via `hello`.
///
/// The token is held in `AppState` only — it's not written to disk. The
/// frontend uses this to manually import an existing token when the
/// auto-bootstrap path is closed (because other tokens already exist).
#[tauri::command]
pub async fn gui_set_token(state: State<'_, AppState>, token: String) -> Result<()> {
    let trimmed = token.trim();
    if trimmed.is_empty() {
        return Err(AppError::InvalidArgument("token is required".into()));
    }
    state.set_token(Some(trimmed.to_string())).await;
    // Force a dial+authenticate so a bad token surfaces *now* rather than
    // on the next list call.
    state.client().await?;
    Ok(())
}

/// Re-read `~/.cloak/cli_token` and apply it to `AppState` if present.
///
/// Returns `true` when a token was loaded and successfully validated.
/// Useful when the user ran `cloak token create --save` *after* launching
/// the GUI — the file exists now but `AppState` only checked it at startup.
#[tauri::command]
pub async fn gui_reload_cli_token(state: State<'_, AppState>) -> Result<bool> {
    let path = crate::paths::cli_token_path()?;
    let data = match std::fs::read_to_string(&path) {
        Ok(d) => d,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Ok(false),
        Err(e) => return Err(AppError::Internal(e.to_string())),
    };
    let trimmed = data.trim();
    if trimmed.is_empty() {
        return Ok(false);
    }
    state.set_token(Some(trimmed.to_string())).await;
    state.client().await?;
    Ok(true)
}
