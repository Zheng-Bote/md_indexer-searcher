// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { computed, Service, signal } from '@angular/core';
import { invoke } from '@tauri-apps/api/core';
import { listen } from '@tauri-apps/api/event';

export type BackendState = 'idle' | 'indexing' | 'starting' | 'ready' | 'error';

/** Mirrors the Rust `BackendStatus` emitted as `backend-status` event. */
export interface BackendStatus {
  state: BackendState;
  vault: string | null;
  apiBaseUrl: string | null;
  message: string | null;
  logTail: string[];
}

export interface BackendConfig {
  vault: string;
  embeddingModel: string | null;
  /** Path of md_indexer's .env; md-api reads the cloud API keys from it. */
  envFile: string | null;
}

const IDLE: BackendStatus = { state: 'idle', vault: null, apiBaseUrl: null, message: null, logTail: [] };

/** Bridge to the Rust sidecar manager running md-indexer and md-api. */
@Service()
export class BackendService {
  private readonly _status = signal<BackendStatus>(IDLE);

  readonly status = this._status.asReadonly();
  /** Base URL of md-api, only set while the backend is ready. */
  readonly apiBaseUrl = computed(() => {
    const status = this._status();
    return status.state === 'ready' ? status.apiBaseUrl : null;
  });

  async init(): Promise<void> {
    await listen<BackendStatus>('backend-status', (event) => this._status.set(event.payload));
    this._status.set(await invoke<BackendStatus>('backend_status'));
  }

  /** Restarts the sidecars; progress arrives through status updates. */
  applyConfig(config: BackendConfig): Promise<void> {
    return invoke('apply_config', { ...config });
  }
}
