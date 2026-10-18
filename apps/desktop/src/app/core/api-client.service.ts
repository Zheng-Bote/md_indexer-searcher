// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { inject, InjectionToken, Service } from '@angular/core';
import { fetch as tauriFetch } from '@tauri-apps/plugin-http';
import {
  Answer,
  ApiEnvelope,
  DocumentView,
  ModelRef,
  ModelsResponse,
  ProviderErrorKind,
} from './api.types';
import { BackendService } from './backend.service';

/** md-api error; `kind` classifies failures of the selected LLM provider. */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly kind?: ProviderErrorKind,
  ) {
    super(message);
  }
}

/** Transport used by ApiClient; tests provide a stub instead of module-mocking the Tauri plugin. */
export const HTTP_FETCH = new InjectionToken<typeof tauriFetch>('HTTP_FETCH', {
  providedIn: 'root',
  factory: () => tauriFetch,
});

/**
 * HTTP client for md-api. Requests go through the Tauri HTTP plugin, which is scoped to
 * 127.0.0.1 and avoids CORS preflights that md-api does not answer.
 */
@Service()
export class ApiClient {
  private readonly backend = inject(BackendService);
  private readonly fetch = inject(HTTP_FETCH);

  listModels(): Promise<ModelsResponse> {
    return this.request<ModelsResponse>('/api/ai/models');
  }

  /** Asks a question; without `model` md-api answers with its default local model. */
  ask(question: string, model?: ModelRef | null): Promise<Answer> {
    return this.request<Answer>('/api/ai/ask', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(model ? { question, model } : { question }),
    });
  }

  /** Loads an indexed document by its exact path. */
  documentByPath(path: string): Promise<DocumentView> {
    return this.request<DocumentView>(`/api/document?${new URLSearchParams({ path })}`);
  }

  private async request<T>(path: string, init?: RequestInit): Promise<T> {
    const baseUrl = this.backend.apiBaseUrl();
    if (!baseUrl) {
      throw new Error('Backend ist nicht bereit.');
    }
    const response = await this.fetch(baseUrl + path, init);
    let body: ApiEnvelope<T>;
    try {
      body = (await response.json()) as ApiEnvelope<T>;
    } catch {
      throw new ApiError(`md-api antwortete mit HTTP ${response.status}.`);
    }
    if (!response.ok || !body.success || body.data === undefined) {
      throw new ApiError(
        body.error ?? `md-api antwortete mit HTTP ${response.status}.`,
        body.error_kind,
      );
    }
    return body.data;
  }
}
