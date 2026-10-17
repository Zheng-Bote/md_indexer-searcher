// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { inject, Service } from '@angular/core';
import { fetch } from '@tauri-apps/plugin-http';
import { Answer, ApiEnvelope, ModelsResponse } from './api.types';
import { BackendService } from './backend.service';

/**
 * HTTP client for md-api. Requests go through the Tauri HTTP plugin, which is scoped to
 * 127.0.0.1 and avoids CORS preflights that md-api does not answer.
 */
@Service()
export class ApiClient {
  private readonly backend = inject(BackendService);

  listModels(): Promise<ModelsResponse> {
    return this.request<ModelsResponse>('/api/ai/models');
  }

  ask(question: string): Promise<Answer> {
    return this.request<Answer>('/api/ai/ask', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ question }),
    });
  }

  private async request<T>(path: string, init?: RequestInit): Promise<T> {
    const baseUrl = this.backend.apiBaseUrl();
    if (!baseUrl) {
      throw new Error('Backend ist nicht bereit.');
    }
    const response = await fetch(baseUrl + path, init);
    let body: ApiEnvelope<T>;
    try {
      body = (await response.json()) as ApiEnvelope<T>;
    } catch {
      throw new Error(`md-api antwortete mit HTTP ${response.status}.`);
    }
    if (!response.ok || !body.success || body.data === undefined) {
      throw new Error(body.error ?? `md-api antwortete mit HTTP ${response.status}.`);
    }
    return body.data;
  }
}
