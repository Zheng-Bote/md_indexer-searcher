// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { signal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ApiClient, ApiError } from '../core/api-client.service';
import { Answer, ModelInfo, ModelRef } from '../core/api.types';
import { EMPTY_SETTINGS } from '../core/settings.service';
import { AskPanel, describeError } from './ask-panel';
import { WorkspaceService } from './workspace.service';

const answer = (text: string): Answer => ({
  question: 'q',
  answer: text,
  intent: 'architecture',
  model: 'qwen3:14b',
  sources: [{ path: '/vault/Old Source.md', section: 'Alt' }],
  gaps: null,
});

describe('AskPanel', () => {
  let ask: ReturnType<typeof vi.fn>;
  let fixture: ComponentFixture<AskPanel>;
  let el: HTMLElement;
  let answerModel: ReturnType<typeof signal<ModelRef | null>>;
  let consent: ReturnType<typeof signal<ReadonlySet<string>>>;
  let answerBlocked: ReturnType<typeof signal<boolean>>;
  let refreshModels: ReturnType<typeof vi.fn>;
  const models = signal<ModelInfo[]>([
    { name: 'qwen3:14b', size: 1, provider: 'ollama', locality: 'local', kind: 'generation' },
    {
      name: 'claude-opus-5-5',
      size: 0,
      provider: 'claude',
      provider_name: 'Claude',
      locality: 'cloud',
      kind: 'generation',
    },
  ]);

  beforeEach(() => {
    ask = vi.fn();
    answerModel = signal<ModelRef | null>(null);
    consent = signal<ReadonlySet<string>>(new Set());
    answerBlocked = signal(false);
    refreshModels = vi.fn().mockResolvedValue(undefined);
    TestBed.configureTestingModule({
      providers: [
        {
          provide: ApiClient,
          useValue: {
            ask,
            documentByPath: vi.fn(async (path: string) => ({
              id: 'o',
              path,
              title: 'Old Source',
              markdown: '# Old Source',
              links: null,
            })),
          },
        },
        {
          provide: WorkspaceService,
          useValue: {
            ready: signal(true),
            answerBlocked,
            refreshModels,
            models,
            settings: () => ({ ...EMPTY_SETTINGS, answerModel: answerModel() }),
            cloudConsent: consent,
            grantCloudConsent: (p: string) => consent.update((c) => new Set(c).add(p)),
          },
        },
      ],
    });
    fixture = TestBed.createComponent(AskPanel);
    el = fixture.nativeElement as HTMLElement;
    fixture.detectChanges();
  });

  async function submit(question: string): Promise<void> {
    const textarea = el.querySelector('textarea')!;
    textarea.value = question;
    textarea.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    el.querySelector('form')!.dispatchEvent(new Event('submit'));
    fixture.detectChanges();
  }

  async function settle(): Promise<void> {
    await fixture.whenStable();
    fixture.detectChanges();
  }

  it('clears the previous answer while the next question is pending', async () => {
    ask.mockResolvedValueOnce(answer('Alte Antwort'));
    await submit('erste Frage');
    await settle();
    expect(el.textContent).toContain('Alte Antwort');

    let resolve!: (a: Answer) => void;
    ask.mockReturnValueOnce(new Promise<Answer>((r) => (resolve = r)));
    await submit('zweite Frage');

    expect(el.textContent).not.toContain('Alte Antwort');
    expect(el.textContent).not.toContain('Old Source.md');
    expect(el.querySelector('textarea')!.value).toBe('zweite Frage');

    resolve(answer('Neue Antwort'));
    await settle();
    expect(el.textContent).toContain('Neue Antwort');
  });

  it('shows only the error when the next question fails', async () => {
    ask.mockResolvedValueOnce(answer('Alte Antwort'));
    await submit('erste Frage');
    await settle();

    ask.mockRejectedValueOnce(new Error('timeout'));
    await submit('zweite Frage');
    await settle();

    expect(el.textContent).toContain('Frage fehlgeschlagen');
    expect(el.textContent).not.toContain('Alte Antwort');
  });

  it('opens a source in the viewer and returns to the unchanged answer', async () => {
    ask.mockResolvedValueOnce(answer('Alte Antwort'));
    await submit('erste Frage');
    await settle();

    const sourceButton = el.querySelector('app-answer-view li button') as HTMLButtonElement;
    sourceButton.focus();
    sourceButton.click();
    await settle();
    expect(el.querySelector('[role="dialog"] article h1')?.textContent).toBe('Old Source');

    (
      Array.from(el.querySelectorAll('[role="dialog"] button')).find(
        (b) => b.textContent?.trim() === 'Schließen',
      ) as HTMLButtonElement
    ).click();
    await settle();

    expect(el.querySelector('[role="dialog"]')).toBeNull();
    expect(el.textContent).toContain('Alte Antwort');
    expect(ask).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(el.querySelector('app-answer-view li button'));
  });

  it('asks for consent before the first question to a cloud model and sends the model along', async () => {
    answerModel.set({ provider: 'claude', name: 'claude-opus-5-5' });
    ask.mockResolvedValue({ ...answer('Cloud-Antwort'), provider: 'claude', locality: 'cloud' });

    await submit('erste Frage');
    expect(ask).not.toHaveBeenCalled();
    expect(el.querySelector('[role="alertdialog"]')?.textContent).toContain('Claude');

    (
      Array.from(el.querySelectorAll('[role="alertdialog"] button')).find(
        (b) => b.textContent?.trim() === 'Senden',
      ) as HTMLButtonElement
    ).click();
    await settle();
    expect(ask).toHaveBeenCalledWith('erste Frage', {
      provider: 'claude',
      name: 'claude-opus-5-5',
    });
    expect(el.textContent).toContain('Cloud · Claude');

    await submit('zweite Frage');
    await settle();
    expect(ask).toHaveBeenCalledTimes(2);
    expect(el.querySelector('[role="alertdialog"]')).toBeNull();
  });

  it('sends local questions without consent', async () => {
    answerModel.set({ provider: 'ollama', name: 'qwen3:14b' });
    ask.mockResolvedValue(answer('lokal'));

    await submit('Frage');
    await settle();

    expect(ask).toHaveBeenCalledWith('Frage', { provider: 'ollama', name: 'qwen3:14b' });
    expect(el.textContent).toContain('Lokal');
    expect(refreshModels).toHaveBeenCalledTimes(1);
  });

  it('does not ask while no answer model is available', async () => {
    answerBlocked.set(true);

    await submit('Frage');
    await settle();

    expect(ask).not.toHaveBeenCalled();
    expect(el.querySelector('button[type="submit"]')!.hasAttribute('disabled')).toBe(true);
    expect(el.textContent).toContain('Kein Antwort-Modell verfügbar');
  });

  it('describes provider errors specifically', () => {
    expect(
      describeError(new ApiError('openrouter: rate_limit (HTTP 429): slow down', 'rate_limit')),
    ).toBe(
      'Rate-Limit erreicht, bitte später erneut versuchen: openrouter: rate_limit (HTTP 429): slow down',
    );
    expect(
      describeError(new ApiError('claude: refusal: request declined (category: cyber)', 'refusal')),
    ).toContain('Anfrage vom Modell abgelehnt');
    expect(describeError(new Error('Backend ist nicht bereit.'))).toBe('Backend ist nicht bereit.');
  });
});
