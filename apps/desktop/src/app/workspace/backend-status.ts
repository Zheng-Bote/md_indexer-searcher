// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { Component, computed, inject } from '@angular/core';
import { HlmAlertImports } from '@spartan-ng/helm/alert';
import { HlmBadgeImports } from '@spartan-ng/helm/badge';
import { HlmSpinnerImports } from '@spartan-ng/helm/spinner';
import { BackendState } from '../core/backend.service';
import { WorkspaceService } from './workspace.service';

const LABELS: Record<BackendState, string> = {
  idle: 'Kein Vault',
  indexing: 'Indexiere Vault…',
  starting: 'Starte KI-Backend…',
  ready: 'Bereit',
  error: 'Fehler',
};

/** Status badge for the header. */
@Component({
  selector: 'app-backend-badge',
  imports: [HlmBadgeImports, HlmSpinnerImports],
  template: `
    <span
      hlmBadge
      role="status"
      [variant]="state() === 'error' ? 'destructive' : state() === 'ready' ? 'default' : 'secondary'"
    >
      @if (state() === 'indexing' || state() === 'starting') {
        <hlm-spinner class="size-3" aria-hidden="true" />
      }
      {{ label() }}
    </span>
  `,
})
export class BackendBadge {
  private readonly workspace = inject(WorkspaceService);

  protected readonly state = computed(() => this.workspace.status().state);
  protected readonly label = computed(() => LABELS[this.state()]);
}

/** Explains the current backend state when questions are not possible yet. */
@Component({
  selector: 'app-backend-notice',
  imports: [HlmAlertImports],
  template: `
    @switch (status().state) {
      @case ('idle') {
        <div hlmAlert>
          <h2 hlmAlertTitle>Kein Vault ausgewählt</h2>
          <p hlmAlertDescription>Wähle links einen Vault-Ordner aus, um Fragen zu stellen.</p>
        </div>
      }
      @case ('indexing') {
        <div hlmAlert>
          <h2 hlmAlertTitle>Vault wird indexiert</h2>
          <p hlmAlertDescription>
            Beim ersten Mal kann das mit Embeddings auf CPU mehrere Minuten dauern. Danach sind nur
            geänderte Notizen betroffen.
          </p>
        </div>
      }
      @case ('error') {
        <div hlmAlert variant="destructive" role="alert">
          <h2 hlmAlertTitle>Backend gestoppt</h2>
          <div hlmAlertDescription>
            <p>{{ status().message }}</p>
            @if (status().logTail.length > 0) {
              <details class="mt-2">
                <summary class="cursor-pointer">Protokoll</summary>
                <pre class="mt-1 max-h-48 overflow-auto text-xs whitespace-pre-wrap">{{
                  status().logTail.join('\n')
                }}</pre>
              </details>
            }
          </div>
        </div>
      }
    }
  `,
})
export class BackendNotice {
  protected readonly status = inject(WorkspaceService).status;
}
