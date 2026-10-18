// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { Component, computed, DestroyRef, inject, signal } from '@angular/core';
import { HlmAlertImports } from '@spartan-ng/helm/alert';
import { HlmButtonImports } from '@spartan-ng/helm/button';
import { HlmSpinnerImports } from '@spartan-ng/helm/spinner';
import { HlmTextareaImports } from '@spartan-ng/helm/textarea';
import { ApiClient } from '../core/api-client.service';
import { Answer } from '../core/api.types';
import { AnswerView, formatDuration } from './answer-view';
import { WorkspaceService } from './workspace.service';

@Component({
  selector: 'app-ask-panel',
  imports: [HlmAlertImports, HlmButtonImports, HlmSpinnerImports, HlmTextareaImports, AnswerView],
  template: `
    <form class="flex flex-col gap-2" (submit)="submit($event)">
      <label class="text-sm font-medium" for="question">Frage an den Vault</label>
      <textarea
        hlmTextarea
        id="question"
        rows="3"
        placeholder="z. B. Wie funktioniert die Ingestion Pipeline?"
        [value]="question()"
        [disabled]="busy()"
        (input)="onInput($event)"
        (keydown.control.enter)="submit($event)"
      ></textarea>
      <div class="flex items-center justify-between gap-2">
        <span class="text-muted-foreground text-xs">Strg+Enter zum Senden</span>
        <button hlmBtn type="submit" [disabled]="!canAsk()">
          @if (busy()) {
            <hlm-spinner aria-hidden="true" />
            Antwort wird erzeugt… {{ elapsedLabel() }}
          } @else {
            Fragen
          }
        </button>
      </div>
    </form>

    <div class="mt-6 flex flex-col gap-4" aria-live="polite">
      @if (busy()) {
        <p class="text-muted-foreground text-sm">
          Auf CPU kann die Antwort mehrere Minuten dauern.
        </p>
      }
      @if (error(); as message) {
        <div hlmAlert variant="destructive" role="alert">
          <h3 hlmAlertTitle>Frage fehlgeschlagen</h3>
          <p hlmAlertDescription>{{ message }}</p>
        </div>
      }
      @if (answer(); as result) {
        <app-answer-view [answer]="result" [durationMs]="durationMs()" />
      }
    </div>
  `,
})
export class AskPanel {
  private readonly api = inject(ApiClient);
  private readonly workspace = inject(WorkspaceService);

  protected readonly question = signal('');
  protected readonly busy = signal(false);
  protected readonly answer = signal<Answer | null>(null);
  protected readonly error = signal<string | null>(null);
  /** Time of the last completed request, shown with its answer. */
  protected readonly durationMs = signal<number | null>(null);
  private readonly elapsedMs = signal(0);
  protected readonly elapsedLabel = computed(() => formatDuration(this.elapsedMs()));
  private timer: ReturnType<typeof setInterval> | undefined;
  protected readonly canAsk = computed(
    () => this.workspace.ready() && !this.busy() && this.question().trim().length > 0,
  );

  constructor() {
    inject(DestroyRef).onDestroy(() => clearInterval(this.timer));
  }

  protected onInput(event: Event): void {
    this.question.set((event.target as HTMLTextAreaElement).value);
  }

  protected async submit(event: Event): Promise<void> {
    event.preventDefault();
    if (!this.canAsk()) {
      return;
    }
    this.busy.set(true);
    this.error.set(null);
    this.answer.set(null);
    this.durationMs.set(null);
    const startedAt = performance.now();
    this.elapsedMs.set(0);
    this.timer = setInterval(() => this.elapsedMs.set(performance.now() - startedAt), 1000);
    try {
      const answer = await this.api.ask(this.question().trim());
      this.durationMs.set(performance.now() - startedAt);
      this.answer.set(answer);
    } catch (err) {
      this.error.set(
        `${err instanceof Error ? err.message : String(err)} (nach ${formatDuration(performance.now() - startedAt)} min)`,
      );
    } finally {
      clearInterval(this.timer);
      this.busy.set(false);
    }
  }
}
