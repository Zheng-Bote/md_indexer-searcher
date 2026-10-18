// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ApiClient, ApiError, HTTP_FETCH } from './api-client.service';
import { BackendService } from './backend.service';

const fetchMock = vi.fn<(input: string, init?: RequestInit) => Promise<Response>>();

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

describe('ApiClient', () => {
  const apiBaseUrl = signal<string | null>('http://127.0.0.1:4711');
  let client: ApiClient;

  beforeEach(() => {
    fetchMock.mockReset();
    apiBaseUrl.set('http://127.0.0.1:4711');
    TestBed.configureTestingModule({
      providers: [
        { provide: BackendService, useValue: { apiBaseUrl } },
        { provide: HTTP_FETCH, useValue: fetchMock },
      ],
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

  it('encodes the document path as query parameter', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(200, { success: true, data: { path: '/vault/A & B.md' } }),
    );

    await client.documentByPath('/vault/A & B.md');

    expect(fetchMock.mock.calls[0][0]).toBe(
      'http://127.0.0.1:4711/api/document?path=%2Fvault%2FA+%26+B.md',
    );
  });

  it('surfaces the md-api error message', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(502, {
        success: false,
        error: 'failed to call ollama at http://localhost:11434/api/tags',
      }),
    );

    await expect(client.listModels()).rejects.toThrow('failed to call ollama');
  });

  it('refuses requests while the backend is not ready', async () => {
    apiBaseUrl.set(null);

    await expect(client.listModels()).rejects.toThrow('nicht bereit');
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('sends the selected model and exposes the provider error kind', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(502, { success: false, error: 'claude: auth (HTTP 401): invalid x-api-key', error_kind: 'auth' }),
    );

    const failure = client.ask('Frage', { provider: 'claude', name: 'claude-opus-5-5' });

    await expect(failure).rejects.toBeInstanceOf(ApiError);
    await expect(failure).rejects.toMatchObject({ kind: 'auth' });
    expect(JSON.parse(fetchMock.mock.calls[0][1]?.body as string)).toEqual({
      question: 'Frage',
      model: { provider: 'claude', name: 'claude-opus-5-5' },
    });
  });
});
