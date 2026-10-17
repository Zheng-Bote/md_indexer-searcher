// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { computed, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ApiClient } from '../core/api-client.service';
import { BackendConfig, BackendService, BackendStatus } from '../core/backend.service';
import { AppSettings, EMPTY_SETTINGS, SettingsService } from '../core/settings.service';
import { WorkspaceService } from './workspace.service';

describe('WorkspaceService', () => {
  let status: ReturnType<typeof signal<BackendStatus>>;
  let applied: BackendConfig[];
  let saved: AppSettings;
  let listModels: ReturnType<typeof vi.fn>;

  function setup(initial: AppSettings): WorkspaceService {
    status = signal<BackendStatus>({ state: 'idle', vault: null, apiBaseUrl: null, message: null, logTail: [] });
    applied = [];
    saved = initial;
    listModels = vi.fn().mockResolvedValue({
      models: [{ name: 'qwen3:14b', size: 1 }],
      active: { model: 'qwen3:14b', embedding_model: '' },
    });

    TestBed.configureTestingModule({
      providers: [
        {
          provide: BackendService,
          useValue: {
            status,
            apiBaseUrl: computed(() => (status().state === 'ready' ? status().apiBaseUrl : null)),
            init: vi.fn().mockResolvedValue(undefined),
            applyConfig: vi.fn(async (config: BackendConfig) => {
              applied.push(config);
            }),
          },
        },
        {
          provide: SettingsService,
          useValue: {
            load: vi.fn(async () => saved),
            save: vi.fn(async (next: AppSettings) => {
              saved = next;
            }),
          },
        },
        { provide: ApiClient, useValue: { listModels } },
      ],
    });
    return TestBed.inject(WorkspaceService);
  }

  it('restores the last vault and starts the backend', async () => {
    const workspace = setup({ vault: '/vault', model: 'qwen3:14b', embeddingModel: 'bge-m3' });

    await workspace.init();

    expect(applied).toEqual([{ vault: '/vault', model: 'qwen3:14b', embeddingModel: 'bge-m3' }]);
  });

  it('does not start anything without a saved vault', async () => {
    const workspace = setup(EMPTY_SETTINGS);

    await workspace.init();

    expect(applied).toEqual([]);
  });

  it('persists a model change and restarts the backend', async () => {
    const workspace = setup({ vault: '/vault', model: 'qwen3:14b', embeddingModel: null });
    await workspace.init();

    await workspace.selectModel('llama3:8b');

    expect(saved.model).toBe('llama3:8b');
    expect(applied.at(-1)).toEqual({ vault: '/vault', model: 'llama3:8b', embeddingModel: null });
  });

  it('loads models once ready and adopts the active model on first start', async () => {
    const workspace = setup({ vault: '/vault', model: null, embeddingModel: null });
    await workspace.init();

    status.set({ state: 'ready', vault: '/vault', apiBaseUrl: 'http://127.0.0.1:1', message: null, logTail: [] });
    TestBed.tick();
    await vi.waitFor(() => expect(workspace.models().length).toBe(1));

    expect(saved.model).toBe('qwen3:14b');
    expect(applied.length).toBe(1);
  });
});
