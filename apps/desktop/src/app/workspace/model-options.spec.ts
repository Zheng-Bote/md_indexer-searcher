// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { ModelInfo } from '../core/api.types';
import { migrateSettings } from '../core/settings.service';
import {
  answerModelGroups,
  embeddingModels,
  modelKey,
  parameterCount,
  parseModelKey,
  providerLabel,
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
});
