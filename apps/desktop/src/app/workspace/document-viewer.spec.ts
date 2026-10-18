// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ApiClient } from '../core/api-client.service';
import { DocumentView } from '../core/api.types';
import { DocumentViewer, SourceRef } from './document-viewer';

const docs: Record<string, DocumentView> = {
  '/vault/A.md': {
    id: 'a',
    path: '/vault/A.md',
    title: 'A',
    markdown:
      '---\ntags: [x]\n---\n# A\n\nGo to [[B|Note B]].\n\n## Setup\n\nText <img src="x" onerror="alert(1)"><script>alert(2)</script>',
    links: [{ target: 'B', path: '/vault/B.md' }],
  },
  '/vault/B.md': {
    id: 'b',
    path: '/vault/B.md',
    title: 'B',
    markdown: '# B\n\nGo to [[C]].',
    links: [{ target: 'C', path: '/vault/C.md' }],
  },
  '/vault/C.md': { id: 'c', path: '/vault/C.md', title: 'C', markdown: '# C', links: null },
};

describe('DocumentViewer', () => {
  let fixture: ComponentFixture<DocumentViewer>;
  let el: HTMLElement;
  let closed: number;
  let scrolled: string[];

  async function open(source: SourceRef): Promise<void> {
    TestBed.configureTestingModule({
      providers: [
        {
          provide: ApiClient,
          useValue: {
            documentByPath: vi.fn(async (path: string) => {
              const doc = docs[path];
              if (!doc) throw new Error('document not found');
              return doc;
            }),
          },
        },
      ],
    });
    scrolled = [];
    Element.prototype.scrollIntoView = function (this: Element) {
      scrolled.push(this.textContent ?? '');
    };
    fixture = TestBed.createComponent(DocumentViewer);
    fixture.componentRef.setInput('source', source);
    closed = 0;
    fixture.componentInstance.closed.subscribe(() => closed++);
    el = fixture.nativeElement as HTMLElement;
    await settle();
  }

  async function settle(): Promise<void> {
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
  }

  const title = () => el.querySelector('#document-title')?.textContent?.trim();
  const button = (label: string) =>
    Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.trim() === label)!;

  it('renders the document sanitized, without frontmatter, and scrolls to the cited section', async () => {
    await open({ path: '/vault/A.md', section: 'Setup' });

    expect(title()).toBe('A');
    expect(el.querySelector('article h1')?.textContent).toBe('A');
    expect(el.textContent).not.toContain('tags:');
    expect(el.querySelector('script')).toBeNull();
    expect(el.querySelector('img')?.getAttribute('onerror')).toBeNull();
    expect(scrolled).toEqual(['Setup']);
    expect(document.activeElement?.id).toBe('document-title');
  });

  it('follows wikilinks and walks the history back to the answer', async () => {
    await open({ path: '/vault/A.md' });

    (el.querySelector('article a') as HTMLElement).click();
    await settle();
    expect(title()).toBe('B');

    (el.querySelector('article a') as HTMLElement).click();
    await settle();
    expect(title()).toBe('C');

    button('Zurück').click();
    await settle();
    expect(title()).toBe('B');

    button('Zurück').click();
    await settle();
    expect(title()).toBe('A');
    expect(closed).toBe(0);

    button('Zurück').click();
    expect(closed).toBe(1);
  });

  it('closes directly via the close button and Escape', async () => {
    await open({ path: '/vault/A.md' });
    (el.querySelector('article a') as HTMLElement).click();
    await settle();

    button('Schließen').click();
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));

    expect(closed).toBe(2);
  });

  it('shows a load error and keeps navigation usable', async () => {
    await open({ path: '/vault/Deleted.md' });

    expect(el.querySelector('[role="alert"]')?.textContent).toContain('Dokument nicht gefunden.');
    button('Schließen').click();
    expect(closed).toBe(1);
  });
});
