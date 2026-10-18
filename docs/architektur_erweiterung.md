<!--
SPDX-FileCopyrightText: 2026 Zheng-Bote
SPDX-License-Identifier: Apache-2.0
-->

# md-ai Hybrid-GraphRAG-Architektur
## SQLite + Wissensgraph + Ollama + OpenRouter + Desktop-Vergleichsmessung

Status-Legende für alle Aussagen in diesem Dokument:

- `[implementiert]` — im aktuellen Code vorhanden und geprüft.
- `[geplant]` — beschlossen, aber noch nicht umgesetzt.
- `[offen]` — Designentscheidung, die vor der Umsetzung getroffen werden muss.
- `[Blocker]` — verhindert die Umsetzung, bis entschieden oder behoben.

Dieses Dokument ist die Architektur-Spezifikation (RFC) für den Ausbau von `md-ai` unter `docs/architektur_erweiterung.md`. Die Referenz für den bereits implementierten Retriever-Kern ist `docs/Hybrid_GraphRAG_DE.md`; siehe Abschnitt 16 zur Konsolidierung.

---

# 1. Zielsetzung

`md-ai` erweitert das lokale RAG-System zu einer mehrstufigen Hybrid-GraphRAG-Plattform mit transparenter, vom Anwender steuerbarer Hybrid-Ausführung (Lokal + Cloud).

Die Architektur verfolgt folgende Ziele:

- Vollständig lokale Grundfunktionalität (`local_only` als Standard)
- Eine einzige SQLite-Datenbank (`knowledge.db`) als zentraler Wissens- und Vektorindex
- Wissensgraph-basierte Retrieval-Strategien inkl. Multi-Hop Reasoning Chains
- Semantische Suche über ausschließlich lokal erzeugte Embeddings (`bge-m3`)
- Sauber getrennte Interfaces für LLM-Generierung (`provider.LLMProvider`) und Embeddings (`embedding.Embedder`)
- Optionale Nutzung leistungsfähiger Cloud-Modelle über OpenRouter nach expliziter Nutzerwahl
- Manuell startbare Vergleichsmessung (Lokal vs. Cloud) im grafischen Desktop-Frontend als Entscheidungsgrundlage
- Strikter Geheimnisschutz: API-Keys ausschließlich in geschützter `.env`, zwingende Secret-Redaction vor jedem LLM-Aufruf
- Kein stilles Routing: jeder Wechsel zwischen lokal und Cloud ist entweder vorgewählt oder explizit begründet und nachvollziehbar
- Markdown bleibt die einzige Source of Truth

Grundsatz:

> Markdown-Dateien sind die Wahrheit. `knowledge.db` ist lediglich ein materialisierter Wissens- und Embedding-Index und kann jederzeit aus dem Vault neu erzeugt werden (wobei gültige Sektions-Embeddings über Content-Hashes erhalten bleiben).

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
            | knowledge.db (Single SQLite DB)      |
            | - Dokumente, Sektionen, Tags, Links  |
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
                +-----------+   +-------------+
                | Ollama    |   | OpenRouter  |
                | (Lokal)   |   | (Cloud)     |
                +-----------+   +-------------+
```

Zwei strukturelle Punkte, die das Diagramm festhält:

- Die Auswahl des Providers ist eine **eigene Stufe über** der Provider-Schicht. Sie gehört weder in `internal/ai/provider` noch in ein Interface, das nur einen Prompt sieht (siehe 3.3).
- Der Context Builder liefert in Phase 1 die Token-Messung, die `auto_context` für seine Entscheidung braucht. Ohne diese Messung ist der Modus nicht implementierbar (siehe 6.2).

---

# 3. Schnittstellen-Konzept (Interface Segregation)

## 3.1 LLM-Generierung (`internal/ai/provider`) `[implementiert]`

Die Textgenerierung ist vollständig vom Embedding-Backend entkoppelt. Alle LLM-Backends (Ollama, OpenRouter) implementieren `LLMProvider` sowie optional `StreamingLLMProvider` und `ModelLister`:

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

Embeddings werden **ausschließlich lokal** über `embedding.Embedder` (`OllamaEmbedder` mit `bge-m3`) erzeugt und als Float32-BLOBs direkt in `knowledge.db` (`section_embeddings`) gespeichert:

```go
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Model() string
}
```

### Warum Embeddings zwingend lokal bleiben:

1. **Vektorraum-Konsistenz:** Index-Embeddings (`md-indexer`) und Query-Embeddings (`md-ai`) müssen dasselbe Modell (`bge-m3`) verwenden, unabhängig davon, welches Chat-Modell die Antwort formuliert.
2. **Datenschutz & Kosten:** Das Einbetten des gesamten Vaults verursacht keinen Cloud-Traffic und keine API-Kosten.
3. **Kein separater Vektor-Store nötig:** `section_embeddings` liegt transaktionssicher und referentiell integer in `knowledge.db`.

## 3.3 Warum die Routing-Entscheidung nicht im Provider-Paket liegt `[geplant]`

Ein `RoutingProvider`, der selbst entscheidet, ob lokal oder in der Cloud generiert wird, ist mit dem heutigen Vertrag nicht umsetzbar:

- `model.GenerateRequest` enthält nur `Model`, `Prompt`, `System`, `Temperature`, `MaxTokens` und `ContextWindow`. Es fehlen `QueryType`, `GraphDepth`, der `RetrievalPlan` und die gemessene Kontextgröße — also genau die Größen, aus denen `auto_context` seine Trigger ableitet.
- `md-indexer.sdd` verlangt, dass providerspezifischer Protokollcode ausschließlich in `internal/ai/provider` und `internal/embedding` liegt. Eine Auswahlheuristik ist kein Protokollcode.

**Entscheidung:** Die Ausführungs-Policy lebt auf der Service-Ebene (`internal/ai/assistant` bzw. ein eigener `execution`-Baustein), erhält den `RetrievalResult`-Plan und die Token-Messung und wählt daraus den Provider. `provider` bleibt ein reines Transportpaket. `GenerateRequest` wird nicht um Routing-Metadaten erweitert.

---

# 4. Sicherheitsarchitektur

## 4.1 API-Keys und `.env` `[Blocker]`

Gemäß `md-indexer.sdd` gilt:

- Der OpenRouter-API-Key (`OPENROUTER_API_KEY` bzw. `MD_AI_OPENROUTER_API_KEY`) wird ausschließlich aus der Prozessumgebung oder einer geschützten lokalen `.env`-Datei geladen.
- **Verboten:** Übergabe per CLI-Argument (`--api-key`), Ausgabe in Logs, Rückgabe in HTTP-Responses.
- Gegenüber dem Desktop-Frontend meldet `md-api` lediglich Metadaten (`cloud_available: true/false` sowie konfigurierte Modellnamen).

Vor der ersten Umsetzung zwingend zu erledigen:

- **`.gitignore` erweitern.** Aktuell enthält sie nur `bin/` und `data/`. Eine `.env` im Repo-Root wäre damit commit-fähig. `.env` und `.env.*` müssen ignoriert werden, bevor irgendein Key existiert.
- **Loader existiert nicht.** Im Code gibt es keinen `.env`-Parser (kein `godotenv`, keine eigene Funktion). Zu bauen ist ein Loader mit definierter Präzedenz: `Prozessumgebung > .env > md-ai.yaml > Defaults`. Explizite Umgebung soll immer gewinnen, damit Container- und CI-Betrieb funktioniert.
- **Dateirechte.** `.env` wird mit `0600` angelegt; der Loader warnt, wenn die Datei für Gruppe oder Andere lesbar ist.
- **Ort der Datei.** Nicht hart auf ein Fremd-Repository verdrahten. Vorschlag: `~/.config/md-indexer/.env` plus optionaler Pfad über `MD_AI_ENV_FILE`.

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

`md-api` bietet derzeit nur `-host` und **keine Authentifizierung**. `md-indexer.sdd` verlangt zudem, dass von Client-Apps gestartete Sidecars an `127.0.0.1` binden.

Daraus folgt: Der im Deployment beschriebene Laptop-Online-Modus über WireGuard macht den vollständigen Vault netzwerklesbar — inklusive `/api/ai/ask`, das Kontextblöcke zurückliefert. Das ist kein Deployment-Detail, sondern eine Grenzverletzung. Vor Umsetzung zu entscheiden:

- **Option A (empfohlen):** SSH-Tunnel oder WireGuard plus Bearer-Token mit konstanter Zeitprüfung; `md-api` bindet zusätzlich an die VPN-Schnittstelle, nicht an `0.0.0.0`.
- **Option B:** mTLS zwischen Laptop und Desktop.
- **In jedem Fall:** `md-indexer.sdd` wird im selben Change aktualisiert (Loopback-Regel, Authentifizierungspflicht für nicht-loopback-Bindings). Laut `.specdd/bootstrap.project.md` ist eine Architekturänderung ohne Spec-Update unzulässig.

## 4.4 Prompt Injection `[offen]`

Abgerufene Markdown-Inhalte sind für das LLM **untrusted Input**. Ein Vault-Dokument kann Anweisungen enthalten, die das Modell als Instruktion liest. Bei einem externen Provider wiegt das schwerer, weil dieselbe Anfrage auch Daten exportiert.

Vorzusehen:

- Explizite Instruktionshierarchie im Systemprompt: abgerufener Kontext ist Datenmaterial, niemals Anweisung.
- Kontextblöcke werden klar abgegrenzt und mit ihrer Herkunft gelabelt.
- Keine Werkzeug- oder Aktionsfähigkeit für das Modell in dieser Ausbaustufe.

---

# 5. Provider

## 5.1 Ollama-Provider (Primärer Standard-Provider) `[implementiert]`

Jede Anfrage wird standardmäßig lokal verarbeitet.

Anwendungsfälle:

- Tägliche Wissenssuche, Dokumentation, ADR-Abfragen, Codeanalyse
- Offline-Betrieb und datenschutzkritische Inhalte
- Kostenfreie Ausführung ohne Cloud-Traffic

Klarstellung zu zwei verbreiteten Fehlannahmen:

- **Token-Limits existieren.** Das Kontextfenster ist durch `max_context_tokens` und das Modellfenster begrenzt; der Code dimensioniert `num_ctx` dynamisch in 8192er-Schritten.
- **`temperature: 0.1` ist nicht deterministisch.** Exakt reproduzierbare Läufe erfordern `temperature: 0` plus festen Seed. Für den Alltag ist `0.1` sinnvoll, für Vergleichsmessungen nicht.

### Desktop-Konfiguration (64 GB RAM)

```yaml
execution_mode: local_only
ollama_host: "http://localhost:11434"
model: "qwen3:32b"
embedding_model: "bge-m3"
temperature: 0.1
max_context_tokens: 16000
```

### Laptop-Konfiguration (16 GB RAM)

```yaml
execution_mode: local_only
ollama_host: "http://localhost:11434"
model: "qwen3:8b"
embedding_model: "bge-m3"
temperature: 0.1
max_context_tokens: 4000
```

## 5.2 OpenRouter-Provider (Optionaler Cloud-Provider) `[geplant]`

Wird ausschließlich genutzt, wenn der Anwender dies über den Ausführungsmodus (`auto_context` oder `cloud_only`), per CLI-Flag oder über eine Vergleichsmessung im Frontend aktiviert.

### Konfigurierbare Modellstrategie

Statt hart im Go-Code verdrahteter Modellnamen werden die Modelle deklarativ konfiguriert:

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
- `max_cost_usd_per_request` und `max_cost_usd_per_day` sind Pflichtfelder, nicht optional. Ohne sie ist `cloud_only` ein unkontrollierter Kostenpfad.

---

# 6. Ausführungsmodi

Der Modus ist ein expliziter Parameter der Anfrage und wird nicht aus versteckten Heuristiken abgeleitet.

## 6.1 `local_only` (Standard) `[geplant]`

- Ausschließliche Verarbeitung über die lokale Ollama-Instanz.
- Garantierter Offline-Betrieb; kein Byte verlässt das System.
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
         mit openrouter.max_context_tokens (z. B. 32.000 Tokens)
```

**c) Der GraphDepth-Trigger muss zur Konfiguration passen.** Der Planner kappt jede Tiefe auf `cfg.GraphDepth` (Default 3). Ein Schwellwert `>= 4` feuert damit nie. Entweder wird der Default angehoben, oder der Schwellwert wird auf `>= cfg.GraphDepth` bezogen. Ein Trigger, der nie auslöst, ist schlimmer als keiner, weil er Sicherheit suggeriert.

## 6.3 `cloud_only` `[geplant]`

- Alle LLM-Antworten werden über OpenRouter generiert.
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
     (Ollama)            (OpenRouter)
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
- `prompt_tokens`, `completion_tokens` (aus der Provider-Response; OpenRouter liefert `usage`, Ollama liefert `prompt_eval_count` und `eval_count`)
- `cost_usd`, sofern der Provider Kosten meldet
- `matched_context_tokens` — **existiert im Datenmodell noch nicht.** `model.Answer` und `model.RetrievalResult` haben kein Tokenfeld. Es muss ergänzt werden, nicht nur dokumentiert.
- `query_type`, `graph_depth`, `max_results` aus dem Plan
- `sources` und `gaps` je Arm

## 7.3 Methodik

- **Interaktiver Standard im Frontend (`runs = 1`):** Auf CPU-Systemen (z. B. 16-GB-Laptop mit 1–3 Minuten pro lokalem Lauf) würden 4 Läufe hintereinander die UI für 8–12 Minuten blockieren. Standardmäßig läuft daher **ein** Durchlauf pro Arm, wobei `load_ms` (Kaltstart) separat von `prefill_ms + eval_ms` (reine Inferenzzeit im warmen Zustand) ausgewiesen wird.
- **Optionaler Benchmark-Modus (`runs = 3` mit Median):** Für GPU-/Desktop-Systeme im UI zuschaltbar, um Varianzen über den Median auszugleichen.
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
| **`architecture`** | *„Wie funktioniert die Ingestion Pipeline?“* | `true` | `true` | `true` | `true` | `4` | `15` |
| **`exploration`** | *„Wie schützen wir sensible Daten?“* | `true` | `true` | `true` | `true` | `1` | `12` |

Anmerkung zu `impact` ohne Vektoren: Das ist das heutige Verhalten, keine begründete Entscheidung. Impact-Fragen profitieren typischerweise von semantischem Recall, weil betroffene Komponenten selten dieselben Wörter verwenden. `[offen]` — vor der nächsten Planner-Änderung evaluieren.

## 8.2 Die vier Retrieval-Quellen auf `knowledge.db`

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

Multi-Hop-GraphRAG ist nur so gut wie die Kanten im Vault — und die entstehen heute ausschließlich aus zwei Quellen:

1. **Frontmatter-Listen** in Entitätsdokumenten (`type: component` plus z. B. `uses: [Package Model]`). Der Schlüsselname wird zu `strings.ToUpper(k)`; unbekannte Schlüssel landen auf `WeightOther`.
2. **Wikilinks** `[[Ziel]]`, `[[Ziel|Alias]]`, `[[Ziel#Überschrift]]` in Entitätsdokumenten, die als `LINKS_TO`-Kanten extrahiert werden. Wikilinks außerhalb von Entitätsdokumenten erzeugen nur Dokumentlinks, keine Graphkanten.

Daraus folgen zwei Pflichten, die im Dokument festgehalten werden müssen, weil sonst die Graphqualität autorenabhängig und unvorhersehbar ist:

- **Vault-Konvention dokumentieren:** erlaubte Relationstypen, Schreibweise der Targets, Pflichtfeld `type:`, Namensauflösung über `documents.title` (`COLLATE NOCASE`).
- **Abdeckung messen:** Anteil der Entitätsdokumente mit mindestens einer Relation. Ein Vault ohne Frontmatter-Relationen hat keinen Graphen, nur Wikilinks — und dann tragen `USES`/`DEPENDS_ON`-Gewichte nichts.

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

## 11.1 Desktop (64 GB RAM) – Primärer Knowledge Server

Komponenten:

- `md-indexer`, `knowledge.db` (inkl. FTS5, Graph & Embeddings), `ollama`, `md-ai`, `md-api`, `apps/desktop`
- Lokale Sidecar-Prozesse der Desktop-App binden zwingend an `127.0.0.1`.

Modelle:

- Lokal: `qwen3:32b` (oder `qwen3:14b`), Embeddings: `bge-m3`
- Optional Cloud (OpenRouter) via `.env` für Vergleichsmessungen, `auto_context` oder `cloud_only`.

## 11.2 Laptop (16 GB RAM)

### Online-Modus (über Desktop-Server) `[Blocker]`

```text
Laptop (apps/desktop oder md-ai)
   ↓
WireGuard / SSH-Tunnel
   ↓
md-api (Desktop-Server, authentifiziert)
   ↓
Ollama (qwen3:32b) + knowledge.db
```

**Voraussetzung:** Authentifizierung nach 4.3 und ein Spec-Update in `md-indexer.sdd`. Ohne beides ist dieser Modus nicht zulässig.

### Eigenständiger Betrieb (Lokal + optional `auto_context`)

- Lokal für schnelle Standardfragen: `qwen3:8b`, Embeddings lokal: `bge-m3`
- Bei `execution_mode: auto_context` eskaliert der Service bei überschrittenem Token-Schwellwert oder tiefer Graph-Analyse auf OpenRouter — fail-closed nach 6.4.

## 11.3 Datenbank-Lebenszyklus `[teilweise implementiert]`

Über das Paket `internal/vaultdb` (bereits auf `main` aktiv in `cmd/indexer`, `cmd/md-api` und `sidecar.rs`) wird die Datenbank standardmäßig unter `<vault>/.md_indexer/md_indexer.db` abgelegt, mit automatischem Fallback auf `UserDataDir()`, falls der Vault schreibgeschützt ist, sowie einmaliger `-migrate-from`-Migration.

Zwei Punkte bleiben dabei `[offen]`:

1. **Sync-Tool-Ausschluss:** Da `<vault>/.md_indexer/` innerhalb des Vault-Verzeichnisses liegt, muss dieser Ordner zwingend von Sync-Tools (Obsidian Sync, Syncthing, Nextcloud, iCloud, git) ausgeschlossen werden. Andernfalls führt die Synchronisation der WAL-Dateien (`-wal`/`-shm`) bei gleichzeitigem Zugriff zweier Rechner zu SQLite-Korruption.
2. **Namensvereinheitlichung (`md_indexer.db` vs. `knowledge.db`):** Während `internal/vaultdb` den Dateinamen `md_indexer.db` verwendet, nutzt `config.DefaultConfig()` in `md-ai` ohne `-vault`-Auflösung noch den Default `"knowledge.db"`. `cmd/md-ai` sollte analog zu `cmd/md-api` `vaultdb.ForReader` nutzen.

---

# 12. Betriebsgrenzen `[geplant]`

- **Rate-Limits:** 429 von OpenRouter erzeugt eine verständliche Meldung mit Retry-Hinweis, keinen generischen „Frage fehlgeschlagen“-Fehler.
- **Timeouts:** getrennt pro Provider (`ollama.request_timeout_seconds`, `openrouter.request_timeout_seconds`). Der heutige globale Timeout von 600 s ist auf lokale CPU-Inferenz ausgelegt und für Cloud-Aufrufe ungeeignet.
- **Retry:** begrenzt (max. 2) mit exponentiellem Backoff und ausschließlich beim selben Provider. Ein Retry darf niemals den Provider wechseln.
- **Kosten:** `max_cost_usd_per_request` und `max_cost_usd_per_day` werden vor dem Aufruf geprüft. Überschreitung = Fehler, keine stille Kürzung.
- **Nebenläufigkeit:** Die Desktop-App stellt Fragen seriell; parallele Anfragen an dasselbe Modell konkurrieren um dieselbe GPU/CPU und verzerren Messungen.

---

# 13. Transparenz und Nachweisbarkeit `[geplant]`

Prinzip 4 dieses Dokuments verlangt Transparenz. Sie ist nur nachweisbar, wenn sie im Antwortobjekt steht:

`model.Answer` enthält heute `Question`, `Answer`, `Intent`, `Plan`, `Model`, `Sources`, `Gaps`. Zu ergänzen:

- `provider` (`ollama` | `openrouter`)
- `execution_mode`
- `escalation_reason` (leer bei lokalem Lauf; sonst z. B. `context_tokens>4000`)
- `context_tokens` (genutzt und ungekürzt)
- `cost_usd`, sofern verfügbar

Zusätzlich: ein Log-Eintrag pro Anfrage mit Provider, Modus, Plan, Token-Zahlen und Dauer — ohne Prompt-Inhalt und ohne Secrets. Der bestehende `--retrieve-only`-Modus bleibt das Werkzeug, um Retrieval-Entscheidungen ohne LLM-Aufruf zu prüfen.

---

# 14. Offene Entscheidungen und Blocker

**Blocker (vor jeder Umsetzung zu klären):**

1. `.env`-Loader, Präzedenz und `.gitignore`-Regel. Ohne Ignore-Regel und Loader darf kein Key existieren.
2. Authentifizierung für remote `md-api`: Bearer-Token oder mTLS, plus Spec-Update in `md-indexer.sdd`.

**Offene Designentscheidungen:**

3. `BuildResult` mit `TotalTokens` und `Truncated` — Signaturänderung in `internal/ai/contextbuilder`.
4. Zweiphasige Context-Budgetierung für `auto_context` — `RetrievalResult` hält `cloud_max_results` Kandidaten vor; `ContextBuilder` budgetiert bei Eskalation mit `openrouter.max_context_tokens` nach (ohne zweites DB-/Embedding-Retrieval).
5. GraphDepth-Schwellwert gegen die Planner-Kappung — Default anheben oder Schwelle relativ zu `cfg.GraphDepth` setzen.
6. Fail-closed bei Ollama-Ausfall — bestätigen, dass keine Eskalation stattfindet.
7. Embedding-Modellwechsel-Warnung für CLI/Standalone (`md-ai` / `md-api`) — prüfen, ob für `cfg.EmbeddingModel` Einträge in `section_embeddings` vorliegen (in der Desktop-App bereits durch den vorgeschalteten `md-indexer`-Lauf gelöst).
8. `impact` ohne Vektoren — Planner evaluieren.
9. `break` vs. `continue` im Token-Budget — Verhalten festlegen und dokumentieren.
10. Sync-Ausschluss für `<vault>/.md_indexer/` dokumentieren und `vaultdb.ForReader` auch in `cmd/md-ai` als Standard verankern (`md_indexer.db` statt abweichendem `"knowledge.db"`-Default).

**Geplant, aber noch nicht vorhanden:**

11. `POST /api/ai/compare` inklusive Schema in `openapi.yaml` (Standard `runs = 1` mit separater Ausweisung von Ollama `load_ms` / `prefill_ms` / `eval_ms`; optional `runs = 3` mit Median).
12. Token- und Kostenfelder in `model.Answer` (`context_tokens`, `cost_usd`, `provider`, `execution_mode`, `escalation_reason`).

---

# 15. Architekturprinzipien

1. **Markdown bleibt die einzige Source of Truth** (strikter Read-Only-Zugriff auf den Vault).
2. **Eine einzige SQLite-Datenbank** ist die alleinige, jederzeit neu aufbaubare Persistenzschicht für Volltext, Graph und Vektoren.
3. **Embeddings bleiben immer lokal (`bge-m3`)**, um Vektorraum-Konsistenz und Datenschutz zu garantieren.
4. **Lokale Verarbeitung (`local_only`) ist der Standard.** Cloud-Nutzung erfolgt ausschließlich nach expliziter Wahl (`local_only`, `auto_context`, `cloud_only`) und ist im Antwortobjekt sichtbar.
5. **Fail-closed statt stiller Fallback.** Kein Providerwechsel bei Fehlern, keine Eskalation bei lokalem Ausfall, kein stiller Rückfall von Cloud auf lokal.
6. **Strikte Geheimnishygiene:** API-Keys ausschließlich in einer geschützten `.env`; alle Kontexte durchlaufen `RedactSecrets`. Redaction ist Best-Effort und ersetzt keine Zustimmung.
7. **Saubere Interface-Trennung:** `provider.LLMProvider` (Generierung) und `embedding.Embedder` (Vektorisierung) sind entkoppelt; die Ausführungs-Policy liegt darüber, nicht darin.
8. **Messen statt Raten:** Die Vergleichsmessung liefert Dauer (inkl. Trennung von Kaltstart `load_ms` und Inferenz), Token, Kosten und Qualität als getrennte Metriken.
9. **Grounded Answers:** Das LLM antwortet ausschließlich auf Basis des bereitgestellten Kontexts und weist Quellen (`Sources`) sowie erkannte Wissenslücken (`KnowledgeGaps`) deterministisch aus.
10. **Nachvollziehbarkeit:** Jede Antwort nennt Provider, Ausführungsmodus, Kontextumfang und — bei Eskalation — den Grund.

---

# 16. Dokument-Governance

Dieses Dokument (`docs/architektur_erweiterung.md`) dient als getrackte Zielbild-Spezifikation (RFC) für die Hybrid-Cloud-Erweiterung. Die Referenz `docs/Hybrid_GraphRAG_DE.md` beschreibt den bereits implementierten lokalen Retriever-Kern und ist im Bootstrap (`.specdd/bootstrap.project.md`) als Einstiegspunkt referenziert.

Damit dieselbe Architektur nicht dauerhaft in zwei Dokumenten parallel gepflegt werden muss, gilt folgende Aufteilung:

- **`docs/architektur_erweiterung.md` (dieses Dokument):** Spezifiziert das Delta (OpenRouter-Provider hinter `LLMProvider`, Ausführungs-Policy auf Service-Ebene, Vergleichsmessung `/api/ai/compare`, Sicherheits- und Betriebsgrenzen).
- **`docs/Hybrid_GraphRAG_DE.md`:** Bleibt die Referenz des implementierten Retrievers und wird bei Umsetzung um den Graph-Extraktionsvertrag (8.3), den Embedding-Lebenszyklus (10) und die explizite RRF-Formel (8.4) ergänzt.
- **`md-indexer.sdd`:** Wird im selben Change aktualisiert, sobald ein Cloud-Provider oder ein nicht-loopback-Binding umgesetzt wird (Provider-Regel, Redaction-Pflicht vor externen Providern, Authentifizierungspflicht).

