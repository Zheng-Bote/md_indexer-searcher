// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { ModelInfo, ModelsResponse } from '../core/api.types';
import { AppSettings, migrateSettings } from '../core/settings.service';
import {
  answerModelGroups,
  checkSelection,
  embeddingModels,
  modelKey,
  parameterCount,
  parseModelKey,
  providerLabel,
  sameModel,
} from './model-options';

const models: ModelInfo[] = [
  {
    name: 'gemma3:12b',
    size: 1,
    provider: 'ollama',
    locality: 'local',
    kind: 'generation',
    parameter_size: '12.2B',
  },
  {
    name: 'bge-m3:latest',
    size: 1,
    provider: 'ollama',
    locality: 'local',
    kind: 'embedding',
    parameter_size: '566.70M',
  },
  {
    name: 'qwen3:32b',
    size: 1,
    provider: 'ollama',
    locality: 'local',
    kind: 'generation',
    parameter_size: '32.8B',
  },
  {
    name: 'gemma4:cloud',
    size: 0,
    provider: 'ollama',
    provider_name: 'Ollama',
    locality: 'cloud',
    kind: 'generation',
  },
  {
    name: 'claude-opus-5-5',
    size: 0,
    provider: 'claude',
    provider_name: 'Claude',
    locality: 'cloud',
    kind: 'generation',
  },
  {
    name: 'deepseek/deepseek-r1:free',
    size: 0,
    provider: 'openrouter',
    provider_name: 'OpenRouter',
    locality: 'cloud',
    kind: 'generation',
  },
];

describe('model options', () => {
  it('lists local generation models by size with label, then cloud groups', () => {
    const groups = answerModelGroups(models);

    expect(groups.map((g) => g.label)).toEqual([
      'Lokal',
      'Cloud · Ollama Cloud',
      'Cloud · Claude',
      'Cloud · OpenRouter',
    ]);
    expect(groups[0].options.map((o) => o.label)).toEqual([
      'qwen3:32b · 32.8B',
      'gemma3:12b · 12.2B',
    ]);
    expect(groups[0].cloud).toBe(false);
    expect(groups.flatMap((g) => g.options).some((o) => o.ref.name === 'bge-m3:latest')).toBe(
      false,
    );
  });

  it('offers only local embedding models for embeddings', () => {
    expect(embeddingModels(models).map((m) => m.name)).toEqual(['bge-m3:latest']);
  });

  it('round-trips model keys with slashes in the model name', () => {
    const ref = { provider: 'openrouter', name: 'deepseek/deepseek-r1:free' };
    expect(parseModelKey(modelKey(ref))).toEqual(ref);
  });

  it('parses parameter sizes and labels providers', () => {
    expect(parameterCount('14.8B')).toBe(14.8e9);
    expect(parameterCount('566.70M')).toBeCloseTo(566.7e6);
    expect(parameterCount(undefined)).toBe(-1);
    expect(providerLabel('ollama', 'gpt-oss:120b-cloud')).toBe('Ollama Cloud');
    expect(providerLabel('nvidia')).toBe('NVIDIA NIM');
  });

  it('migrates the former model string to an Ollama answer model', () => {
    expect(
      migrateSettings({ vault: '/v', model: 'qwen3:14b', embeddingModel: 'bge-m3' } as never),
    ).toEqual({
      vault: '/v',
      answerModel: { provider: 'ollama', name: 'qwen3:14b' },
      embeddingModel: 'bge-m3',
      envFile: null,
    });
    expect(migrateSettings(undefined).answerModel).toBeNull();
  });

  describe('checkSelection', () => {
    const qwen32 = { provider: 'ollama', name: 'qwen3:32b' };
    const claude = { provider: 'claude', name: 'claude-opus-5-5' };
    const settings = (patch: Partial<AppSettings>): AppSettings => ({
      vault: '/v',
      answerModel: null,
      embeddingModel: null,
      envFile: null,
      ...patch,
    });
    const response = (names: string[], patch: Partial<ModelsResponse> = {}): ModelsResponse => ({
      models: models.filter((m) => names.includes(m.name)),
      active: { model: 'gemma3:12b', embedding_model: 'bge-m3' },
      ...patch,
    });

    it('keeps available models without notice', () => {
      const check = checkSelection(
        settings({ answerModel: qwen32, embeddingModel: 'bge-m3' }),
        response(['qwen3:32b', 'gemma3:12b', 'bge-m3:latest']),
      );

      expect(check).toEqual({
        answerModel: qwen32,
        answerNotice: null,
        answerBlocked: false,
        embeddingNotice: null,
      });
    });

    it('adopts the default local model on first start without notice', () => {
      const check = checkSelection(settings({}), response(['gemma3:12b']));

      expect(check.answerModel).toEqual({ provider: 'ollama', name: 'gemma3:12b' });
      expect(check.answerNotice).toBeNull();
    });

    it('resets a missing answer model to the local default with a notice', () => {
      const check = checkSelection(settings({ answerModel: claude }), response(['gemma3:12b']));

      expect(check.answerModel).toEqual({ provider: 'ollama', name: 'gemma3:12b' });
      expect(check.answerNotice).toContain('claude-opus-5-5 ist nicht verfügbar');
      expect(check.answerBlocked).toBe(false);
    });

    it('blocks asking when the default is missing too', () => {
      const check = checkSelection(settings({ answerModel: qwen32 }), response(['claude-opus-5-5']));

      expect(check.answerModel).toEqual(qwen32);
      expect(check.answerBlocked).toBe(true);
      expect(check.answerNotice).toContain('Standardmodell gemma3:12b');
    });

    it('reports a missing embedding model without changing it', () => {
      const check = checkSelection(
        settings({ answerModel: qwen32, embeddingModel: 'nomic-embed-text' }),
        response(['qwen3:32b', 'bge-m3:latest']),
      );

      expect(check.embeddingNotice).toContain('ollama pull nomic-embed-text');
      expect(check.answerModel).toEqual(qwen32);
    });

    it('does not check local models while Ollama is unreachable', () => {
      const check = checkSelection(
        settings({ answerModel: qwen32, embeddingModel: 'bge-m3' }),
        response(['claude-opus-5-5'], { local_error: 'connection refused' }),
      );

      expect(check).toEqual({
        answerModel: qwen32,
        answerNotice: null,
        answerBlocked: false,
        embeddingNotice: null,
      });
    });

    it('still checks cloud models while Ollama is unreachable', () => {
      const check = checkSelection(
        settings({ answerModel: claude }),
        response(['claude-opus-5-5'], { local_error: 'connection refused' }),
      );

      expect(check.answerModel).toEqual(claude);
      expect(check.answerNotice).toBeNull();
    });

    it('never offers an embedding model as answer model', () => {
      const check = checkSelection(
        settings({ answerModel: { provider: 'ollama', name: 'bge-m3' } }),
        response(['gemma3:12b', 'bge-m3:latest']),
      );

      expect(check.answerModel).toEqual({ provider: 'ollama', name: 'gemma3:12b' });
    });
  });

  it('matches Ollama names with an implicit latest tag', () => {
    expect(sameModel('bge-m3', 'bge-m3:latest')).toBe(true);
    expect(sameModel('qwen3:14b', 'qwen3:32b')).toBe(false);
    expect(sameModel('hf.co/org/model', 'hf.co/org/model:latest')).toBe(true);
    expect(sameModel('deepseek/deepseek-r1:free', 'deepseek/deepseek-r1:free')).toBe(true);
  });
});
