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

  const claude = { provider: 'claude', name: 'claude-opus-5-5' };

  it('restores the last vault and starts the backend with embedding model and .env path', async () => {
    const workspace = setup({ vault: '/vault', answerModel: claude, embeddingModel: 'bge-m3', envFile: '/home/u/.env' });

    await workspace.init();

    expect(applied).toEqual([{ vault: '/vault', embeddingModel: 'bge-m3', envFile: '/home/u/.env' }]);
  });

  it('does not start anything without a saved vault', async () => {
    const workspace = setup(EMPTY_SETTINGS);

    await workspace.init();

    expect(applied).toEqual([]);
  });

  it('persists an answer model change without restarting the backend', async () => {
    const workspace = setup({ vault: '/vault', answerModel: null, embeddingModel: null, envFile: null });
    await workspace.init();

    await workspace.selectAnswerModel(claude);

    expect(saved.answerModel).toEqual(claude);
    expect(applied.length).toBe(1);
  });

  it('restarts the backend when the .env file changes', async () => {
    const workspace = setup({ vault: '/vault', answerModel: claude, embeddingModel: null, envFile: null });
    await workspace.init();

    await workspace.selectEnvFile('/secrets/md.env');

    expect(saved.envFile).toBe('/secrets/md.env');
    expect(applied.at(-1)).toEqual({ vault: '/vault', embeddingModel: null, envFile: '/secrets/md.env' });
  });

  it('loads models once ready and adopts the active model as local answer model on first start', async () => {
    const workspace = setup({ vault: '/vault', answerModel: null, embeddingModel: null, envFile: null });
    await workspace.init();

    status.set({ state: 'ready', vault: '/vault', apiBaseUrl: 'http://127.0.0.1:1', message: null, logTail: [] });
    TestBed.tick();
    await vi.waitFor(() => expect(workspace.models().length).toBe(1));

    expect(saved.answerModel).toEqual({ provider: 'ollama', name: 'qwen3:14b' });
    expect(applied.length).toBe(1);
  });

  it('reloads models on demand, resets a missing answer model and clears the notice on selection', async () => {
    const workspace = setup({ vault: '/vault', answerModel: claude, embeddingModel: null, envFile: null });
    await workspace.init();
    status.set({ state: 'ready', vault: '/vault', apiBaseUrl: 'http://127.0.0.1:1', message: null, logTail: [] });
    TestBed.tick();
    await vi.waitFor(() => expect(workspace.models().length).toBe(1));

    expect(saved.answerModel).toEqual({ provider: 'ollama', name: 'qwen3:14b' });
    expect(workspace.answerNotice()).toContain('claude-opus-5-5');

    listModels.mockResolvedValue({
      models: [{ name: 'qwen3:14b', size: 1 }, { name: 'gemma3:12b', size: 1 }],
      active: { model: 'qwen3:14b', embedding_model: '' },
    });
    await workspace.refreshModels();

    expect(workspace.models().length).toBe(2);
    expect(applied.length).toBe(1);

    await workspace.selectAnswerModel({ provider: 'ollama', name: 'gemma3:12b' });
    expect(workspace.answerNotice()).toBeNull();
  });

  it('keeps the previous list and reports the local error when Ollama is down', async () => {
    const workspace = setup({ vault: '/vault', answerModel: null, embeddingModel: null, envFile: null });
    await workspace.init();
    status.set({ state: 'ready', vault: '/vault', apiBaseUrl: 'http://127.0.0.1:1', message: null, logTail: [] });
    TestBed.tick();
    await vi.waitFor(() => expect(workspace.models().length).toBe(1));

    listModels.mockRejectedValueOnce(new Error('offline'));
    await workspace.refreshModels();
    expect(workspace.models().length).toBe(1);
    expect(workspace.modelsError()).toBe('offline');

    listModels.mockResolvedValue({
      models: [{ name: 'claude-opus-5-5', size: 0, provider: 'claude', locality: 'cloud', kind: 'generation' }],
      active: { model: 'qwen3:14b', embedding_model: '' },
      local_error: 'connection refused',
    });
    await workspace.refreshModels();
    expect(workspace.localError()).toBe('connection refused');
    expect(workspace.modelsError()).toBeNull();
    expect(saved.answerModel).toEqual({ provider: 'ollama', name: 'qwen3:14b' });
  });

  it('keeps cloud consent per provider until the app closes', () => {
    const workspace = setup(EMPTY_SETTINGS);

    workspace.grantCloudConsent('Claude');

    expect(workspace.cloudConsent().has('Claude')).toBe(true);
    expect(workspace.cloudConsent().has('Gemini')).toBe(false);
  });
});
