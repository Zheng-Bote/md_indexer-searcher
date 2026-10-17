// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

/** Response envelope used by every md-api endpoint. */
export interface ApiEnvelope<T> {
  success: boolean;
  data?: T;
  error?: string;
}

export interface ModelInfo {
  name: string;
  size: number;
  family?: string;
}

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
  sources: SourceCitation[] | null;
  gaps?: KnowledgeGap[] | null;
}
