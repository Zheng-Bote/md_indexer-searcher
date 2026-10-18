// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

//! Lifecycle management for the `md-indexer` and `md-api` sidecar processes.
//!
//! Every configuration change starts a new "generation". Work belonging to an older
//! generation (readiness polling, process events) is discarded, so rapid vault or
//! model switches never leave stale processes or status updates behind.

use std::collections::VecDeque;
use std::net::TcpListener;
use std::path::{Path, PathBuf};
use std::sync::Mutex;
use std::time::Duration;

use serde::Serialize;
use sha2::{Digest, Sha256};
use tauri::{AppHandle, Emitter, Manager};
use tauri_plugin_shell::ShellExt;
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpStream;

const STATUS_EVENT: &str = "backend-status";
const LOG_TAIL_LINES: usize = 40;
const BIND_HOST: &str = "127.0.0.1";
const POLL_INTERVAL: Duration = Duration::from_millis(500);

#[derive(Clone, Copy, Serialize, PartialEq, Eq, Debug, Default)]
#[serde(rename_all = "lowercase")]
pub enum BackendState {
    #[default]
    Idle,
    Indexing,
    Starting,
    Ready,
    Error,
}

/// Snapshot of the backend sent to the frontend via `backend-status` events.
#[derive(Clone, Serialize, Debug, Default)]
#[serde(rename_all = "camelCase")]
pub struct BackendStatus {
    pub state: BackendState,
    pub vault: Option<String>,
    pub api_base_url: Option<String>,
    pub message: Option<String>,
    pub log_tail: Vec<String>,
}

/// Requested backend configuration. An empty embedding model disables vector retrieval.
/// The answer model is chosen per question (md-api routes it), so changing it needs no restart.
/// `env_file` is the path of md_indexer's .env; md-api reads the API keys from it itself, so keys
/// never pass through the app.
pub struct BackendConfig {
    pub vault: String,
    pub embedding_model: Option<String>,
    pub env_file: Option<String>,
}

#[derive(Default)]
struct Inner {
    generation: u64,
    indexer: Option<CommandChild>,
    api: Option<CommandChild>,
    status: BackendStatus,
    log_tail: VecDeque<String>,
}

#[derive(Default)]
pub struct SidecarManager {
    inner: Mutex<Inner>,
}

impl SidecarManager {
    pub fn status(&self) -> BackendStatus {
        let inner = self.inner.lock().unwrap();
        let mut status = inner.status.clone();
        status.log_tail = inner.log_tail.iter().cloned().collect();
        status
    }

    /// Terminates all sidecars and invalidates any running startup.
    pub fn stop(&self) {
        let mut inner = self.inner.lock().unwrap();
        inner.generation += 1;
        kill_children(&mut inner);
        inner.status = BackendStatus::default();
    }

    /// Validates the configuration and starts the sidecars in the background.
    /// Progress is reported through `backend-status` events.
    pub fn apply(&self, app: &AppHandle, config: BackendConfig) -> Result<(), String> {
        let vault = Path::new(&config.vault);
        if !vault.is_dir() {
            return Err(format!("vault is not a directory: {}", config.vault));
        }
        let legacy_db = legacy_database_path(app, vault)?;

        let generation = {
            let mut inner = self.inner.lock().unwrap();
            inner.generation += 1;
            kill_children(&mut inner);
            inner.log_tail.clear();
            inner.status = BackendStatus {
                state: BackendState::Indexing,
                vault: Some(config.vault.clone()),
                ..BackendStatus::default()
            };
            inner.generation
        };
        emit_status(app, &self.status());

        let app = app.clone();
        tauri::async_runtime::spawn(async move {
            if let Err(err) = start(&app, generation, &config, &legacy_db).await {
                app.state::<SidecarManager>().update(
                    &app,
                    generation,
                    BackendState::Error,
                    None,
                    Some(err),
                );
            }
        });
        Ok(())
    }

    fn is_active(&self, generation: u64) -> bool {
        let inner = self.inner.lock().unwrap();
        inner.generation == generation && inner.status.state != BackendState::Error
    }

    fn update(
        &self,
        app: &AppHandle,
        generation: u64,
        state: BackendState,
        api_base_url: Option<String>,
        message: Option<String>,
    ) {
        {
            let mut inner = self.inner.lock().unwrap();
            if inner.generation != generation {
                return;
            }
            inner.status.state = state;
            inner.status.api_base_url = api_base_url;
            inner.status.message = message;
        }
        emit_status(app, &self.status());
    }

    /// Stores a spawned child unless its generation was superseded meanwhile.
    fn attach(&self, generation: u64, sidecar: Sidecar, child: CommandChild) {
        let mut inner = self.inner.lock().unwrap();
        if inner.generation != generation {
            let _ = child.kill();
            return;
        }
        match sidecar {
            Sidecar::Indexer => inner.indexer = Some(child),
            Sidecar::Api => inner.api = Some(child),
        }
    }

    fn push_log(&self, generation: u64, line: String) {
        let mut inner = self.inner.lock().unwrap();
        if inner.generation != generation {
            return;
        }
        if inner.log_tail.len() == LOG_TAIL_LINES {
            inner.log_tail.pop_front();
        }
        inner.log_tail.push_back(line);
    }
}

#[derive(Clone, Copy)]
enum Sidecar {
    Indexer,
    Api,
}

impl Sidecar {
    fn binary(self) -> &'static str {
        match self {
            Sidecar::Indexer => "md-indexer",
            Sidecar::Api => "md-api",
        }
    }
}

async fn start(
    app: &AppHandle,
    generation: u64,
    config: &BackendConfig,
    legacy_db: &Path,
) -> Result<(), String> {
    let manager = app.state::<SidecarManager>();
    let embedding_model = config.embedding_model.as_deref().filter(|m| !m.is_empty());

    // md-indexer only opens its HTTP port after the initial index is complete,
    // so a healthy /health doubles as the "indexing finished" signal.
    let indexer_port = free_port()?;
    let legacy_db = legacy_db.is_file().then_some(legacy_db);
    let args = indexer_args(&config.vault, legacy_db, indexer_port, embedding_model);
    spawn(app, generation, Sidecar::Indexer, args, Vec::new())?;
    if !wait_healthy(app, generation, indexer_port).await {
        return Ok(());
    }

    manager.update(app, generation, BackendState::Starting, None, None);
    let api_port = free_port()?;
    let args = api_args(&config.vault, api_port, embedding_model);
    let env_file = config.env_file.as_deref().filter(|p| !p.is_empty());
    spawn(app, generation, Sidecar::Api, args, api_env(env_file))?;
    if !wait_healthy(app, generation, api_port).await {
        return Ok(());
    }

    let url = format!("http://{BIND_HOST}:{api_port}");
    manager.update(app, generation, BackendState::Ready, Some(url), None);
    Ok(())
}

/// Arguments for `md-indexer`. The database location is resolved by md-indexer itself
/// (`<vault>/.md_indexer/`, or its user data dir fallback); a legacy app database is moved there once.
fn indexer_args(
    vault: &str,
    legacy_db: Option<&Path>,
    port: u16,
    embedding_model: Option<&str>,
) -> Vec<String> {
    let mut args = vec![
        "-dir".to_string(),
        vault.to_string(),
        "-host".to_string(),
        BIND_HOST.to_string(),
        "-port".to_string(),
        port.to_string(),
    ];
    if let Some(legacy) = legacy_db {
        args.extend([
            "-migrate-from".to_string(),
            legacy.to_string_lossy().into_owned(),
        ]);
    }
    if let Some(embedding) = embedding_model {
        args.extend(["-embed-model".to_string(), embedding.to_string()]);
    }
    args
}

/// Arguments for `md-api`; it opens the index database that md-indexer resolved for the vault.
fn api_args(vault: &str, port: u16, embedding_model: Option<&str>) -> Vec<String> {
    let mut args = vec![
        "-vault".to_string(),
        vault.to_string(),
        "-host".to_string(),
        BIND_HOST.to_string(),
        "-port".to_string(),
        port.to_string(),
        "-ai".to_string(),
    ];
    match embedding_model {
        Some(embedding) => args.extend([
            "-vectors=true".to_string(),
            "-embedding-model".to_string(),
            embedding.to_string(),
        ]),
        None => args.push("-vectors=false".to_string()),
    }
    args
}

/// Environment for `md-api`: only the path of the .env file, never its keys.
fn api_env(env_file: Option<&str>) -> Vec<(String, String)> {
    env_file
        .map(|path| vec![("MD_AI_ENV_FILE".to_string(), path.to_string())])
        .unwrap_or_default()
}

fn spawn(
    app: &AppHandle,
    generation: u64,
    sidecar: Sidecar,
    args: Vec<String>,
    env: Vec<(String, String)>,
) -> Result<(), String> {
    let name = sidecar.binary();
    let (mut events, child) = app
        .shell()
        .sidecar(name)
        .map_err(|e| format!("{name} sidecar not found: {e}"))?
        .args(args)
        .envs(env)
        .spawn()
        .map_err(|e| format!("failed to start {name}: {e}"))?;
    app.state::<SidecarManager>()
        .attach(generation, sidecar, child);

    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        let manager = app.state::<SidecarManager>();
        while let Some(event) = events.recv().await {
            match event {
                CommandEvent::Stdout(line) | CommandEvent::Stderr(line) => {
                    let text = strip_ansi(&String::from_utf8_lossy(&line));
                    let text = text.trim_end();
                    if !text.is_empty() {
                        manager.push_log(generation, format!("[{name}] {text}"));
                    }
                }
                CommandEvent::Terminated(payload) => {
                    if manager.is_active(generation) {
                        let code = payload
                            .code
                            .map_or_else(|| "signal".to_string(), |c| c.to_string());
                        let message = format!("{name} exited unexpectedly ({code})");
                        manager.update(&app, generation, BackendState::Error, None, Some(message));
                    }
                    break;
                }
                _ => {}
            }
        }
    });
    Ok(())
}

/// Polls `/health` until it answers, the sidecar fails, or the generation is superseded.
async fn wait_healthy(app: &AppHandle, generation: u64, port: u16) -> bool {
    let manager = app.state::<SidecarManager>();
    while manager.is_active(generation) {
        if health_ok(port).await {
            return true;
        }
        tokio::time::sleep(POLL_INTERVAL).await;
    }
    false
}

async fn health_ok(port: u16) -> bool {
    let request = async {
        let mut stream = TcpStream::connect((BIND_HOST, port)).await?;
        stream
            .write_all(b"GET /health HTTP/1.0\r\nHost: 127.0.0.1\r\n\r\n")
            .await?;
        let mut buf = [0u8; 32];
        let n = stream.read(&mut buf).await?;
        Ok::<bool, std::io::Error>(
            buf[..n].starts_with(b"HTTP/1.") && buf[..n].windows(5).any(|w| w == b" 200 "),
        )
    };
    matches!(
        tokio::time::timeout(Duration::from_secs(2), request).await,
        Ok(Ok(true))
    )
}

fn kill_children(inner: &mut Inner) {
    for child in [inner.api.take(), inner.indexer.take()]
        .into_iter()
        .flatten()
    {
        let _ = child.kill();
    }
}

fn emit_status(app: &AppHandle, status: &BackendStatus) {
    let _ = app.emit(STATUS_EVENT, status);
}

fn free_port() -> Result<u16, String> {
    TcpListener::bind((BIND_HOST, 0))
        .and_then(|l| l.local_addr())
        .map(|a| a.port())
        .map_err(|e| format!("no free local port: {e}"))
}

/// Each vault gets its own database, keyed by a hash of its absolute path, so switching
/// back to a vault reuses its index and embeddings.
/// Database location used by app versions before the vault-local database (issue #23).
/// Only computed here; md-indexer moves the file into the vault if it exists.
fn legacy_database_path(app: &AppHandle, vault: &Path) -> Result<PathBuf, String> {
    let absolute = vault
        .canonicalize()
        .map_err(|e| format!("cannot resolve vault path: {e}"))?;
    let digest = Sha256::digest(absolute.to_string_lossy().as_bytes());
    let key: String = digest[..8].iter().map(|b| format!("{b:02x}")).collect();

    let dir = app
        .path()
        .app_data_dir()
        .map_err(|e| format!("no app data directory: {e}"))?
        .join("vaults");
    Ok(dir.join(format!("{key}.db")))
}

/// Removes ANSI color sequences emitted by zerolog's console writer.
fn strip_ansi(input: &str) -> String {
    let mut out = String::with_capacity(input.len());
    let mut chars = input.chars();
    while let Some(c) = chars.next() {
        if c == '\u{1b}' {
            for c in chars.by_ref() {
                if c.is_ascii_alphabetic() {
                    break;
                }
            }
        } else {
            out.push(c);
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::{api_args, api_env, indexer_args, strip_ansi};
    use std::path::Path;

    #[test]
    fn indexer_args_pass_vault_and_optional_migration() {
        assert_eq!(
            indexer_args("/v", None, 4711, None),
            ["-dir", "/v", "-host", "127.0.0.1", "-port", "4711"]
        );
        assert_eq!(
            indexer_args(
                "/v",
                Some(Path::new("/data/vaults/ab.db")),
                4711,
                Some("bge-m3")
            ),
            [
                "-dir",
                "/v",
                "-host",
                "127.0.0.1",
                "-port",
                "4711",
                "-migrate-from",
                "/data/vaults/ab.db",
                "-embed-model",
                "bge-m3",
            ]
        );
    }

    #[test]
    fn api_args_use_the_vault_and_no_answer_model() {
        let args = api_args("/v", 4712, None);
        assert_eq!(
            args,
            [
                "-vault",
                "/v",
                "-host",
                "127.0.0.1",
                "-port",
                "4712",
                "-ai",
                "-vectors=false",
            ]
        );
        assert!(!args.iter().any(|a| a == "-db" || a == "-model"));
        assert!(api_args("/v", 4712, Some("bge-m3")).ends_with(&[
            "-vectors=true".to_string(),
            "-embedding-model".to_string(),
            "bge-m3".to_string(),
        ]));
    }

    #[test]
    fn api_env_passes_only_the_env_file_path() {
        assert!(api_env(None).is_empty());
        assert_eq!(
            api_env(Some("/home/u/.config/md-indexer/.env")),
            [(
                "MD_AI_ENV_FILE".to_string(),
                "/home/u/.config/md-indexer/.env".to_string()
            )]
        );
    }

    #[test]
    fn strips_color_sequences() {
        assert_eq!(
            strip_ansi("\u{1b}[32mINF\u{1b}[0m Starting"),
            "INF Starting"
        );
        assert_eq!(strip_ansi("plain"), "plain");
    }
}
