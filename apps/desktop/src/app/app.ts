// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { Component, DOCUMENT, inject, OnInit } from '@angular/core';
import { toast } from '@spartan-ng/brain/sonner';
import { HlmToasterImports } from '@spartan-ng/helm/sonner';
import { AskPanel } from './workspace/ask-panel';
import { BackendBadge, BackendNotice } from './workspace/backend-status';
import { ModelSelector } from './workspace/model-selector';
import { VaultPicker } from './workspace/vault-picker';
import { WorkspaceService } from './workspace/workspace.service';

@Component({
  selector: 'app-root',
  imports: [AskPanel, BackendBadge, BackendNotice, HlmToasterImports, ModelSelector, VaultPicker],
  template: `
    <div class="flex h-dvh flex-col">
      <header class="flex items-center justify-between border-b px-4 py-3">
        <h1 class="text-base font-semibold">md-indexer</h1>
        <app-backend-badge />
      </header>
      <div class="flex min-h-0 flex-1">
        <aside class="flex w-72 shrink-0 flex-col gap-6 overflow-y-auto border-r p-4" aria-label="Einstellungen">
          <app-vault-picker />
          <app-model-selector />
        </aside>
        <main class="flex min-w-0 flex-1 flex-col gap-4 overflow-y-auto p-4">
          <app-backend-notice />
          <app-ask-panel />
        </main>
      </div>
    </div>
    <hlm-toaster />
  `,
})
export class App implements OnInit {
  private readonly workspace = inject(WorkspaceService);

  constructor() {
    // Follow the OS color scheme; Spartan's dark tokens are keyed on the .dark class.
    const root = inject(DOCUMENT).documentElement;
    const media = matchMedia('(prefers-color-scheme: dark)');
    const apply = () => root.classList.toggle('dark', media.matches);
    apply();
    media.addEventListener('change', apply);
  }

  ngOnInit(): void {
    this.workspace
      .init()
      .catch((err: unknown) => toast.error('Start fehlgeschlagen', { description: String(err) }));
  }
}
