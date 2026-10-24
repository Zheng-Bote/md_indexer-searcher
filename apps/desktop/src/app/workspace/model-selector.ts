// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { Component, computed, inject } from '@angular/core';
import { HlmButtonImports } from '@spartan-ng/helm/button';
import { HlmSelectImports } from '@spartan-ng/helm/select';
import { toast } from '@spartan-ng/brain/sonner';
import { open } from '@tauri-apps/plugin-dialog';
import { answerModelGroups, embeddingModels, modelKey, parseModelKey } from './model-options';
import { WorkspaceService } from './workspace.service';

/** Select value standing for "no embedding model" (vector retrieval disabled). */
const NO_EMBEDDING = '';

@Component({
  selector: 'app-model-selector',
  imports: [HlmButtonImports, HlmSelectImports],
  template: `
    <div class="flex flex-col gap-4">
      <div class="flex flex-col gap-1.5">
        <label class="text-sm font-medium" for="model-select">Antwort-Modell</label>
        <hlm-select [value]="answerKey()" (valueChange)="onAnswerModel($event)" [disabled]="disabled()">
          <hlm-select-trigger class="w-full" buttonId="model-select">
            <hlm-select-value placeholder="Modell wählen" />
          </hlm-select-trigger>
          <hlm-select-content *hlmSelectPortal class="max-h-80">
            @for (group of answerGroups(); track group.label) {
              <hlm-select-group>
                <hlm-select-label>{{ group.label }}</hlm-select-label>
                @for (option of group.options; track option.key) {
                  <hlm-select-item [value]="option.key">{{ option.label }}</hlm-select-item>
                }
              </hlm-select-group>
            }
          </hlm-select-content>
        </hlm-select>
        @if (cloudSelected()) {
          <p class="text-muted-foreground text-xs">
            Cloud-Modell: Auszüge aus dem Vault werden an den Anbieter gesendet.
          </p>
        }
        @if (workspace.answerNotice(); as notice) {
          <p class="text-destructive text-xs" role="status">{{ notice }}</p>
        }
      </div>

      <div class="flex flex-col gap-1.5">
        <label class="text-sm font-medium" for="embedding-select">Embedding-Modell</label>
        <hlm-select
          [value]="embeddingModel()"
          (valueChange)="onEmbeddingModel($event)"
          [disabled]="disabled()"
        >
          <hlm-select-trigger class="w-full" buttonId="embedding-select">
            <hlm-select-value placeholder="Embedding-Modell wählen" />
          </hlm-select-trigger>
          <hlm-select-content *hlmSelectPortal class="max-h-80">
            <hlm-select-group>
              <hlm-select-item [value]="noEmbedding">Keins (nur Volltext &amp; Graph)</hlm-select-item>
              @for (m of embeddingOptions(); track m.name) {
                <hlm-select-item [value]="m.name">{{ m.name }}</hlm-select-item>
              }
            </hlm-select-group>
          </hlm-select-content>
        </hlm-select>
        <p class="text-muted-foreground text-xs">
          Ein neues Embedding-Modell berechnet fehlende Embeddings beim nächsten Indexlauf.
        </p>
        @if (workspace.embeddingNotice(); as notice) {
          <p class="text-destructive text-xs" role="status">{{ notice }}</p>
        }
      </div>

      <div class="flex flex-col gap-1.5">
        <button
          hlmBtn
          variant="outline"
          size="sm"
          type="button"
          [disabled]="!workspace.ready() || workspace.refreshing()"
          (click)="workspace.refreshModels()"
        >
          {{ workspace.refreshing() ? 'Modelle werden geladen…' : 'Modelle neu laden' }}
        </button>
        @if (workspace.localError(); as error) {
          <p class="text-destructive text-xs" role="status">Ollama nicht erreichbar: {{ error }}</p>
        }
      </div>

      <div class="flex flex-col gap-1.5">
        <span class="text-sm font-medium" id="env-file-label">Cloud-Anbieter (.env)</span>
        <p
          class="truncate rounded-md border px-2.5 py-1.5 text-sm"
          aria-labelledby="env-file-label"
          [title]="envFile() ?? ''"
        >
          {{ envFile() ?? 'Standard (~/.config/md-indexer/.env)' }}
        </p>
        <div class="flex gap-2">
          <button hlmBtn variant="outline" size="sm" type="button" (click)="pickEnvFile()">.env wählen…</button>
          @if (envFile()) {
            <button hlmBtn variant="ghost" size="sm" type="button" (click)="setEnvFile(null)">Standard</button>
          }
        </div>
      </div>

      @if (workspace.modelsError(); as error) {
        <p class="text-destructive text-xs" role="alert">Modelle nicht verfügbar: {{ error }}</p>
      }
    </div>
  `,
})
export class ModelSelector {
  protected readonly workspace = inject(WorkspaceService);
  protected readonly noEmbedding = NO_EMBEDDING;

  protected readonly answerGroups = computed(() => answerModelGroups(this.workspace.models()));
  protected readonly embeddingOptions = computed(() => embeddingModels(this.workspace.models()));
  protected readonly answerKey = computed(() => {
    const ref = this.workspace.settings().answerModel;
    return ref ? modelKey(ref) : null;
  });
  protected readonly cloudSelected = computed(() => {
    const key = this.answerKey();
    return this.answerGroups().some((g) => g.cloud && g.options.some((o) => o.key === key));
  });
  protected readonly embeddingModel = computed(
    () => this.workspace.settings().embeddingModel ?? NO_EMBEDDING,
  );
  protected readonly envFile = computed(() => this.workspace.settings().envFile);
  protected readonly disabled = computed(() => {
    const state = this.workspace.status().state;
    return this.workspace.models().length === 0 || state === 'indexing' || state === 'starting';
  });

  protected onAnswerModel(value: string | null | undefined): void {
    if (value && value !== this.answerKey()) {
      this.apply(this.workspace.selectAnswerModel(parseModelKey(value)));
    }
  }

  protected onEmbeddingModel(value: string | null | undefined): void {
    const next = value ? value : null;
    if (next !== this.workspace.settings().embeddingModel) {
      this.apply(this.workspace.selectEmbeddingModel(next));
    }
  }

  protected async pickEnvFile(): Promise<void> {
    const selected = await open({ directory: false, multiple: false, title: '.env mit API-Keys wählen' });
    if (typeof selected === 'string') {
      this.setEnvFile(selected);
    }
  }

  protected setEnvFile(path: string | null): void {
    this.apply(this.workspace.selectEnvFile(path));
  }

  private apply(change: Promise<void>): void {
    change.catch((err: unknown) =>
      toast.error('Einstellung konnte nicht übernommen werden', { description: String(err) }),
    );
  }
}
