// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { Service } from '@angular/core';
import { LazyStore } from '@tauri-apps/plugin-store';

/** Persisted user choices. A null embedding model means vector retrieval is disabled. */
export interface AppSettings {
  vault: string | null;
  model: string | null;
  embeddingModel: string | null;
}

export const EMPTY_SETTINGS: AppSettings = { vault: null, model: null, embeddingModel: null };

const SETTINGS_KEY = 'settings';

@Service()
export class SettingsService {
  private readonly store = new LazyStore('settings.json');

  async load(): Promise<AppSettings> {
    const saved = await this.store.get<Partial<AppSettings>>(SETTINGS_KEY);
    return { ...EMPTY_SETTINGS, ...saved };
  }

  async save(settings: AppSettings): Promise<void> {
    await this.store.set(SETTINGS_KEY, settings);
    await this.store.save();
  }
}
