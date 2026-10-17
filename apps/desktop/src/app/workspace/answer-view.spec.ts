// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { TestBed } from '@angular/core/testing';
import { Answer } from '../core/api.types';
import { AnswerView, formatDuration } from './answer-view';

function render(answer: Answer, durationMs: number | null = null): HTMLElement {
  const fixture = TestBed.createComponent(AnswerView);
  fixture.componentRef.setInput('answer', answer);
  fixture.componentRef.setInput('durationMs', durationMs);
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

const base: Answer = {
  question: 'q',
  answer: '',
  intent: 'architecture',
  model: 'qwen3:14b',
  sources: null,
  gaps: null,
};

describe('AnswerView', () => {
  it('renders the Markdown answer, sources and gaps', () => {
    const el = render({
      ...base,
      answer: '## Pipeline\n\n- **Ingestion** liest MQTT',
      sources: [{ path: '/vault/arch/Ingestion Pipeline.md', section: 'Ablauf' }],
      gaps: [{ entity: 'MQTT Broker', reason: 'no ADR linked' }],
    });

    expect(el.querySelector('.answer-markdown h2')?.textContent).toBe('Pipeline');
    expect(el.querySelector('.answer-markdown strong')?.textContent).toBe('Ingestion');
    expect(el.textContent).toContain('Ingestion Pipeline.md');
    expect(el.textContent).toContain('Ablauf');
    expect(el.textContent).toContain('MQTT Broker');
  });

  it('sanitizes HTML coming from the model', () => {
    const el = render({ ...base, answer: 'ok <img src="x" onerror="alert(1)"><script>alert(2)</script>' });

    expect(el.querySelector('script')).toBeNull();
    expect(el.querySelector('img')?.getAttribute('onerror')).toBeNull();
  });

  it('omits empty sections when Go sends null slices', () => {
    const el = render({ ...base, answer: 'Keine Belege.' });

    expect(el.textContent).not.toContain('Quellen');
    expect(el.textContent).not.toContain('Wissenslücken');
  });

  it('shows the response time when known', () => {
    expect(render({ ...base, answer: 'x' }, 190_532).textContent).toContain('Antwortzeit 3:11 min');
    expect(render({ ...base, answer: 'x' }).textContent).not.toContain('Antwortzeit');
  });
});

describe('formatDuration', () => {
  it('formats as m:ss', () => {
    expect(formatDuration(0)).toBe('0:00');
    expect(formatDuration(9_400)).toBe('0:09');
    expect(formatDuration(190_532)).toBe('3:11');
  });
});
