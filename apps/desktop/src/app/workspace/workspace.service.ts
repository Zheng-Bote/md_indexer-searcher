// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { computed, effect, inject, Service, signal } from '@angular/core';
import { ApiClient } from '../core/api-client.service';
import { ModelInfo, ModelRef } from '../core/api.types';
import { BackendService } from '../core/backend.service';
import { AppSettings, EMPTY_SETTINGS, SettingsService } from '../core/settings.service';

/**
 * Keeps the selected vault and models, persists them, and restarts the backend when the vault,
 * the embedding model or the .env file change. The answer model is sent with every question,
 * so changing it needs no restart.
 */
@Service()
export class WorkspaceService {
  private readonly backend = inject(BackendService);
  private readonly settingsStore = inject(SettingsService);
  private readonly api = inject(ApiClient);

  private readonly _settings = signal<AppSettings>(EMPTY_SETTINGS);
  private readonly _models = signal<ModelInfo[]>([]);
  private readonly _modelsError = signal<string | null>(null);
  /** Cloud providers the user agreed to send vault excerpts to; kept until the app closes. */
  private readonly _cloudConsent = signal<ReadonlySet<string>>(new Set());

  readonly settings = this._settings.asReadonly();
  /** Last successfully loaded model list; kept while the backend restarts. */
  readonly models = this._models.asReadonly();
  readonly modelsError = this._modelsError.asReadonly();
  readonly cloudConsent = this._cloudConsent.asReadonly();
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
      await this.backend.applyConfig(this.backendConfig(saved, saved.vault));
    }
  }

  selectVault(vault: string): Promise<void> {
    return this.update({ vault });
  }

  /** Persists the answer model; no restart, it is sent with every question. */
  async selectAnswerModel(answerModel: ModelRef): Promise<void> {
    const next = { ...this._settings(), answerModel };
    this._settings.set(next);
    await this.settingsStore.save(next);
  }

  selectEmbeddingModel(embeddingModel: string | null): Promise<void> {
    return this.update({ embeddingModel });
  }

  selectEnvFile(envFile: string | null): Promise<void> {
    return this.update({ envFile });
  }

  /** Records that vault excerpts may be sent to `provider` until the app closes. */
  grantCloudConsent(provider: string): void {
    this._cloudConsent.update((granted) => new Set(granted).add(provider));
  }

  private async update(patch: Partial<AppSettings>): Promise<void> {
    const next = { ...this._settings(), ...patch };
    this._settings.set(next);
    await this.settingsStore.save(next);
    if (next.vault) {
      await this.backend.applyConfig(this.backendConfig(next, next.vault));
    }
  }

  private backendConfig(settings: AppSettings, vault: string) {
    return { vault, embeddingModel: settings.embeddingModel, envFile: settings.envFile };
  }

  private async refreshModels(): Promise<void> {
    try {
      const { models, active } = await this.api.listModels();
      this._models.set(models);
      this._modelsError.set(null);
      // On first start adopt md-api's default local model as answer model.
      const current = this._settings();
      if (!current.answerModel && active.model) {
        const next: AppSettings = {
          ...current,
          answerModel: { provider: 'ollama', name: active.model },
        };
        this._settings.set(next);
        await this.settingsStore.save(next);
      }
    } catch (err) {
      this._modelsError.set(err instanceof Error ? err.message : String(err));
    }
  }
}
