// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { Component, computed, input } from '@angular/core';
import { HlmBadgeImports } from '@spartan-ng/helm/badge';
import { HlmCardImports } from '@spartan-ng/helm/card';
import { marked } from 'marked';
import { Answer } from '../core/api.types';

@Component({
  selector: 'app-answer-view',
  imports: [HlmCardImports, HlmBadgeImports],
  template: `
    <section hlmCard aria-labelledby="answer-title">
      <div hlmCardHeader>
        <h2 hlmCardTitle id="answer-title">Antwort</h2>
        <p hlmCardDescription>
          {{ answer().model }} · {{ answer().intent }}
          @if (durationMs() !== null) {
            · Antwortzeit {{ duration() }} min
          }
        </p>
      </div>
      <div hlmCardContent class="flex flex-col gap-5">
        <!-- Rendered via [innerHTML], so Angular's sanitizer strips scripts and event handlers. -->
        <div class="answer-markdown" [innerHTML]="html()"></div>

        @if (sources().length > 0) {
          <div>
            <h3 class="mb-1.5 text-sm font-medium">Quellen</h3>
            <ul class="flex flex-col gap-1 text-sm">
              @for (source of sources(); track $index) {
                <li>
                  <span class="font-mono" [title]="source.path">{{ fileName(source.path) }}</span>
                  @if (source.section) {
                    <span class="text-muted-foreground"> · {{ source.section }}</span>
                  }
                </li>
              }
            </ul>
          </div>
        }

        @if (gaps().length > 0) {
          <div>
            <h3 class="mb-1.5 text-sm font-medium">Wissenslücken</h3>
            <ul class="flex flex-col gap-1.5 text-sm">
              @for (gap of gaps(); track $index) {
                <li class="flex flex-wrap items-center gap-2">
                  <span hlmBadge variant="outline">{{ gap.entity }}</span>
                  <span>{{ gap.reason }}</span>
                </li>
              }
            </ul>
          </div>
        }
      </div>
    </section>
  `,
})
export class AnswerView {
  readonly answer = input.required<Answer>();
  /** Wall-clock time from sending the question to receiving the answer. */
  readonly durationMs = input<number | null>(null);

  protected readonly html = computed(() => marked.parse(this.answer().answer, { async: false }));
  protected readonly sources = computed(() => this.answer().sources ?? []);
  protected readonly gaps = computed(() => this.answer().gaps ?? []);
  protected readonly duration = computed(() => formatDuration(this.durationMs() ?? 0));

  protected fileName(path: string): string {
    return path.split(/[\\/]/).pop() ?? path;
  }
}

/** Formats milliseconds as `m:ss`. */
export function formatDuration(ms: number): string {
  const totalSeconds = Math.round(ms / 1000);
  const seconds = totalSeconds % 60;
  return `${Math.floor(totalSeconds / 60)}:${seconds.toString().padStart(2, '0')}`;
}
