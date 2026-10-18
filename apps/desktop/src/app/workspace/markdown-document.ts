// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { Marked, Tokens } from 'marked';
import { ResolvedLink } from '../core/api.types';

/** Fragment prefix of rendered wikilinks; the rest is the URI-encoded target document path. */
export const DOC_LINK_PREFIX = '#doc/';

const FRONTMATTER = /^---\r?\n[\s\S]*?\r?\n---(?:\r?\n|$)/;

/** Removes a leading YAML frontmatter block. */
export function stripFrontmatter(markdown: string): string {
  return markdown.replace(FRONTMATTER, '');
}

/** Mirrors Go `normalizeLinkTarget`: `folder/Note#Heading|Alias` -> `Note`. */
export function normalizeLinkTarget(raw: string): string {
  let target = raw.split('|')[0].split('#')[0].trim();
  target = target.slice(Math.max(target.lastIndexOf('/'), target.lastIndexOf('\\')) + 1);
  return target.replace(/\.md$/, '').trim();
}

function escapeHtml(text: string): string {
  return text.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`);
}

/**
 * Renders vault Markdown for the document viewer: frontmatter hidden, wikilinks that resolve in the
 * current vault become `#doc/<path>` links, everything else that would navigate away is plain text.
 * The result must still be bound via `[innerHTML]` so Angular sanitizes it.
 */
export function renderDocument(markdown: string, links: ResolvedLink[] | null): string {
  const resolved = new Map((links ?? []).map((l) => [l.target.toLowerCase(), l.path]));

  const marked = new Marked({
    extensions: [
      {
        name: 'embed',
        level: 'inline',
        start: (src) => src.indexOf('![['),
        tokenizer(src) {
          const m = /^!\[\[([^\]\n]+)\]\]/.exec(src);
          return m ? { type: 'embed', raw: m[0] } : undefined;
        },
        renderer: (token) => escapeHtml(token.raw),
      },
      {
        name: 'wikilink',
        level: 'inline',
        start: (src) => src.indexOf('[['),
        tokenizer(src) {
          const m = /^\[\[([^\]\n]+)\]\]/.exec(src);
          return m ? { type: 'wikilink', raw: m[0], inner: m[1] } : undefined;
        },
        renderer(token) {
          const inner = token['inner'] as string;
          const [target, alias] = inner.split('|');
          const label = escapeHtml((alias ?? target).trim());
          const path = resolved.get(normalizeLinkTarget(inner).toLowerCase());
          return path
            ? `<a href="${DOC_LINK_PREFIX}${encodeURIComponent(path)}">${label}</a>`
            : label;
        },
      },
    ],
    renderer: {
      // External links and images are out of scope for v1 and render as text.
      link(token: Tokens.Link) {
        return this.parser.parseInline(token.tokens);
      },
      image(token: Tokens.Image) {
        return escapeHtml(token.text);
      },
    },
  });

  return marked.parse(stripFrontmatter(markdown), { async: false });
}
