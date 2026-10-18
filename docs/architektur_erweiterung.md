<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# md-ai Hybrid-GraphRAG-Architektur
## SQLite + Wissensgraph + Ollama + Cloud-LLMs + Desktop-Vergleichsmessung

Status-Legende für alle Aussagen in diesem Dokument:

- `[implementiert]` — im aktuellen Code vorhanden und geprüft.
- `[geplant]` — beschlossen, aber noch nicht umgesetzt.
- `[offen]` — Designentscheidung, die vor der Umsetzung getroffen werden muss.
- `[Blocker]` — verhindert die Umsetzung, bis entschieden oder behoben.

Dieses Dokument ist die Architektur-Spezifikation (RFC) für den Ausbau von `md-ai` unter `docs/architektur_erweiterung.md`. Die Referenz für den bereits implementierten Retriever-Kern ist `docs/Hybrid_GraphRAG_DE.md`; siehe Abschnitt 16 zur Konsolidierung. Umsetzbare Teilschritte werden als Feature-Specs unter `specs/features/<feature>/` (Spec Kit) geführt; der erste Schritt ist Issue [#24](https://github.com/Zheng-Bote/md_indexer-searcher/issues/24) (Online-LLMs im Desktop-Frontend).

Stand: 2026-10-18 (nach #21, #22, #23, #29).

---

# 1. Zielsetzung

`md-ai` erweitert das lokale RAG-System zu einer mehrstufigen Hybrid-GraphRAG-Plattform mit transparenter, vom Anwender steuerbarer Hybrid-Ausführung (Lokal + Cloud).

Die Architektur verfolgt folgende Ziele:

- Vollständig lokale Grundfunktionalität (`local_only` als Standard)
- Eine einzige SQLite-Datenbank pro Vault (Index-DB `<vault>/.md_indexer/md_indexer.db`) als zentraler Wissens- und Vektorindex
- Wissensgraph-basierte Retrieval-Strategien inkl. Multi-Hop Reasoning Chains
- Semantische Suche über ausschließlich lokal erzeugte Embeddings (`bge-m3`)
- Sauber getrennte Interfaces für LLM-Generierung (`provider.LLMProvider`) und Embeddings (`embedding.Embedder`)
- Optionale Nutzung leistungsfähiger Cloud-Modelle (Ollama Cloud, OpenRouter, Groq, NVIDIA NIM) nach expliziter Nutzerwahl
- Manuell startbare Vergleichsmessung (Lokal vs. Cloud) im grafischen Desktop-Frontend als Entscheidungsgrundlage
- Strikter Geheimnisschutz: API-Keys ausschließlich in geschützter `.env`, zwingende Secret-Redaction vor jedem LLM-Aufruf
- Kein stilles Routing: jeder Wechsel zwischen lokal und Cloud ist entweder vorgewählt oder explizit begründet und nachvollziehbar
- Markdown bleibt die einzige Source of Truth

Grundsatz:

> Markdown-Dateien sind die Wahrheit. Die Index-DB (`md_indexer.db`) ist lediglich ein materialisierter Wissens- und Embedding-Index und kann jederzeit aus dem Vault neu erzeugt werden (wobei gültige Sektions-Embeddings über Content-Hashes erhalten bleiben).

---

# 2. Gesamtarchitektur

```text
                    +----------------------+
                    | Markdown Vault       |
                    | ADRs                 |
                    | Dokumentation        |
                    | Architekturwissen    |
                    +-----------+----------+
                                |
                                v
                    +----------------------+
                    | md-indexer           |
                    | + OllamaEmbedder     |
                    |   (lokal: bge-m3)    |
                    +-----------+----------+
                                |
                                v
            +--------------------------------------+
            | md_indexer.db (Single SQLite DB)     |
            | <vault>/.md_indexer/                 |
            | - Dokumente (inkl. Markdown), Sekt.  |
            | - Tags, Links                        |
            | - FTS5 (fts_sections)                |
            | - Entities, Properties, Relations    |
            | - Section Embeddings (BLOB + Hash)   |
            +-------------------+------------------+
                                |
                                | (strikter Read-Only-Zugriff)
                                v
                    +----------------------+
                    | md-ai / md-api       |
                    +-----------+----------+
                                |
                                v
                +---------------------------+
                | Query Classification      |
                +-------------+-------------+
                              |
                              v
                +---------------------------+
                | Retrieval Planning        |
                +-------------+-------------+
                              |
        +---------------------+----------------------+
        |                     |                      |
        v                     v                      v
 +--------------+    +---------------+    +-------------------+
 | FTS5 Search  |    | Entity Search |    | Vector Search     |
 | (BM25)       |    | + Knowledge   |    | (lokal via        |
 |              |    |   Gaps        |    |  Ollama bge-m3)   |
 +--------------+    +---------------+    +-------------------+
        |                     |                      |
        +----------+----------+----------+-----------+
                   |                     |
                   v                     v
            +--------------------------------+
            | Weighted Graph Expansion       |
            | - Entity Relations (bidirekt.) |
            | - Wikilinks / Backlinks        |
            | - Spezifische Tags             |
            | - Multi-Hop Reasoning Chains   |
            +---------------+----------------+
                            |
                            v
                  +----------------------+
                  | Plan-Aware RRF       |
                  | Fusion Engine        |
                  +----------+-----------+
                             |
                             v
                  +----------------------+
                  | Context Builder      |
                  | Phase 1: Budget +    |
                  | Token-Messung        |
                  +----------+-----------+
                             |
                             v
                  +----------------------+
                  | Execution Policy     |
                  | (Service-Ebene,      |
                  |  NICHT Provider)     |
                  | - local_only         |
                  | - auto_context       |
                  | - cloud_only         |
                  | - compare            |
                  | Fail-closed          |
                  +----+------------+----+
                       |            |
                       v            v
                +-----------+   +------------------------+
                | Ollama    |   | Cloud-Provider         |
                | (Lokal,   |   | OpenRouter, Groq,      |
                |  localhost|   | NVIDIA NIM (OpenAI-    |
                |  ohne     |   | kompatibel), Ollama    |
                |  :cloud)  |   | Cloud / :cloud-Modelle |
                +-----------+   +------------------------+
```

Zwei strukturelle Punkte, die das Diagramm festhält:

- Die Auswahl des Providers ist eine **eigene Stufe über** der Provider-Schicht. Sie gehört weder in `internal/ai/provider` noch in ein Interface, das nur einen Prompt sieht (siehe 3.3).
- Der Context Builder liefert in Phase 1 die Token-Messung, die `auto_context` für seine Entscheidung braucht. Ohne diese Messung ist der Modus nicht implementierbar (siehe 6.2).

---

# 3. Schnittstellen-Konzept (Interface Segregation)

## 3.1 LLM-Generierung (`internal/ai/provider`) — Interfaces und Ollama `[implementiert]`, Cloud-Provider `[geplant]`

Die Textgenerierung ist vollständig vom Embedding-Backend entkoppelt. Heute implementiert nur der Ollama-Provider `LLMProvider`, `StreamingLLMProvider` und `ModelLister`. Die Cloud-Provider aus #24 (ein OpenAI-kompatibler Provider für OpenRouter, Groq und NVIDIA NIM; API-Key-Unterstützung im Ollama-Provider für Ollama Cloud) implementieren dieselben Interfaces:

```go
type LLMProvider interface {
    Generate(ctx context.Context, req *model.GenerateRequest) (*model.GenerateResponse, error)
    Name() string
}

type StreamingLLMProvider interface {
    LLMProvider
    GenerateStream(ctx context.Context, req *model.GenerateRequest, onToken func(token string)) (*model.GenerateResponse, error)
}

type ModelLister interface {
    ListModels(ctx context.Context) ([]model.ModelInfo, error)
}
```

## 3.2 Lokale Embeddings (`internal/embedding`) `[implementiert]`

Embeddings werden **ausschließlich lokal** über `embedding.Embedder` (`OllamaEmbedder` mit `bge-m3`) erzeugt und als Float32-BLOBs direkt in der Index-DB (`section_embeddings`) gespeichert:

```go
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Model() string
}
```

### Warum Embeddings zwingend lokal bleiben:

1. **Vektorraum-Konsistenz:** Index-Embeddings (`md-indexer`) und Query-Embeddings (`md-ai`) müssen dasselbe Modell (`bge-m3`) verwenden, unabhängig davon, welches Chat-Modell die Antwort formuliert.
2. **Datenschutz & Kosten:** Das Einbetten des gesamten Vaults verursacht keinen Cloud-Traffic und keine API-Kosten.
3. **Kein separater Vektor-Store nötig:** `section_embeddings` liegt transaktionssicher und referentiell integer in der Index-DB. Seit #29 sind Dokument-IDs pfadbasiert; Sektions-IDs (`<docID>-<Überschrift>[#n]`) und damit die Embedding-Schlüssel bleiben stabil, solange eine Notiz nicht umbenannt wird.

## 3.3 Warum die Routing-Entscheidung nicht im Provider-Paket liegt `[geplant]`

Ein `RoutingProvider`, der selbst entscheidet, ob lokal oder in der Cloud generiert wird, ist mit dem heutigen Vertrag nicht umsetzbar:

- `model.GenerateRequest` enthält nur `Model`, `Prompt`, `System`, `Temperature`, `MaxTokens` und `ContextWindow`. Es fehlen `QueryType`, `GraphDepth`, der `RetrievalPlan` und die gemessene Kontextgröße — also genau die Größen, aus denen `auto_context` seine Trigger ableitet.
- `md-indexer.sdd` verlangt, dass providerspezifischer Protokollcode ausschließlich in `internal/ai/provider` und `internal/embedding` liegt. Eine Auswahlheuristik ist kein Protokollcode.

**Entscheidung:** Die Ausführungs-Policy lebt auf der Service-Ebene (`internal/ai/assistant` bzw. ein eigener `execution`-Baustein), erhält den `RetrievalResult`-Plan und die Token-Messung und wählt daraus den Provider. `provider` bleibt ein reines Transportpaket. `GenerateRequest` wird nicht um Routing-Metadaten erweitert.

---

# 4. Sicherheitsarchitektur

## 4.1 API-Keys und `.env` `[Blocker]` (`.gitignore` `[implementiert]`)

Gemäß `md-indexer.sdd` und den Entscheidungen in Issue #24 gilt:

- md_indexer nutzt eine **eigene `.env` im Format von `ai_txt-system`** und liest nicht dessen `private.env`: je Anbieter `<PROVIDER>_API_KEY`, `<PROVIDER>_ENDPOINT` und die angebotenen Modelle als `<PROVIDER>_LLM_01 … _LLM_nn` (Anbieter: `OLLAMA` für Ollama Cloud, `OPENROUTER`, `GROQ`, `NVIDIA`).
- Keys kommen ausschließlich aus der Prozessumgebung oder dieser `.env`. An Sidecars werden sie per Umgebungsvariable übergeben.
- **Verboten:** Übergabe per CLI-Argument (`--api-key`), Ausgabe in Logs, Rückgabe in HTTP-Responses.
- Gegenüber dem Desktop-Frontend meldet `md-api` lediglich Metadaten (Anbieter, Modellnamen, Einstufung lokal/Cloud), nie Keys.

Stand und verbleibende Arbeit:

- **`.gitignore`** `[implementiert]` (Commit `abdb0a9`): `.env`, `.env.*` und `/data/.env*` werden ignoriert. Eine committete `example.env` dokumentiert das Format (Name bewusst nicht `.env.example`, weil `.env.*` diese Datei ignorieren würde).
- **Loader existiert nicht.** Im Code gibt es keinen `.env`-Parser. Zu bauen ist ein Loader mit definierter Präzedenz: `Prozessumgebung > .env > md-ai.yaml > Defaults`. Explizite Umgebung gewinnt immer, damit Container- und CI-Betrieb funktioniert.
- **Dateirechte.** `.env` wird mit `0600` angelegt; der Loader warnt, wenn die Datei für Gruppe oder Andere lesbar ist.
- **Ort der Datei** (laut #24 konfigurierbar über Desktop-Settings, CLI-Flag und Umgebungsvariable). Vorschlag für den Default: `~/.config/md-indexer/.env`, überschreibbar mit `MD_AI_ENV_FILE`. Nicht hart auf ein Fremd-Repository verdrahten.

## 4.2 Secret-Redaction und ihre Grenzen `[implementiert]`

`contextbuilder.RedactSecrets` maskiert vor jedem LLM-Aufruf deterministisch:

- Key-Value-Muster mit `api_key`, `secret`, `token`, `password`, `access_key`
- bekannte Token-Präfixe (`sk-…`, `gh[pousr]_…`, `AIza…`)
- `Authorization: Bearer …`

Wichtig für die Bewertung des Restrisikos: Das ist **Best-Effort auf Regex-Basis, keine Garantie**. Nicht erkannt werden unter anderem Schlüssel ohne sprechenden Namen, mehrzeilige Zertifikate und private Keys, Base64-Blobs sowie Zugangsdaten in Prosa. Für Cloud-Aufrufe ist deshalb zusätzlich vorzusehen:

- **Payload-Vorschau vor dem ersten Cloud-Aufruf einer Sitzung:** Anzahl Sektionen, geschätzte Tokens und die betroffenen Dokumentpfade.
- **Harte Obergrenze** für die Payload-Größe pro Cloud-Request.
- **Einmalige, sichtbare Zustimmung** pro Sitzung (nicht pro Frage), die im Frontend widerrufbar ist.

## 4.3 Remote-Zugriff auf `md-api` `[Blocker]`

`md-api` bietet derzeit nur `-host` und **keine Authentifizierung**; seit #31 lauscht es ohne `-host` nur auf `127.0.0.1` (siehe 4.4), ein Remote-Zugriff erfordert also ein explizites `-host` und ist dann ungeschützt. `md-indexer.sdd` verlangt zudem, dass von Client-Apps gestartete Sidecars an `127.0.0.1` binden.

Daraus folgt: Der im Deployment beschriebene Laptop-Online-Modus über WireGuard macht den vollständigen Vault netzwerklesbar — inklusive `/api/ai/ask`, das Kontextblöcke zurückliefert. Das ist kein Deployment-Detail, sondern eine Grenzverletzung. Vor Umsetzung zu entscheiden:

- **Option A (empfohlen):** SSH-Tunnel oder WireGuard plus Bearer-Token mit konstanter Zeitprüfung; `md-api` bindet zusätzlich an die VPN-Schnittstelle, nicht an `0.0.0.0`.
- **Option B:** mTLS zwischen Laptop und Desktop.
- **In jedem Fall:** `md-indexer.sdd` wird im selben Change aktualisiert (Loopback-Regel, Authentifizierungspflicht für nicht-loopback-Bindings). Laut `.specdd/bootstrap.project.md` ist eine Architekturänderung ohne Spec-Update unzulässig.

## 4.4 Lokale Angriffsfläche von `md-api` und `md-indexer` `[implementiert]` (Issue [#31](https://github.com/Zheng-Bote/md_indexer-searcher/issues/31))

Bis #31 lauschten `md-api` und `md-indexer` ohne `-host` auf allen Schnittstellen, und `md-api` sendete `Access-Control-Allow-Origin: *`. Damit waren Suche, `/api/document` und `/api/ai/ask` aus dem LAN und für jede im Browser geöffnete Webseite lesbar. Seit #31 gilt:

- **Bind-Default `127.0.0.1`** für beide Programme. Jede andere Adresse (auch `0.0.0.0` oder ein leerer Wert) muss explizit per `-host` gesetzt werden und erzeugt eine Warnung, dass die API ohne Authentifizierung erreichbar ist. Adressen werden mit `net.JoinHostPort` gebildet (IPv6-fähig).
- **Kein CORS:** `md-api` sendet keine `Access-Control-*`-Header; Browser verweigern damit das Lesen fremder Antworten. Die Desktop-App nutzt das Tauri-HTTP-Plugin und braucht kein CORS.
- **`md-indexer` beantwortet nur noch `/health`.** Die früheren Routen `/documents` und `/search` lasen das Repository direkt und umgingen die Search-Engine-Schicht.
- **Docker:** `md-api` läuft im Container mit `-host 0.0.0.0`; die Ports sollten nur an `127.0.0.1` veröffentlicht werden.
- `md-indexer.sdd` verlangt Loopback als Default für alle HTTP-Server und verbietet Wildcard-CORS.

Für einen Zugriff von anderen Rechnern bleibt 4.3 (Authentifizierung) der Blocker.

## 4.5 Prompt Injection `[offen]`

Abgerufene Markdown-Inhalte sind für das LLM **untrusted Input**. Ein Vault-Dokument kann Anweisungen enthalten, die das Modell als Instruktion liest. Bei einem externen Provider wiegt das schwerer, weil dieselbe Anfrage auch Daten exportiert.

Vorzusehen:

- Explizite Instruktionshierarchie im Systemprompt: abgerufener Kontext ist Datenmaterial, niemals Anweisung.
- Kontextblöcke werden klar abgegrenzt und mit ihrer Herkunft gelabelt.
- Keine Werkzeug- oder Aktionsfähigkeit für das Modell in dieser Ausbaustufe.

---

# 5. Provider

## 5.1 Ollama-Provider (Primärer Standard-Provider) `[implementiert]`

Jede Anfrage wird standardmäßig lokal verarbeitet.

**„Lokal“ ist eine Eigenschaft des Modells, nicht des Providers.** Der lokale Ollama-Daemon leitet Cloud-Modelle (Suffix `:cloud`) an ollama.com weiter; sie erscheinen auch in `/api/tags` und damit in der Modellauswahl. Dazu kommt Ollama Cloud direkt per API-Key (#24). Als lokal gilt daher nur ein Modell, das über einen Ollama-Host auf `localhost`/`127.0.0.1` läuft und kein `:cloud`-Modell ist. Diese Einstufung steuert `local_only`, die Zustimmung (4.2) und das Cloud-Badge im Frontend `[geplant]`.

Anwendungsfälle:

- Tägliche Wissenssuche, Dokumentation, ADR-Abfragen, Codeanalyse
- Offline-Betrieb und datenschutzkritische Inhalte
- Kostenfreie Ausführung ohne Cloud-Traffic

Klarstellung zu zwei verbreiteten Fehlannahmen:

- **Token-Limits existieren.** Das Kontextfenster ist durch `max_context_tokens` und das Modellfenster begrenzt; der Code dimensioniert `num_ctx` dynamisch in 8192er-Schritten.
- **`temperature: 0.1` ist nicht deterministisch.** Exakt reproduzierbare Läufe erfordern `temperature: 0` plus festen Seed. Für den Alltag ist `0.1` sinnvoll, für Vergleichsmessungen nicht.

### Desktop-Konfiguration (62 GB RAM, nur CPU)

```yaml
execution_mode: local_only        # [geplant]; heute implizit
ollama_host: "http://localhost:11434"
model: "qwen3:14b"                # Projekt-Default
embedding_model: "bge-m3"
temperature: 0.1                  # Projekt-Default heute: 0.2
max_context_tokens: 4000          # Projekt-Default
```

Messung vom 2026-10-18 auf diesem Rechner (Test-Vault mit 2.056 Notizen, Kontextbudget 4.000 Tokens):

| | `qwen3:14b` | `qwen3:32b` |
|---|---|---|
| RAM geladen | 10,8 GB | 22,6 GB |
| Prefill | 42 Tokens/s | 13 Tokens/s |
| Generierung | 3,7 Tokens/s | 0,8 Tokens/s |
| Antwortzeit (3 Fragen) | 1:51–2:36 min | 7:03–8:19 min, 1× Timeout nach 600 s |

Die Antwortqualität von `qwen3:32b` war nur leicht besser. Entscheidung: `qwen3:14b` bleibt Default, `qwen3:32b` ist über die Modellauswahl optional. Ein größeres Kontextbudget (z. B. 16.000 Tokens) ist auf dieser CPU nicht praktikabel (Prefill allein ≈ 6 min mit 14b, ≈ 20 min mit 32b) und bleibt Cloud-Modellen vorbehalten.

### Laptop-Konfiguration (16 GB RAM)

```yaml
execution_mode: local_only
ollama_host: "http://localhost:11434"
model: "qwen3:8b"
embedding_model: "bge-m3"
temperature: 0.1
max_context_tokens: 4000
```

## 5.2 Cloud-Provider (Issue #24) `[geplant]`

Werden ausschließlich genutzt, wenn der Anwender ein Cloud-Modell auswählt (Desktop-Modellauswahl, CLI-Flag) bzw. später über den Ausführungsmodus (`auto_context`, `cloud_only`) oder eine Vergleichsmessung.

### Stufe 1: Manuelle Modellwahl (Issue #24, beschlossen)

- Ein **OpenAI-kompatibler Provider** (`/chat/completions`, `Authorization: Bearer`) deckt OpenRouter, Groq und NVIDIA NIM ab; der Ollama-Provider erhält einen optionalen API-Key für Ollama Cloud.
- Die **Modellliste** kommt aus der `.env` (`<PROVIDER>_LLM_xx`), nicht aus `/models` der Anbieter (OpenRouter listet mehrere hundert Modelle). Das Frontend gruppiert nach Anbieter und kennzeichnet Cloud-Modelle sichtbar.
- **Kontextbudget pro Provider:** `max_context_tokens: 4000` ist auf lokale CPU-Inferenz abgestimmt; Cloud-Modelle erhalten ein eigenes, größeres Budget.
- **Fehler:** HTTP 401/429 werden als verständliche Meldung angezeigt.
- Retrieval und Embeddings bleiben lokal.

### Stufe 2: Deklarative Modellstrategie für `auto_context` `[offen]`

Für eine automatische Auswahl nach QueryType wäre eine deklarative Zuordnung denkbar (Beispiel OpenRouter):

```yaml
openrouter:
  base_url: "https://openrouter.ai/api/v1"
  default_model: "deepseek/deepseek-r1"
  max_context_tokens: 32000
  max_cost_usd_per_request: 0.05
  max_cost_usd_per_day: 1.00
  request_timeout_seconds: 120
  models_by_query_type:
    architecture: "deepseek/deepseek-r1"      # Architekturreviews, Trade-Offs, Root-Cause
    impact: "deepseek/deepseek-r1"            # Mehrstufige Abhängigkeits- und Impact-Analysen
    fact: "anthropic/claude-3.7-sonnet"       # Präzise Dokumentation & ADR-Zusammenfassung
    exploration: "google/gemini-2.5-pro"      # Große Kontexte, Vault-weite Analysen
```

Zu beachten:

- Die Modell-IDs sind Beispiele und veralten. Beim Start wird gegen die Modellliste des Providers validiert; ein unbekannter Name erzeugt eine klare Fehlermeldung statt eines 404 zur Laufzeit.
- Reasoning-Modelle wie `deepseek-r1` erzeugen lange Zwischenausgaben. Latenz und Kosten sind deshalb Teil der Messung (Abschnitt 7) und nicht vorab anzunehmen.
- `max_cost_usd_per_request` und `max_cost_usd_per_day` sind für **kostenpflichtige** Modelle Pflichtfelder; ohne sie ist `cloud_only` ein unkontrollierter Kostenpfad. Für die in der `.env` gelisteten Free-Tier-Modelle (OpenRouter `:free`, Groq, NVIDIA-Credits) sind Rate-Limits der relevante Grenzwert `[offen]`.

---

# 6. Ausführungsmodi

Der Modus ist ein expliziter Parameter der Anfrage und wird nicht aus versteckten Heuristiken abgeleitet.

## 6.1 `local_only` (Standard) `[geplant]`

- Ausschließliche Verarbeitung über die lokale Ollama-Instanz.
- Garantierter Offline-Betrieb; kein Byte verlässt das System — vorausgesetzt, das gewählte Modell ist im Sinne von 5.1 lokal (localhost-Host, kein `:cloud`-Modell). `local_only` lehnt nicht-lokale Modelle ab, statt sie auszuführen.
- Ein Vergleichslauf ist in diesem Modus **nicht** implizit enthalten: Er sendet Vault-Inhalte nach außen und braucht eine eigene, sichtbare Zustimmung pro Lauf (Abschnitt 7).

## 6.2 `auto_context` `[offen]`

Standardanfragen laufen lokal. Eskaliert wird nur, wenn eine der beiden Bedingungen zutrifft:

- der **gemessene** Kontextbedarf überschreitet `cloud_token_threshold`,
- der `RetrievalPlan.GraphDepth` erreicht oder überschreitet `cloud_graph_depth_threshold`.

Zwei Umsetzungshindernisse, die vor der Implementierung gelöst werden müssen:

**a) Die Token-Messung fehlt.** `ContextBuilder.BuildContext` gibt ausschließlich die bereits budgetierten Blöcke zurück. Der ungekürzte Bedarf ist mit der heutigen Signatur nicht abrufbar. Nötig ist ein zusätzliches Ergebnis, etwa:

```go
type BuildResult struct {
    Blocks       []model.ContextBlock
    UsedTokens   int
    TotalTokens  int   // ungekürzter Bedarf vor Budgetierung
    Truncated    bool
}
```

Ohne `TotalTokens` ist der Schwellwert nicht berechenbar, und `auto_context` bleibt Papier.

**b) Zweiphasige Context-Budgetierung ohne doppeltes Retrieval.** Würde bei einer Eskalation lediglich das LLM gewechselt, während `Fuse()` vorher hart auf `plan.MaxResults` (15) bzw. das lokale Token-Budget gekürzt hat, bekäme die Cloud dieselbe abgeschnittene Evidenz. Ein zweiter kompletter Retrieval-Durchlauf (erneutes Ollama-Query-Embedding + SQLite-Abfragen) wäre jedoch unnötig teuer. Stattdessen hält `Retriever.Retrieve` bereits den erweiterten RRF-Kandidatenpool (`cloud_max_results`, z. B. 30 Sektionen) im `RetrievalResult` vor, und nur der `ContextBuilder` budgetiert zweiphasig:

```text
Phase 1: Einmaliges Retrieval (Kandidatenpool bis cloud_max_results)
         + ContextBuilder mit lokalem Budget (max_context_tokens)
         -> liefert BuildResult{Blocks, UsedTokens, TotalTokens, Truncated}

Phase 2 (nur bei Eskalation auf Cloud):
         Kein erneutes DB-/Embedding-Retrieval;
         ContextBuilder läuft über dasselbe RetrievalResult
         mit dem Cloud-Kontextbudget des gewählten Modells (z. B. 32.000 Tokens)
```

**c) Der GraphDepth-Trigger muss zur Konfiguration passen.** Der Planner kappt jede Tiefe auf `cfg.GraphDepth` (Default 3). Ein Schwellwert `>= 4` feuert damit nie. Entweder wird der Default angehoben, oder der Schwellwert wird auf `>= cfg.GraphDepth` bezogen. Ein Trigger, der nie auslöst, ist schlimmer als keiner, weil er Sicherheit suggeriert.

## 6.3 `cloud_only` `[geplant]`

- Alle LLM-Antworten werden über das gewählte Cloud-Modell generiert (Ollama Cloud, OpenRouter, Groq oder NVIDIA NIM).
- Retrieval und Embedding-Berechnung (`bge-m3`) laufen weiterhin zu 100 % lokal.
- Alle Kontextblöcke durchlaufen `RedactSecrets`, die Payload-Obergrenze und die Sitzungszustimmung aus 4.2.

## 6.4 Fail-closed-Semantik `[offen]`

Drei Pfade, die heute undefiniert sind und still nach außen führen könnten:

- **Ollama nicht erreichbar in `auto_context`:** keine Eskalation. Ein lokaler Ausfall darf niemals zum Exportpfad werden. Der Fehler wird gemeldet.
- **`cloud_only` ohne Key oder ohne Netz:** klarer Fehler. Kein stiller Rückfall auf lokal, weil der Nutzer sonst eine lokale Antwort für eine Cloud-Antwort hält.
- **HTTP 401/429/5xx im Cloud-Arm:** Fehler wird sichtbar gemacht. Kein Retry, der den Provider wechselt.

## 6.5 Persistenz und Request-Parameter `[offen]`

`/api/ai/ask` akzeptiert heute ausschließlich `{"question": "..."}`. Vorgesehen ist:

```json
{
  "question": "Welche Risiken enthält diese Architektur?",
  "execution_mode": "auto_context"
}
```

- Der Modus ist **zustandslos** pro Anfrage; `md-api` schreibt keine YAML.
- Persistiert wird nur der Default in `AppSettings` (Desktop) bzw. `md-ai.yaml`.
- Ein Schreibpfad von `md-api` in die Konfiguration wäre neu, kollidiert mit Sidecar-Neustarts und ist in `md-indexer.sdd` nicht vorgesehen.

---

# 7. Vergleichsmessung (Lokal vs. Cloud) `[geplant]`

Die Vergleichsmessung ist eine **Messreihe mit definierten Metriken**, keine objektive Qualitätsentscheidung. Ein einzelner Lauf über nicht-deterministische Modelle ist keine Entscheidungsgrundlage; das Dokument sollte diese Grenze nicht überspielen.

## 7.1 Ablauf

```text
Nutzer startet "Lokal vs. Cloud vergleichen"
                  │
                  ▼
   1. Einmaliges lokales Hybrid-Retrieval
      (Classification -> Plan -> FTS/Vector/Entity/Graph -> RRF -> RedactSecrets)
                  │
        ┌─────────┴─────────┐
        ▼                   ▼
 2a. Lokales LLM     2b. Cloud LLM
     (Ollama)            (gewähltes Cloud-Modell)
        │                   │
        └─────────┬─────────┘
                  ▼
   3. Side-by-Side Vergleichsansicht im Frontend
```

Beide Arme laufen unabhängig. Ein Fehler im Cloud-Arm (429, Timeout, Auth) darf die lokale Antwort nicht verwerfen; er wird als eigener Zustand angezeigt.

## 7.2 Erfasste Messwerte

- `retrieval_ms` und `generation_ms` getrennt. Beim lokalen Lauf dominiert der CPU-Prefill; ohne Trennung ist der Vergleich wertlos, weil das Retrieval in beiden Armen identisch ist.
- Für den lokalen Ollama-Arm zusätzlich die aus `/api/generate` gelieferten Teilzeiten: `load_ms` (`load_duration` für Kaltstart/Modell-Load), `prefill_ms` (`prompt_eval_duration`) und `eval_ms` (`eval_duration`). Dadurch lässt sich der Kaltstart-Anteil bereits bei einem einzelnen Lauf exakt herausrechnen.
- `total_ms`
- `prompt_tokens`, `completion_tokens` (aus der Provider-Response; OpenAI-kompatible Anbieter liefern `usage`, Ollama liefert `prompt_eval_count` und `eval_count`)
- `cost_usd`, sofern der Provider Kosten meldet
- `matched_context_tokens` — **existiert im Datenmodell noch nicht.** `model.Answer` und `model.RetrievalResult` haben kein Tokenfeld. Es muss ergänzt werden, nicht nur dokumentiert.
- `query_type`, `graph_depth`, `max_results` aus dem Plan
- `sources` und `gaps` je Arm

## 7.3 Methodik

- **Interaktiver Standard im Frontend (`runs = 1`):** Auf CPU-Systemen (z. B. 16-GB-Laptop mit 1–3 Minuten pro lokalem Lauf) würden 4 Läufe hintereinander die UI für 8–12 Minuten blockieren. Standardmäßig läuft daher **ein** Durchlauf pro Arm, wobei `load_ms` (Kaltstart) separat von `prefill_ms + eval_ms` (reine Inferenzzeit im warmen Zustand) ausgewiesen wird.
- **Optionaler Benchmark-Modus (`runs = 3` mit Median):** Für schnelle Systeme (GPU) im UI zuschaltbar, um Varianzen über den Median auszugleichen. Der heutige Desktop ist CPU-only; dort dauert schon ein lokaler Lauf mehrere Minuten (siehe 5.1).
- **Feste Temperatur (`temperature: 0`)** während der Vergleichsmessung für beide Arme, um Sampling-Rauschen zu minimieren.
- **Hinweis in der UI:** Eine einzelne Frage zeigt das Laufzeitverhältnis für genau diesen `QueryType` und Kontextumfang; für eine übergreifende Aussage sollten mehrere repräsentative Fragen (z. B. `fact` vs. `architecture`) verglichen werden.

## 7.4 Endpoint und Zustimmung

- `POST /api/ai/compare` existiert nicht und muss ergänzt werden — inklusive `openapi.yaml` und Frontend.
- Auch im Modus `local_only` sendet ein Vergleichslauf Vault-Inhalte nach außen. Deshalb: explizite Bestätigung pro Lauf mit sichtbarer Dokumentliste und Token-Schätzung, keine stille Ausnahme vom Modus.

---

# 8. Retrieval Engine (Implementierter Kern in `internal/ai/hybrid`) `[implementiert]`

## 8.1 Query Classification & Retrieval Planning

Fragen werden durch `Classifier` und `Planner` klassifiziert und in einen `RetrievalPlan` überführt:

| QueryType | Beispiel | FTS | Entities | Graph | Vectors | GraphDepth | MaxResults |
| :--- | :--- | :---: | :---: | :---: | :---: | :---: | :---: |
| **`fact`** | *„Was ist ADR-003?“* | `true` | `false` | `false` | `true` | `0` | `8` |
| **`entity`** | *„Was ist der Encryption Service?“* | `true` | `true` | `false` | `true` | `0` | `10` |
| **`impact`** | *„Welche Komponenten verwenden Encryption?“* | `true` | `true` | `true` | `false` | `3` | `15` |
| **`architecture`** | *„Wie funktioniert die Ingestion Pipeline?“* | `true` | `true` | `true` | `true` | `4` (→ `3`) | `15` |
| **`exploration`** | *„Wie schützen wir sensible Daten?“* | `true` | `true` | `true` | `true` | `1` | `12` |

`GraphDepth` ist der Planwert; der Planner kappt ihn auf `cfg.GraphDepth` (Default `3`, `--graph-depth`). Bei Default-Konfiguration nutzt `architecture` also Tiefe `3` (siehe 6.2c).

Anmerkung zu `impact` ohne Vektoren: Das ist das heutige Verhalten, keine begründete Entscheidung. Impact-Fragen profitieren typischerweise von semantischem Recall, weil betroffene Komponenten selten dieselben Wörter verwenden. `[offen]` — vor der nächsten Planner-Änderung evaluieren.

## 8.2 Die vier Retrieval-Quellen auf der Index-DB

### FTS5 Retrieval (`SQLiteFTS`)

Durchsucht `fts_sections` mit `-bm25(fts_sections, 0, 0, 1.0, 0.8, 0.7, 0.5, 0.4)`:

- Title `1.0`, Heading `0.8`, Tag `0.7`, Content `0.5`, Metadata `0.4`

### Entity & Knowledge-Gap Retrieval (`SQLiteEntities`)

Nutzt `entities`, `entity_properties`, `entity_relations`, `entity_types`:

- Extrahiert benannte Entitäten aus der Frage, lädt Attribute, direkte Relationen und definierende Sektionen.
- **Retrieval-Time Knowledge Gap Detection:** Prüft beim Retrieval, ob für gefundene Komponenten/Services eine Beschreibung, eine API-Sektion und eine verlinkte ADR existieren, und erzeugt strukturierte `model.KnowledgeGap`-Einträge.

### Semantic Vector Retrieval (`SQLiteVectors`)

- Berechnet das Query-Embedding lokal über Ollama (`bge-m3`).
- Vergleicht per Cosine Similarity (≥ `0.55`) gegen die in `section_embeddings` gespeicherten Sektions-Vektoren (Brute Force, mit In-Memory-Cache).
- Findet semantische Konzepte (z. B. *„sensible Daten schützen“* → *„Field-Level Encryption“*) ohne exakte Wortübereinstimmung.

### Wissensgraph & Multi-Hop GraphRAG (`SQLiteGraph`)

Startet bei den Top-Seed-Dokumenten aus Entity-, FTS- und Vektorsuche und traversiert gewichtet über:

- `USES` (`1.0`), `DEPENDS_ON` (`0.9`), `ADR` (`0.8`), `CONTAINS` (`0.5`), `LINKS_TO` (`0.4`), `SHARES_TAG` (`0.3`), `REFERENCES` (`0.2`); unbekannte Relationstypen `0.3`.

Extrahiert redundanzfreie Multi-Hop-Pfade als strukturierte `model.ReasoningChain`:

```go
type ReasoningChain struct {
    Steps []ReasoningStep
}

type ReasoningStep struct {
    Source   string
    Relation string
    Target   string
}
```

## 8.3 Graph-Extraktionsvertrag `[offen]`

Multi-Hop-GraphRAG ist nur so gut wie die Kanten im Vault. `SQLiteGraph` traversiert heute drei Kantenarten:

1. **Frontmatter-Listen** in Entitätsdokumenten (`type: component` plus z. B. `uses: [Package Model]`) → `entity_relations`. Der Schlüsselname wird zu `strings.ToUpper(k)`; unbekannte Schlüssel landen auf `WeightOther`. Wikilinks im Text von Entitätsdokumenten werden zusätzlich als `LINKS_TO`-Relation in `entity_relations` abgelegt.
2. **Wikilinks aller Dokumente** `[[Ziel]]`, `[[Ziel|Alias]]`, `[[Ziel#Überschrift]]` → Tabelle `links`; die Graph-Expansion nutzt sie als `LINKS_TO`-Kanten (`0.4`) zwischen Dokumenten, auch außerhalb von Entitätsdokumenten.
3. **Gemeinsame spezifische Tags** (Tags an 2–10 Dokumenten) → `SHARES_TAG`-Kanten (`0.3`).

Die stark gewichteten Typen (`USES`, `DEPENDS_ON`, `ADR`) entstehen aber nur aus Frontmatter-Listen.

Daraus folgen zwei Pflichten, die festgehalten werden müssen, weil sonst die Graphqualität autorenabhängig und unvorhersehbar ist (eine Vorlage mit typisierten Relationslisten liegt in `_templates_/entity.md` des Test-Vaults):

- **Vault-Konvention dokumentieren:** erlaubte Relationstypen, Schreibweise der Targets, Pflichtfeld `type:`, Namensauflösung über `documents.title` (`COLLATE NOCASE`).
- **Abdeckung messen:** Anteil der Entitätsdokumente mit mindestens einer Relation. Ein Vault ohne Frontmatter-Relationen hat nur Wikilink- und Tag-Kanten — dann tragen `USES`/`DEPENDS_ON`-Gewichte nichts. Beispiel: Der Test-Vault (Joplin-Export, 2.056 Notizen) hat praktisch keine nutzbaren Graphkanten.

## 8.4 Plan-Aware RRF Fusion (`Fuse`)

BM25-Scores, Cosine-Similarities und Graph-Kantengewichte haben unterschiedliche Verteilungen. Eine lineare Summe wäre ohne Normalisierung nicht vergleichbar; deshalb kombiniert die Fusion Engine über **gewichtete Reciprocal Rank Fusion**:

```text
FinalScore(section) = Σ_source  weight(source) / (rrfK + rank_source + 1)

rrfK = 60
max. 3 Sektionen pro Dokument
```

Basisgewichte (konfigurierbar):

```yaml
fusion_fts_weight: 0.35
fusion_vector_weight: 0.35
fusion_graph_weight: 0.20
fusion_entity_weight: 0.10
```

Plan-abhängiger Graph-Boost für `impact` und `architecture`:

```text
w_Graph ← max(2 · w_Graph , w_FTS)
```

Bei Default-Gewichten ergibt das `0.40`. Die Formel ist maßgeblich, nicht der Wert — bei anderen Basisgewichten fällt das Ergebnis anders aus.

---

# 9. Context Builder & Compression (`internal/ai/contextbuilder`)

## 9.1 Prioritätsstufen `[implementiert]`

1. **Priority 1:** Direkte Entitäts-Definitionen (`res.Entities`) und deren primäre Dokumentsektionen (`strategy == "entity"`)
2. **Priority 2:** Synthetisiertes `ArchitectureContext` (Services, Components, Pipelines, ADRs, Dependencies), direkte `EntityRelations`, Multi-Hop `ReasoningChains`
3. **Priority 3:** Architekturentscheidungen (`strategy == "adr"`)
4. **Priority 4:** Übrige fusionierte Dokumentsektionen, absteigend nach RRF-`FinalScore`

## 9.2 Compression & Schutz `[implementiert]`

- **Near-Duplicate-Filter:** entfernt redundante Abschnitte über Wortmengen-Jaccard (≥ `0.80`).
- **Secret Redaction:** maskiert Geheimnisse (`[REDACTED]`) in allen Blöcken — Best-Effort, siehe 4.2.
- **Token Budgeting:** konservative Schätzung `~3 Bytes/Token`, Abbruch beim ersten Block, der das Budget überschreiten würde.

## 9.3 Zweiphasige Assemblierung `[geplant]`

Für `auto_context` muss `BuildContext` den ungekürzten Bedarf melden (siehe 6.2). Zusätzlich ist die Abbruchlogik zu prüfen: Der heutige `break` verwirft jeden folgenden Block, auch wenn ein kleinerer Block noch ins Restbudget passen würde. Das ist konservativ, aber bei knappem Budget verschenkt es Kontext. `[offen]` — entweder `continue` mit Restbudget oder bewusst beim `break` bleiben und dokumentieren.

---

# 10. Embedding-Lebenszyklus `[teilweise implementiert / offen für CLI]`

`section_embeddings` speichert `section_id`, `embedding`, `dimensions`, `model`, `content_hash`, `created_at`. `md-indexer.sdd` verlangt, dass Re-Indexierung gültige Embeddings nicht verwirft — das ist umgesetzt (`EmbeddingService.EmbedPending`).

Differenzierung zwischen Desktop-App und CLI/Standalone:

- **Desktop-App (`[implementiert]`):** Wechselt der Nutzer in `ModelSelector` das Embedding-Modell, startet `SidecarManager::apply` automatisch zuerst `md-indexer -embed-model <neu>` und wartet auf dessen `/health` (also bis alle fehlenden Sektions-Embeddings für das neue Modell berechnet sind), bevor `md-api` in den Status `Ready` wechselt.
- **CLI / Standalone `md-ai` & `md-api` (`[offen]`):** `SQLiteVectors` filtert auf `WHERE e.model = ?` mit dem aktuell konfigurierten Modell. Wechselt der Nutzer in `md-ai.yaml` oder per Flag das Embedding-Modell, ohne zuvor `md-indexer -embed-model <neu>` ausgeführt zu haben, liefert die Vektorsuche **null Kandidaten** — ohne Fehler, ohne Warnung. Die Antwortqualität sinkt lautlos.
- **Erforderlich für CLI/API:** Start- bzw. Query-Check in `SQLiteVectors`, der prüft, ob für `cfg.EmbeddingModel` Vektoren existieren, obwohl Dokumente vorhanden sind, und bei Abweichung klar warnt.
- **Dimensionsprüfung:** `dimensions` gegen die Modellausgabe validieren; ein Modellwechsel kann die Dimension ändern.
- **Speicherbedarf:** `bge-m3` liefert 1024 Dimensionen, float32 → ca. 4 KB pro Sektion. Bei 10.000 Sektionen sind das ca. 40 MB nur für Vektoren, zusätzlich zu FTS und Graph.

---

# 11. Deployment-Architektur

## 11.1 Desktop (62 GB RAM, nur CPU) – Primärer Knowledge Server

Komponenten:

- `md-indexer`, Index-DB je Vault (inkl. FTS5, Graph & Embeddings), `ollama`, `md-ai`, `md-api`, `apps/desktop`
- Lokale Sidecar-Prozesse der Desktop-App binden zwingend an `127.0.0.1`.

Modelle:

- Lokal: `qwen3:14b` (Default), `qwen3:32b` optional (siehe Messung in 5.1); Embeddings: `bge-m3`
- Optional Cloud (Ollama Cloud, OpenRouter, Groq, NVIDIA NIM) via `.env` (#24) — zunächst manuell gewählt, später für Vergleichsmessungen, `auto_context` oder `cloud_only`.

## 11.2 Laptop (16 GB RAM)

### Online-Modus (über Desktop-Server) `[Blocker]`

```text
Laptop (apps/desktop oder md-ai)
   ↓
WireGuard / SSH-Tunnel
   ↓
md-api (Desktop-Server, authentifiziert)
   ↓
Ollama (qwen3:14b) + Index-DB
```

**Voraussetzung:** Authentifizierung nach 4.3 und ein Spec-Update in `md-indexer.sdd` (die lokale Härtung nach 4.4 ist umgesetzt). Ohne das ist dieser Modus nicht zulässig.

### Eigenständiger Betrieb (Lokal + optional `auto_context`)

- Lokal für schnelle Standardfragen: `qwen3:8b`, Embeddings lokal: `bge-m3`
- Bei `execution_mode: auto_context` eskaliert der Service bei überschrittenem Token-Schwellwert oder tiefer Graph-Analyse auf ein Cloud-Modell — fail-closed nach 6.4.

## 11.3 Datenbank-Lebenszyklus `[implementiert]`

- **Ort (#23):** `internal/vaultdb` legt die Index-DB unter `<vault>/.md_indexer/md_indexer.db` ab, mit Fallback auf `<user data dir>/md-indexer/vaults/<hash>.db`, wenn der Vault schreibgeschützt ist. Alle Werkzeuge nutzen das: `md-indexer -dir`, `md-api -vault`, `md-search --vault`, `md-ai --vault`; die Desktop-Sidecars übergeben nur den Vault. Lesende Werkzeuge legen nie eine DB an. Explizite Pfade (`-db`, `MD_AI_DB_PATH`, `db_path`) haben Vorrang. Datenbanken früherer Desktop-Versionen werden einmalig per `-migrate-from` verschoben. `md-indexer.sdd` erlaubt Schreibzugriffe im Vault nur unter `.md_indexer/`.
- **Inhalt (#22, #29):** `documents.markdown` hält das Original-Markdown für den Dokument-Viewer (`GET /api/document`). Dokument-IDs sind der SHA-256 des vault-relativen Pfads; wiederholte Überschriften erhalten eigene Sektions-IDs (`#2`, `#3`, …). Bestehende Indexe werden beim nächsten `md-indexer`-Lauf migriert, Embeddings dabei umgeschlüsselt statt neu berechnet.
- **Sync-Tools** `[offen, nur dokumentierbar]`: `.md_indexer/.gitignore` (`*`) schließt git aus, die README weist auf den Ausschluss in Syncthing, Nextcloud, iCloud usw. hin. Erzwingen lässt sich das nicht; die Synchronisation einer WAL-Datenbank zwischen zwei aktiv indexierenden Rechnern kann sie beschädigen.

---

# 12. Betriebsgrenzen `[geplant]`

- **Rate-Limits:** HTTP 429 eines Cloud-Anbieters erzeugt eine verständliche Meldung mit Retry-Hinweis, keinen generischen „Frage fehlgeschlagen“-Fehler.
- **Timeouts:** getrennt pro Provider bzw. Modellklasse (lokal/Cloud). Der heutige globale Timeout von 600 s ist auf lokale CPU-Inferenz mit `qwen3:14b` ausgelegt (größere lokale Modelle überschreiten ihn bereits, siehe 5.1) und für Cloud-Aufrufe ungeeignet.
- **Retry:** begrenzt (max. 2) mit exponentiellem Backoff und ausschließlich beim selben Provider. Ein Retry darf niemals den Provider wechseln.
- **Kosten:** `max_cost_usd_per_request` und `max_cost_usd_per_day` werden vor dem Aufruf geprüft. Überschreitung = Fehler, keine stille Kürzung.
- **Nebenläufigkeit:** Die Desktop-App stellt Fragen seriell; parallele Anfragen an dasselbe Modell konkurrieren um dieselbe GPU/CPU und verzerren Messungen.

---

# 13. Transparenz und Nachweisbarkeit `[geplant]`

Prinzip 4 dieses Dokuments verlangt Transparenz. Sie ist nur nachweisbar, wenn sie im Antwortobjekt steht:

`model.Answer` enthält heute `Question`, `Answer`, `Intent`, `Plan`, `Model`, `Sources`, `Gaps`. Zu ergänzen:

- `provider` (z. B. `ollama`, `ollama-cloud`, `openrouter`, `groq`, `nvidia`) und `locality` (`local` | `cloud`, nach 5.1)
- `execution_mode`
- `escalation_reason` (leer bei lokalem Lauf; sonst z. B. `context_tokens>4000`)
- `context_tokens` (genutzt und ungekürzt)
- `cost_usd`, sofern verfügbar

Zusätzlich: ein Log-Eintrag pro Anfrage mit Provider, Modus, Plan, Token-Zahlen und Dauer — ohne Prompt-Inhalt und ohne Secrets. Der bestehende `--retrieve-only`-Modus bleibt das Werkzeug, um Retrieval-Entscheidungen ohne LLM-Aufruf zu prüfen.

---

# 14. Offene Entscheidungen und Blocker

**Blocker (vor jeder Umsetzung zu klären):**

1. `.env`-Loader mit Präzedenz und Dateirechten (4.1). Die `.gitignore`-Regel ist erledigt.
2. Authentifizierung für remote `md-api`: Bearer-Token oder mTLS, plus Spec-Update in `md-indexer.sdd` (4.3).

**Offene Designentscheidungen:**

3. Einstufung lokal/Cloud pro Modell (localhost-Host, kein `:cloud`-Modell) als Grundlage für `local_only`, Zustimmung und Badge (5.1).
4. `BuildResult` mit `TotalTokens` und `Truncated` — Signaturänderung in `internal/ai/contextbuilder`.
5. Zweiphasige Context-Budgetierung für `auto_context` — `RetrievalResult` hält `cloud_max_results` Kandidaten vor; `ContextBuilder` budgetiert bei Eskalation mit dem Cloud-Budget nach (ohne zweites DB-/Embedding-Retrieval).
6. GraphDepth-Schwellwert gegen die Planner-Kappung — Default anheben oder Schwelle relativ zu `cfg.GraphDepth` setzen.
7. Fail-closed bei Ollama-Ausfall — bestätigen, dass keine Eskalation stattfindet.
8. Embedding-Modellwechsel-Warnung für CLI/Standalone (`md-ai` / `md-api`) — prüfen, ob für `cfg.EmbeddingModel` Einträge in `section_embeddings` vorliegen (in der Desktop-App bereits durch den vorgeschalteten `md-indexer`-Lauf gelöst).
9. `impact` ohne Vektoren — Planner evaluieren.
10. `break` vs. `continue` im Token-Budget — Verhalten festlegen und dokumentieren.
11. Kostenlimits nur für kostenpflichtige Modelle; Rate-Limit-Behandlung für Free-Tier-Modelle (5.2).

**Geplant, aber noch nicht vorhanden:**

12. Cloud-Provider und Modellauswahl nach Issue #24 (OpenAI-kompatibler Provider, Ollama-Cloud-Key, `.env`-Modellliste, Cloud-Badge, verständliche 401/429-Fehler).
13. `POST /api/ai/compare` inklusive Schema in `openapi.yaml` (Standard `runs = 1` mit separater Ausweisung von Ollama `load_ms` / `prefill_ms` / `eval_ms`; optional `runs = 3` mit Median).
14. Token- und Kostenfelder in `model.Answer` (`context_tokens`, `cost_usd`, `provider`, `locality`, `execution_mode`, `escalation_reason`).

**Erledigt seit der ersten Fassung:** vault-lokale Index-DB inkl. `md-ai --vault` (#23), Sync-Hinweis in der README, `.gitignore` für `.env`, Original-Markdown im Index (#22), pfadbasierte Dokument-IDs (#29), Loopback-Default und kein Wildcard-CORS (#31).

---

# 15. Architekturprinzipien

1. **Markdown bleibt die einzige Source of Truth** (strikter Read-Only-Zugriff auf den Vault).
2. **Eine einzige SQLite-Datenbank pro Vault** (`<vault>/.md_indexer/md_indexer.db`) ist die alleinige, jederzeit neu aufbaubare Persistenzschicht für Volltext, Graph und Vektoren.
3. **Embeddings bleiben immer lokal (`bge-m3`)**, um Vektorraum-Konsistenz und Datenschutz zu garantieren.
4. **Lokale Verarbeitung (`local_only`) ist der Standard.** Cloud-Nutzung erfolgt ausschließlich nach expliziter Wahl (Cloud-Modell, `auto_context`, `cloud_only`) und ist im Antwortobjekt sichtbar. „Lokal“ wird pro Modell bestimmt, nicht pro Provider.
5. **Fail-closed statt stiller Fallback.** Kein Providerwechsel bei Fehlern, keine Eskalation bei lokalem Ausfall, kein stiller Rückfall von Cloud auf lokal.
6. **Strikte Geheimnishygiene:** API-Keys ausschließlich in der Prozessumgebung oder einer geschützten `.env` (eigenes Format nach #24); alle Kontexte durchlaufen `RedactSecrets`. Redaction ist Best-Effort und ersetzt keine Zustimmung.
7. **Saubere Interface-Trennung:** `provider.LLMProvider` (Generierung) und `embedding.Embedder` (Vektorisierung) sind entkoppelt; die Ausführungs-Policy liegt darüber, nicht darin.
8. **Messen statt Raten:** Die Vergleichsmessung liefert Dauer (inkl. Trennung von Kaltstart `load_ms` und Inferenz), Token, Kosten und Qualität als getrennte Metriken.
9. **Grounded Answers:** Das LLM antwortet ausschließlich auf Basis des bereitgestellten Kontexts und weist Quellen (`Sources`) sowie erkannte Wissenslücken (`KnowledgeGaps`) deterministisch aus.
10. **Nachvollziehbarkeit:** Jede Antwort nennt Provider, Ausführungsmodus, Kontextumfang und — bei Eskalation — den Grund.

---

# 16. Dokument-Governance

Dieses Dokument (`docs/architektur_erweiterung.md`) dient als getrackte Zielbild-Spezifikation (RFC) für die Hybrid-Cloud-Erweiterung. Die Referenz `docs/Hybrid_GraphRAG_DE.md` beschreibt den bereits implementierten lokalen Retriever-Kern und ist im Bootstrap (`.specdd/bootstrap.project.md`) als Einstiegspunkt referenziert.

Damit dieselbe Architektur nicht dauerhaft in zwei Dokumenten parallel gepflegt werden muss, gilt folgende Aufteilung:

- **`docs/architektur_erweiterung.md` (dieses Dokument):** Spezifiziert das Delta als Zielbild (Cloud-Provider hinter `LLMProvider`, Ausführungs-Policy auf Service-Ebene, Vergleichsmessung `/api/ai/compare`, Sicherheits- und Betriebsgrenzen).
- **`specs/features/<feature>/` (Spec Kit):** Jeder umsetzbare Teilschritt erhält eine eigene Feature-Spec mit Plan und Tasks, beginnend mit #24. Bei Abweichungen gilt die Feature-Spec; dieses Dokument wird danach nachgezogen.
- **`docs/Hybrid_GraphRAG_DE.md`:** Bleibt die Referenz des implementierten Retrievers und wird bei Umsetzung um den Graph-Extraktionsvertrag (8.3), den Embedding-Lebenszyklus (10) und die explizite RRF-Formel (8.4) ergänzt.
- **`md-indexer.sdd`:** Wird im selben Change aktualisiert, sobald ein Cloud-Provider oder ein nicht-loopback-Binding umgesetzt wird (Provider-Regel, Redaction-Pflicht vor externen Providern, Authentifizierungspflicht).

