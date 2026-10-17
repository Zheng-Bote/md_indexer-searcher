// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { computed, effect, inject, Service, signal } from '@angular/core';
import { ApiClient } from '../core/api-client.service';
import { ModelInfo } from '../core/api.types';
import { BackendService } from '../core/backend.service';
import { AppSettings, EMPTY_SETTINGS, SettingsService } from '../core/settings.service';

/**
 * Keeps the selected vault and models, persists them, and restarts the backend whenever
 * they change.
 */
@Service()
export class WorkspaceService {
  private readonly backend = inject(BackendService);
  private readonly settingsStore = inject(SettingsService);
  private readonly api = inject(ApiClient);

  private readonly _settings = signal<AppSettings>(EMPTY_SETTINGS);
  private readonly _models = signal<ModelInfo[]>([]);
  private readonly _modelsError = signal<string | null>(null);

  readonly settings = this._settings.asReadonly();
  /** Last successfully loaded model list; kept while the backend restarts. */
  readonly models = this._models.asReadonly();
  readonly modelsError = this._modelsError.asReadonly();
  readonly status = this.backend.status;
  readonly ready = computed(() => this.status().state === 'ready');

  constructor() {
    effect(() => {
      if (this.backend.apiBaseUrl()) {
        void this.refreshModels();
      }
    });
  }

  /** Restores saved settings and starts the backend unless it is already running. */
  async init(): Promise<void> {
    await this.backend.init();
    const saved = await this.settingsStore.load();
    this._settings.set(saved);
    if (saved.vault && this.status().state === 'idle') {
      await this.backend.applyConfig({ ...saved, vault: saved.vault });
    }
  }

  selectVault(vault: string): Promise<void> {
    return this.update({ vault });
  }

  selectModel(model: string): Promise<void> {
    return this.update({ model });
  }

  selectEmbeddingModel(embeddingModel: string | null): Promise<void> {
    return this.update({ embeddingModel });
  }

  private async update(patch: Partial<AppSettings>): Promise<void> {
    const next = { ...this._settings(), ...patch };
    this._settings.set(next);
    await this.settingsStore.save(next);
    if (next.vault) {
      await this.backend.applyConfig({ ...next, vault: next.vault });
    }
  }

  private async refreshModels(): Promise<void> {
    try {
      const { models, active } = await this.api.listModels();
      this._models.set(models);
      this._modelsError.set(null);
      // On first start md-api runs with its configured default model; adopt it without a restart.
      const current = this._settings();
      if (!current.model && active.model) {
        const next = { ...current, model: active.model };
        this._settings.set(next);
        await this.settingsStore.save(next);
      }
    } catch (err) {
      this._modelsError.set(err instanceof Error ? err.message : String(err));
    }
  }
}
