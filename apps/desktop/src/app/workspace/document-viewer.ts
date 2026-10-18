// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import {
  afterNextRender,
  Component,
  computed,
  effect,
  ElementRef,
  inject,
  Injector,
  input,
  linkedSignal,
  output,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { HlmAlertImports } from '@spartan-ng/helm/alert';
import { HlmButtonImports } from '@spartan-ng/helm/button';
import { HlmSpinnerImports } from '@spartan-ng/helm/spinner';
import { ApiClient } from '../core/api-client.service';
import { DocumentView } from '../core/api.types';
import { DOC_LINK_PREFIX, renderDocument } from './markdown-document';

/** A document to show, optionally scrolled to a section heading. */
export interface SourceRef {
  path: string;
  section?: string;
}

@Component({
  selector: 'app-document-viewer',
  imports: [HlmAlertImports, HlmButtonImports, HlmSpinnerImports],
  host: { '(document:keydown.escape)': 'closed.emit()' },
  template: `
    <div class="bg-background/80 fixed inset-0 z-40 backdrop-blur-sm" aria-hidden="true"></div>
    <section
      role="dialog"
      aria-modal="true"
      aria-labelledby="document-title"
      class="bg-background fixed inset-4 z-50 mx-auto flex max-w-4xl flex-col rounded-lg border shadow-lg"
    >
      <header class="flex items-center gap-2 border-b px-4 py-3">
        <button hlmBtn variant="outline" size="sm" type="button" (click)="back()">Zurück</button>
        <h2
          #heading
          id="document-title"
          tabindex="-1"
          class="min-w-0 flex-1 truncate font-semibold"
          [title]="current().path"
        >
          {{ title() }}
        </h2>
        <button hlmBtn variant="ghost" size="sm" type="button" (click)="closed.emit()">
          Schließen
        </button>
      </header>
      <div #content class="min-h-0 flex-1 overflow-y-auto px-6 py-4">
        @if (loading()) {
          <p class="text-muted-foreground flex items-center gap-2 text-sm">
            <hlm-spinner aria-hidden="true" /> Dokument wird geladen…
          </p>
        } @else if (error(); as message) {
          <div hlmAlert variant="destructive" role="alert">
            <h3 hlmAlertTitle>Dokument konnte nicht geladen werden</h3>
            <p hlmAlertDescription>{{ message }}</p>
          </div>
        } @else {
          <!-- Rendered via [innerHTML], so Angular's sanitizer strips scripts and event handlers. -->
          <article
            class="answer-markdown"
            [innerHTML]="html()"
            (click)="onContentClick($event)"
          ></article>
        }
      </div>
    </section>
  `,
})
export class DocumentViewer {
  private readonly api = inject(ApiClient);
  private readonly injector = inject(Injector);

  readonly source = input.required<SourceRef>();
  readonly closed = output<void>();

  /** Documents visited before the current one; "Zurück" walks this stack. */
  private readonly history = signal<SourceRef[]>([]);
  protected readonly current = linkedSignal(() => this.source());
  private readonly doc = signal<DocumentView | null>(null);
  protected readonly loading = signal(false);
  protected readonly error = signal<string | null>(null);
  protected readonly html = computed(() => {
    const doc = this.doc();
    return doc ? renderDocument(doc.markdown, doc.links) : '';
  });
  protected readonly title = computed(
    () => this.doc()?.title ?? this.current().path.split(/[\\/]/).pop() ?? this.current().path,
  );

  private readonly heading = viewChild.required<ElementRef<HTMLElement>>('heading');
  private readonly content = viewChild.required<ElementRef<HTMLElement>>('content');
  private requestId = 0;

  constructor() {
    afterNextRender(() => this.heading().nativeElement.focus());
    effect(() => {
      const ref = this.current();
      untracked(() => void this.load(ref));
    });
  }

  protected back(): void {
    const previous = this.history().at(-1);
    if (!previous) {
      this.closed.emit();
      return;
    }
    this.history.update((h) => h.slice(0, -1));
    this.current.set(previous);
  }

  protected onContentClick(event: MouseEvent): void {
    const href = (event.target as Element).closest('a')?.getAttribute('href');
    if (!href?.startsWith(DOC_LINK_PREFIX)) {
      return;
    }
    event.preventDefault();
    this.history.update((h) => [...h, this.current()]);
    this.current.set({ path: decodeURIComponent(href.slice(DOC_LINK_PREFIX.length)) });
  }

  private async load(ref: SourceRef): Promise<void> {
    const id = ++this.requestId;
    this.loading.set(true);
    this.error.set(null);
    this.doc.set(null);
    try {
      const doc = await this.api.documentByPath(ref.path);
      if (id !== this.requestId) {
        return;
      }
      this.doc.set(doc);
    } catch (err) {
      if (id !== this.requestId) {
        return;
      }
      const message = err instanceof Error ? err.message : String(err);
      this.error.set(message === 'document not found' ? 'Dokument nicht gefunden.' : message);
    } finally {
      if (id === this.requestId) {
        this.loading.set(false);
      }
    }
    afterNextRender(() => this.reveal(ref.section), { injector: this.injector });
  }

  /** Scrolls to the cited section, or to the top for documents opened via wikilink. */
  private reveal(section: string | undefined): void {
    const container = this.content().nativeElement;
    const target = section
      ? Array.from(container.querySelectorAll('h1, h2, h3, h4, h5, h6')).find(
          (h) => h.textContent?.trim() === section.trim(),
        )
      : undefined;
    if (target) {
      target.scrollIntoView?.({ block: 'start' });
    } else {
      container.scrollTop = 0;
    }
  }
}
