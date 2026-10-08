# Feature Spec: Graph Traversal (Recursive CTEs)

## 1. Problem & Goal
The `GraphEngine` currently returns `nil` for all graph traversals (`DirectRelations`, `DependencyTree`, `ImpactAnalysis`, `ShortestPath`). 
To resolve entity dependencies and perform impact analysis without N+1 query problems in Go, we will implement these functions using SQLite's `WITH RECURSIVE` Common Table Expressions (CTEs).

## 2. Scope & Acceptance Criteria
*   **DirectRelations:** Returns 1st-degree outgoing relations for an entity.
*   **ReverseRelations:** Returns 1st-degree incoming relations for an entity.
*   **DependencyTree:** Returns a hierarchical tree (up to a max depth) of all outgoing relations.
*   **ImpactAnalysis:** Returns a hierarchical tree (up to a max depth) of all incoming relations (who depends on this?).
*   **ShortestPath:** Computes the shortest path between two entities using BFS inside SQLite.
*   Must strictly use SQLite, adhering to `.sdd` rules.

## 3. Architecture Plan
*   **Target:** `internal/graph/engine.go`
*   No changes to `md-indexer` write-path are needed.
*   We'll use standard SQL `WITH RECURSIVE` queries over `entity_relations` and `entities` tables.

## 4. Tasks
*   [ ] Task 1: Implement `DirectRelations` and `ReverseRelations` in `internal/graph/engine.go`.
*   [ ] Task 2: Implement `DependencyTree` using `WITH RECURSIVE`.
*   [ ] Task 3: Implement `ImpactAnalysis` using `WITH RECURSIVE`.
*   [ ] Task 4: Implement `ShortestPath` using `WITH RECURSIVE` breadth-first search.
