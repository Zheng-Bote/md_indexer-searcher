// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { computed, effect, inject, Service, signal, untracked } from '@angular/core';
import { ApiClient } from '../core/api-client.service';
import { ModelInfo, ModelRef } from '../core/api.types';
import { BackendService } from '../core/backend.service';
import { AppSettings, EMPTY_SETTINGS, SettingsService } from '../core/settings.service';
import { checkSelection } from './model-options';

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
  private readonly _localError = signal<string | null>(null);
  private readonly _refreshing = signal(false);
  private readonly _answerNotice = signal<string | null>(null);
  private readonly _answerBlocked = signal(false);
  private readonly _embeddingNotice = signal<string | null>(null);
  /** Cloud providers the user agreed to send vault excerpts to; kept until the app closes. */
  private readonly _cloudConsent = signal<ReadonlySet<string>>(new Set());

  readonly settings = this._settings.asReadonly();
  /** Last successfully loaded model list; kept while the backend restarts. */
  readonly models = this._models.asReadonly();
  readonly modelsError = this._modelsError.asReadonly();
  /** Why the local Ollama daemon could not list its models; cloud models stay available. */
  readonly localError = this._localError.asReadonly();
  readonly refreshing = this._refreshing.asReadonly();
  /** Persistent notice about a missing answer model; cleared when the user selects one. */
  readonly answerNotice = this._answerNotice.asReadonly();
  /** No answer model is available; asking is disabled until the user selects one. */
  readonly answerBlocked = this._answerBlocked.asReadonly();
  /** Persistent notice about a missing embedding model; the setting itself is never changed. */
  readonly embeddingNotice = this._embeddingNotice.asReadonly();
  readonly cloudConsent = this._cloudConsent.asReadonly();
  readonly status = this.backend.status;
  readonly ready = computed(() => this.status().state === 'ready');

  constructor() {
    effect(() => {
      if (this.backend.apiBaseUrl()) {
        // Only the backend URL triggers a reload, not the signals refreshModels() reads.
        untracked(() => void this.refreshModels());
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
    this._answerNotice.set(null);
    this._answerBlocked.set(false);
    await this.settingsStore.save(next);
  }

  selectEmbeddingModel(embeddingModel: string | null): Promise<void> {
    this._embeddingNotice.set(null);
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

  /**
   * Reloads the model list without restarting the backend and checks the saved models against it.
   * The previous list stays visible while loading and when loading fails.
   */
  async refreshModels(): Promise<void> {
    if (this._refreshing()) {
      return;
    }
    this._refreshing.set(true);
    try {
      const response = await this.api.listModels();
      this._models.set(response.models);
      this._modelsError.set(null);
      this._localError.set(response.local_error ?? null);
      const current = this._settings();
      const check = checkSelection(current, response);
      this._answerNotice.set(check.answerNotice);
      this._answerBlocked.set(check.answerBlocked);
      this._embeddingNotice.set(check.embeddingNotice);
      if (check.answerModel !== current.answerModel) {
        const next: AppSettings = { ...current, answerModel: check.answerModel };
        this._settings.set(next);
        await this.settingsStore.save(next);
      }
    } catch (err) {
      this._modelsError.set(err instanceof Error ? err.message : String(err));
    } finally {
      this._refreshing.set(false);
    }
  }
}
