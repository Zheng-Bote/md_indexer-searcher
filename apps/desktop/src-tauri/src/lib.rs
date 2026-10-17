// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

mod sidecar;

use sidecar::{BackendConfig, BackendStatus, SidecarManager};
use tauri::{AppHandle, Manager, RunEvent, State};

/// Restarts the sidecars for the given vault and models. Returns once the startup is
/// scheduled; readiness is reported through `backend-status` events.
#[tauri::command]
fn apply_config(
    app: AppHandle,
    manager: State<'_, SidecarManager>,
    vault: String,
    model: Option<String>,
    embedding_model: Option<String>,
) -> Result<(), String> {
    manager.apply(
        &app,
        BackendConfig {
            vault,
            model,
            embedding_model,
        },
    )
}

#[tauri::command]
fn backend_status(manager: State<'_, SidecarManager>) -> BackendStatus {
    manager.status()
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_store::Builder::default().build())
        .plugin(tauri_plugin_http::init())
        .manage(SidecarManager::default())
        .invoke_handler(tauri::generate_handler![apply_config, backend_status])
        .build(tauri::generate_context!())
        .expect("error while building tauri application")
        .run(|app, event| {
            if let RunEvent::Exit = event {
                app.state::<SidecarManager>().stop();
            }
        });
}
