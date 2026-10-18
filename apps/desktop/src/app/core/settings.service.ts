// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { Service } from '@angular/core';
import { LazyStore } from '@tauri-apps/plugin-store';
import { ModelRef } from './api.types';

/**
 * Persisted user choices. A null embedding model means vector retrieval is disabled; a null
 * answer model means md-api's default local model. Only the path of the .env file is stored,
 * never API keys.
 */
export interface AppSettings {
  vault: string | null;
  answerModel: ModelRef | null;
  embeddingModel: string | null;
  envFile: string | null;
}

export const EMPTY_SETTINGS: AppSettings = {
  vault: null,
  answerModel: null,
  embeddingModel: null,
  envFile: null,
};

/** Settings written before cloud providers stored the answer model as plain Ollama model name. */
type StoredSettings = Partial<AppSettings> & { model?: string | null };

/** Fills defaults and migrates the former `model` string to an Ollama model reference. */
export function migrateSettings(saved: StoredSettings | undefined): AppSettings {
  const { model, ...rest } = saved ?? {};
  const settings = { ...EMPTY_SETTINGS, ...rest };
  if (!settings.answerModel && model) {
    settings.answerModel = { provider: 'ollama', name: model };
  }
  return settings;
}

const SETTINGS_KEY = 'settings';

@Service()
export class SettingsService {
  private readonly store = new LazyStore('settings.json');

  async load(): Promise<AppSettings> {
    return migrateSettings(await this.store.get<StoredSettings>(SETTINGS_KEY));
  }

  async save(settings: AppSettings): Promise<void> {
    await this.store.set(SETTINGS_KEY, settings);
    await this.store.save();
  }
}
