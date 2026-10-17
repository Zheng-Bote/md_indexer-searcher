// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"os"
	"strings"
	"testing"
)

func TestParseFile_EntityAndRelations(t *testing.T) {
	content := `---
type: component
name: AuthGateway
owner: security-team
tier: 1
uses:
  - TokenService
  - UserDB
---
# Authentication Gateway
The gateway handles incoming requests and validates them with [[TokenService]].
It also stores auditing records in [[AuditLogger]] and adheres to #security and #compliance.
`
	f, err := os.CreateTemp("", "*.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	f.Close()

	p := NewParser()
	doc, err := p.ParseFile(f.Name())
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	// 1. Entity attributes
	if !doc.IsEntity {
		t.Fatalf("Expected doc.IsEntity to be true")
	}
	if doc.EntityType != "component" {
		t.Errorf("Expected EntityType 'component', got '%s'", doc.EntityType)
	}
	if doc.EntityName != "AuthGateway" {
		t.Errorf("Expected EntityName 'AuthGateway', got '%s'", doc.EntityName)
	}

	// 2. Scalar properties
	if doc.EntityProperties["owner"] != "security-team" {
		t.Errorf("Expected owner 'security-team', got '%s'", doc.EntityProperties["owner"])
	}
	if doc.EntityProperties["tier"] != "1" {
		t.Errorf("Expected tier '1', got '%s'", doc.EntityProperties["tier"])
	}

	// 3. Frontmatter relations
	hasUsesToken := false
	hasUsesDB := false
	for _, rel := range doc.EntityRelations {
		if rel.SourceID == "AuthGateway" && rel.TargetID == "TokenService" && rel.Type == "USES" {
			hasUsesToken = true
		}
		if rel.SourceID == "AuthGateway" && rel.TargetID == "UserDB" && rel.Type == "USES" {
			hasUsesDB = true
		}
	}
	if !hasUsesToken {
		t.Errorf("Expected EntityRelation USES TokenService, got %+v", doc.EntityRelations)
	}
	if !hasUsesDB {
		t.Errorf("Expected EntityRelation USES UserDB, got %+v", doc.EntityRelations)
	}

	// 4. Wikilinks & LINKS_TO relations
	hasLinkToken := false
	hasLinkAudit := false
	for _, link := range doc.Links {
		if link.TargetDocID == "TokenService" {
			hasLinkToken = true
		}
		if link.TargetDocID == "AuditLogger" {
			hasLinkAudit = true
		}
	}
	if !hasLinkToken {
		t.Errorf("Expected Wikilink 'TokenService' in doc.Links, got %+v", doc.Links)
	}
	if !hasLinkAudit {
		t.Errorf("Expected Wikilink 'AuditLogger' in doc.Links, got %+v", doc.Links)
	}

	hasRelLinksAudit := false
	for _, rel := range doc.EntityRelations {
		if rel.TargetID == "AuditLogger" && rel.Type == "LINKS_TO" {
			hasRelLinksAudit = true
			break
		}
	}
	if !hasRelLinksAudit {
		t.Errorf("Expected LINKS_TO relation for AuditLogger, got %+v", doc.EntityRelations)
	}

	// 5. Hashtags
	hasSecurityTag := false
	hasComplianceTag := false
	for _, tag := range doc.Tags {
		if tag == "security" {
			hasSecurityTag = true
		}
		if tag == "compliance" {
			hasComplianceTag = true
		}
	}
	if !hasSecurityTag || !hasComplianceTag {
		t.Errorf("Expected tags 'security' and 'compliance', got %+v", doc.Tags)
	}
}

func TestParseFile_NonEntityDocument(t *testing.T) {
	content := `# Regular Note
Just a standard markdown note linking to [[ArchitectureGuide]].
`
	f, err := os.CreateTemp("", "*.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	f.WriteString(content)
	f.Close()

	p := NewParser()
	doc, err := p.ParseFile(f.Name())
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if doc.IsEntity {
		t.Errorf("Expected doc.IsEntity to be false")
	}
	if len(doc.Links) != 1 || doc.Links[0].TargetDocID != "ArchitectureGuide" {
		t.Errorf("Expected 1 link to ArchitectureGuide, got %+v", doc.Links)
	}
	if len(doc.EntityRelations) != 0 {
		t.Errorf("Expected 0 entity relations for non-entity doc, got %+v", doc.EntityRelations)
	}
}

func TestParseFile_LeadSectionAndRichBlocks(t *testing.T) {
	content := `This is introductory lead text before any heading is written.
It provides high-level context.

# Implementation Details
Here is some code:
` + "```go\nfunc StartEngine() bool {\n    return true\n}\n```" + `

Key components:
- Engine Controller
- [[TelemetryService]] collector
`
	f, err := os.CreateTemp("", "*.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	f.WriteString(content)
	f.Close()

	p := NewParser()
	doc, err := p.ParseFile(f.Name())
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if len(doc.Sections) != 2 {
		t.Fatalf("Expected 2 sections (Overview + Implementation Details), got %d: %+v", len(doc.Sections), doc.Sections)
	}

	// 1. Overview section check
	overview := doc.Sections[0]
	if overview.Heading != "Overview" {
		t.Errorf("Expected first section heading 'Overview', got '%s'", overview.Heading)
	}
	if !strings.Contains(overview.Content, "introductory lead text before any heading") {
		t.Errorf("Expected overview content to contain lead text, got '%s'", overview.Content)
	}

	// 2. Implementation Details section check
	impl := doc.Sections[1]
	if impl.Heading != "Implementation Details" {
		t.Errorf("Expected section heading 'Implementation Details', got '%s'", impl.Heading)
	}
	if !strings.Contains(impl.Content, "StartEngine") {
		t.Errorf("Expected code block text 'StartEngine' in section content, got '%s'", impl.Content)
	}
	if !strings.Contains(impl.Content, "Engine Controller") {
		t.Errorf("Expected list item 'Engine Controller' in section content, got '%s'", impl.Content)
	}

	// 3. Wikilink inside list item
	hasTelemetry := false
	for _, l := range doc.Links {
		if l.TargetDocID == "TelemetryService" {
			hasTelemetry = true
			break
		}
	}
	if !hasTelemetry {
		t.Errorf("Expected wikilink to 'TelemetryService' from list item, got %+v", doc.Links)
	}

	// 4. Combined doc.Content
	if !strings.Contains(doc.Content, "Overview") || !strings.Contains(doc.Content, "Implementation Details") {
		t.Errorf("Expected doc.Content to combine all sections, got '%s'", doc.Content)
	}
}

func TestParseFile_LinkNormalizationAndCodeBlockExclusion(t *testing.T) {
	content := `---
type: service
name: SensorHub
uses:
  - "[[MQTT Broker]]"
  - "[[folder/InfluxDB#Write|Influx]]"
---
# Overview
Connects to [[MQTT Broker|Broker Alias]] and [[Zigbee2MQTT#Pairing]] with #iot and #123.
Inline code like ` + "`[[which treefrog]] #define`" + ` must be ignored.

` + "```c\n#include <stdio.h>\n#define MAX 10\n// [[FakeCodeLink]]\n```\n"

	f, err := os.CreateTemp("", "*.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	f.WriteString(content)
	f.Close()

	doc, err := NewParser().ParseFile(f.Name())
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	// Frontmatter relations must strip [[ ]], path, #heading, and |alias.
	wantRels := map[string]bool{"MQTT Broker": false, "InfluxDB": false}
	for _, r := range doc.EntityRelations {
		if r.Type == "USES" {
			if _, ok := wantRels[r.TargetID]; ok {
				wantRels[r.TargetID] = true
			} else {
				t.Errorf("unexpected USES relation target %q", r.TargetID)
			}
		}
	}
	for target, seen := range wantRels {
		if !seen {
			t.Errorf("missing normalized USES relation to %q, got %+v", target, doc.EntityRelations)
		}
	}

	// Inline links must normalize aliases/headings and ignore code blocks & inline code.
	gotLinks := make(map[string]bool)
	for _, l := range doc.Links {
		gotLinks[l.TargetDocID] = true
	}
	if !gotLinks["MQTT Broker"] || !gotLinks["Zigbee2MQTT"] {
		t.Errorf("expected normalized links to MQTT Broker and Zigbee2MQTT, got %+v", doc.Links)
	}
	if gotLinks["which treefrog"] || gotLinks["FakeCodeLink"] {
		t.Errorf("links inside inline code or fenced code blocks must be ignored, got %+v", doc.Links)
	}

	// Tags must include #iot, but ignore #include, #define, and numeric #123.
	if len(doc.Tags) != 1 || doc.Tags[0] != "iot" {
		t.Errorf("expected only tag 'iot', got %+v", doc.Tags)
	}
}
