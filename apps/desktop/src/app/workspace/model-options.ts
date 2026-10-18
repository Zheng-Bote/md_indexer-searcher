// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { ModelInfo, ModelRef, PROVIDER_NAMES } from '../core/api.types';

export interface ModelOption {
  key: string;
  label: string;
  ref: ModelRef;
}

export interface ModelGroup {
  label: string;
  cloud: boolean;
  options: ModelOption[];
}

/** Select value of a model reference; provider IDs never contain "/". */
export function modelKey(ref: ModelRef): string {
  return `${ref.provider}/${ref.name}`;
}

export function parseModelKey(key: string): ModelRef {
  const slash = key.indexOf('/');
  return { provider: key.slice(0, slash), name: key.slice(slash + 1) };
}

/** Converts Ollama parameter sizes like "14.8B" or "566.70M" to a number for sorting; unknown sizes sort last. */
export function parameterCount(size: string | undefined): number {
  const match = /^([\d.]+)\s*([KMBT])?$/i.exec(size?.trim() ?? '');
  if (!match) {
    return -1;
  }
  const factor = { K: 1e3, M: 1e6, B: 1e9, T: 1e12 }[(match[2] ?? '').toUpperCase()] ?? 1;
  return parseFloat(match[1]) * factor;
}

function providerOf(m: ModelInfo): string {
  return m.provider ?? 'ollama';
}

/**
 * Answer models: local generation models (largest first, with size label), then one group per cloud
 * provider. Embedding models are never offered; cloud models of the local daemon (":cloud") form
 * their own cloud group.
 */
export function answerModelGroups(models: ModelInfo[]): ModelGroup[] {
  const generation = models.filter((m) => m.kind !== 'embedding');
  const local = generation
    .filter((m) => m.locality !== 'cloud')
    .sort((a, b) => parameterCount(b.parameter_size) - parameterCount(a.parameter_size));
  const groups: ModelGroup[] = [];
  if (local.length > 0) {
    groups.push({
      label: 'Lokal',
      cloud: false,
      options: local.map((m) => {
        const ref = { provider: providerOf(m), name: m.name };
        const label = m.parameter_size ? `${m.name} · ${m.parameter_size}` : m.name;
        return { key: modelKey(ref), label, ref };
      }),
    });
  }
  for (const m of generation.filter((m) => m.locality === 'cloud')) {
    const provider = providerOf(m);
    const label = `Cloud · ${m.provider_name && provider !== 'ollama' ? m.provider_name : providerLabel(provider, m.name)}`;
    let group = groups.find((g) => g.label === label);
    if (!group) {
      group = { label, cloud: true, options: [] };
      groups.push(group);
    }
    const ref = { provider, name: m.name };
    group.options.push({ key: modelKey(ref), label: m.name, ref });
  }
  return groups;
}

/** Embedding models only; models without a kind (older md-api) are kept. */
export function embeddingModels(models: ModelInfo[]): ModelInfo[] {
  return models.filter((m) => m.kind !== 'generation' && m.locality !== 'cloud');
}

/** Display name of a provider; the local daemon's ":cloud" models are served by Ollama's cloud. */
export function providerLabel(provider: string | undefined, model = ''): string {
  if (provider === 'ollama' && /:(.*-)?cloud$/.test(model)) {
    return 'Ollama Cloud';
  }
  return PROVIDER_NAMES[provider ?? ''] ?? provider ?? 'Ollama';
}
