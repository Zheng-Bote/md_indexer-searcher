// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { signal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ApiClient } from '../core/api-client.service';
import { Answer } from '../core/api.types';
import { AskPanel } from './ask-panel';
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

  beforeEach(() => {
    ask = vi.fn();
    TestBed.configureTestingModule({
      providers: [
        { provide: ApiClient, useValue: { ask } },
        { provide: WorkspaceService, useValue: { ready: signal(true) } },
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
});
