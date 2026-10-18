// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

import { normalizeLinkTarget, renderDocument, stripFrontmatter } from './markdown-document';

const links = [{ target: 'MQTT Broker', path: '/vault/MQTT/MQTT Broker.md' }];

function render(markdown: string): HTMLElement {
  const el = document.createElement('div');
  el.innerHTML = renderDocument(markdown, links);
  return el;
}

describe('markdown-document', () => {
  it('strips a leading frontmatter block only', () => {
    expect(stripFrontmatter('---\ntags: [a]\n---\n# Title\n\n---\n')).toBe('# Title\n\n---\n');
    expect(stripFrontmatter('# No frontmatter')).toBe('# No frontmatter');
  });

  it('normalizes wikilink targets like the Go parser', () => {
    expect(normalizeLinkTarget('folder/MQTT Broker.md#Setup|Broker')).toBe('MQTT Broker');
  });

  it('renders resolved wikilinks with alias and heading as document links', () => {
    const el = render('See [[mqtt broker#Setup|the broker]] and [[MQTT Broker]].');
    const anchors = el.querySelectorAll('a');

    expect(anchors.length).toBe(2);
    expect(anchors[0].textContent).toBe('the broker');
    expect(anchors[0].getAttribute('href')).toBe(
      '#doc/' + encodeURIComponent('/vault/MQTT/MQTT Broker.md'),
    );
    expect(anchors[1].textContent).toBe('MQTT Broker');
  });

  it('renders unresolved wikilinks, embeds, external links and images as text', () => {
    const el = render(
      '[[Kubernetes]] ![[diagram.png]] [site](https://example.com) ![logo](logo.png)',
    );

    expect(el.querySelector('a')).toBeNull();
    expect(el.querySelector('img')).toBeNull();
    expect(el.textContent).toContain('Kubernetes');
    expect(el.textContent).toContain('![[diagram.png]]');
    expect(el.textContent).toContain('site');
    expect(el.textContent).toContain('logo');
  });

  it('leaves wikilinks in code untouched', () => {
    const el = render('`[[MQTT Broker]]`\n\n```\n[[MQTT Broker]]\n```');

    expect(el.querySelector('a')).toBeNull();
    expect(el.querySelectorAll('code').length).toBe(2);
  });

  it('escapes HTML inside wikilink labels', () => {
    const el = render('[[MQTT Broker|<img src=x onerror=alert(1)>]]');

    expect(el.querySelector('img')).toBeNull();
    expect(el.querySelector('a')?.textContent).toBe('<img src=x onerror=alert(1)>');
  });
});
