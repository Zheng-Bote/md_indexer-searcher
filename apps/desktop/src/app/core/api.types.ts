// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

/** Response envelope used by every md-api endpoint. */
export interface ApiEnvelope<T> {
  success: boolean;
  data?: T;
  error?: string;
  /** Set on provider failures of /api/ai/ask (HTTP 502). */
  error_kind?: ProviderErrorKind;
}

export type ProviderErrorKind =
  'auth' | 'rate_limit' | 'refusal' | 'truncated' | 'timeout' | 'unavailable';

export type Locality = 'local' | 'cloud';

export interface ModelInfo {
  name: string;
  size: number;
  family?: string;
  provider?: string;
  provider_name?: string;
  locality?: Locality;
  kind?: 'generation' | 'embedding';
  parameter_size?: string;
}

/** Selects the model that answers a question. */
export interface ModelRef {
  provider: string;
  name: string;
}

/** Display names of the provider IDs used by md-api. */
export const PROVIDER_NAMES: Record<string, string> = {
  ollama: 'Ollama',
  'ollama-cloud': 'Ollama Cloud',
  openrouter: 'OpenRouter',
  nvidia: 'NVIDIA NIM',
  claude: 'Claude',
  gemini: 'Gemini',
};

/** Payload of GET /api/ai/models. An empty embedding_model means vectors are disabled. */
export interface ModelsResponse {
  models: ModelInfo[];
  active: {
    model: string;
    embedding_model: string;
  };
}

export interface SourceCitation {
  path: string;
  section?: string;
  snippet?: string;
}

export interface KnowledgeGap {
  entity: string;
  reason: string;
}

/** Payload of POST /api/ai/ask. Go encodes empty slices as null. */
export interface Answer {
  question: string;
  answer: string;
  intent: string;
  model: string;
  provider?: string;
  locality?: Locality;
  sources: SourceCitation[] | null;
  gaps?: KnowledgeGap[] | null;
}

/** Outgoing wikilink target that resolves to a document of the current vault. */
export interface ResolvedLink {
  target: string;
  path: string;
}

/** Payload of GET /api/document. */
export interface DocumentView {
  id: string;
  path: string;
  title: string;
  markdown: string;
  links: ResolvedLink[] | null;
}
