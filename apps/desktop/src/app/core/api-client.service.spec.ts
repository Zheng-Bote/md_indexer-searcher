// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { fetch } from '@tauri-apps/plugin-http';
import { ApiClient } from './api-client.service';
import { BackendService } from './backend.service';

vi.mock('@tauri-apps/plugin-http', () => ({ fetch: vi.fn() }));

const fetchMock = vi.mocked(fetch);

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

describe('ApiClient', () => {
  const apiBaseUrl = signal<string | null>('http://127.0.0.1:4711');
  let client: ApiClient;

  beforeEach(() => {
    fetchMock.mockReset();
    apiBaseUrl.set('http://127.0.0.1:4711');
    TestBed.configureTestingModule({
      providers: [{ provide: BackendService, useValue: { apiBaseUrl } }],
    });
    client = TestBed.inject(ApiClient);
  });

  it('posts the question and unwraps the envelope', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(200, { success: true, data: { answer: 'Antwort', sources: null } }),
    );

    const answer = await client.ask('Was ist MQTT?');

    expect(answer.answer).toBe('Antwort');
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('http://127.0.0.1:4711/api/ai/ask');
    expect(init?.method).toBe('POST');
    expect(JSON.parse(init?.body as string)).toEqual({ question: 'Was ist MQTT?' });
  });

  it('surfaces the md-api error message', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(502, { success: false, error: 'failed to call ollama at http://localhost:11434/api/tags' }),
    );

    await expect(client.listModels()).rejects.toThrow('failed to call ollama');
  });

  it('refuses requests while the backend is not ready', async () => {
    apiBaseUrl.set(null);

    await expect(client.listModels()).rejects.toThrow('nicht bereit');
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
