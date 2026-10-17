// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { Component, computed, inject } from '@angular/core';
import { HlmButtonImports } from '@spartan-ng/helm/button';
import { toast } from '@spartan-ng/brain/sonner';
import { open } from '@tauri-apps/plugin-dialog';
import { WorkspaceService } from './workspace.service';

@Component({
  selector: 'app-vault-picker',
  imports: [HlmButtonImports],
  template: `
    <div class="flex flex-col gap-1.5">
      <span class="text-sm font-medium" id="vault-label">Vault</span>
      <p
        class="truncate rounded-md border px-2.5 py-1.5 text-sm"
        aria-labelledby="vault-label"
        [title]="vault() ?? ''"
      >
        {{ vault() ?? 'Kein Vault ausgewählt' }}
      </p>
      <button hlmBtn variant="outline" type="button" (click)="pick()">Vault auswählen…</button>
    </div>
  `,
})
export class VaultPicker {
  private readonly workspace = inject(WorkspaceService);

  protected readonly vault = computed(() => this.workspace.settings().vault);

  protected async pick(): Promise<void> {
    const selected = await open({ directory: true, multiple: false, title: 'Vault auswählen' });
    if (typeof selected !== 'string') {
      return;
    }
    try {
      await this.workspace.selectVault(selected);
    } catch (err) {
      toast.error('Vault konnte nicht geöffnet werden', { description: String(err) });
    }
  }
}
