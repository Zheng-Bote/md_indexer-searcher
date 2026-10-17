// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { Component, computed, inject } from '@angular/core';
import { HlmSelectImports } from '@spartan-ng/helm/select';
import { toast } from '@spartan-ng/brain/sonner';
import { WorkspaceService } from './workspace.service';

/** Select value standing for "no embedding model" (vector retrieval disabled). */
const NO_EMBEDDING = '';

@Component({
  selector: 'app-model-selector',
  imports: [HlmSelectImports],
  template: `
    <div class="flex flex-col gap-4">
      <div class="flex flex-col gap-1.5">
        <label class="text-sm font-medium" for="model-select">Sprachmodell</label>
        <hlm-select [value]="model()" (valueChange)="onModel($event)" [disabled]="disabled()">
          <hlm-select-trigger class="w-full" buttonId="model-select">
            <hlm-select-value placeholder="Modell wählen" />
          </hlm-select-trigger>
          <hlm-select-content *hlmSelectPortal class="max-h-80">
            <hlm-select-group>
              @for (m of models(); track m.name) {
                <hlm-select-item [value]="m.name">{{ m.name }}</hlm-select-item>
              }
            </hlm-select-group>
          </hlm-select-content>
        </hlm-select>
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
              @for (m of models(); track m.name) {
                <hlm-select-item [value]="m.name">{{ m.name }}</hlm-select-item>
              }
            </hlm-select-group>
          </hlm-select-content>
        </hlm-select>
        <p class="text-muted-foreground text-xs">
          Ein neues Embedding-Modell berechnet fehlende Embeddings beim nächsten Indexlauf.
        </p>
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

  protected readonly models = this.workspace.models;
  protected readonly model = computed(() => this.workspace.settings().model);
  protected readonly embeddingModel = computed(
    () => this.workspace.settings().embeddingModel ?? NO_EMBEDDING,
  );
  protected readonly disabled = computed(() => {
    const state = this.workspace.status().state;
    return this.models().length === 0 || state === 'indexing' || state === 'starting';
  });

  protected onModel(value: string | null | undefined): void {
    if (value && value !== this.model()) {
      this.apply(this.workspace.selectModel(value));
    }
  }

  protected onEmbeddingModel(value: string | null | undefined): void {
    const next = value ? value : null;
    if (next !== this.workspace.settings().embeddingModel) {
      this.apply(this.workspace.selectEmbeddingModel(next));
    }
  }

  private apply(change: Promise<void>): void {
    change.catch((err: unknown) =>
      toast.error('Modell konnte nicht übernommen werden', { description: String(err) }),
    );
  }
}
