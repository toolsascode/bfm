package registry

import (
	"fmt"
	"sort"
	"strings"

	"bfm/api/internal/backends"
	"bfm/api/internal/state"
)

// MigrationNode represents a node in the dependency graph
type MigrationNode struct {
	Migration *backends.MigrationScript
	ID        string
	InDegree  int
	Visited   bool
}

// DependencyGraph represents a graph of migration dependencies
type DependencyGraph struct {
	nodes map[string]*MigrationNode
	edges map[string][]string // from -> to (dependencies)
}

// NewDependencyGraph creates a new dependency graph
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		nodes: make(map[string]*MigrationNode),
		edges: make(map[string][]string),
	}
}

// AddNode adds a migration node to the graph
func (g *DependencyGraph) AddNode(migration *backends.MigrationScript, migrationID string) {
	if _, exists := g.nodes[migrationID]; !exists {
		g.nodes[migrationID] = &MigrationNode{
			Migration: migration,
			ID:        migrationID,
			InDegree:  0,
			Visited:   false,
		}
		g.edges[migrationID] = []string{}
	}
}

// AddEdge adds a dependency edge from 'from' to 'to' (from depends on to)
// This means 'to' must execute before 'from'
func (g *DependencyGraph) AddEdge(from, to string) {
	if _, exists := g.nodes[from]; !exists {
		return
	}
	if _, exists := g.nodes[to]; !exists {
		return
	}
	g.edges[from] = append(g.edges[from], to)
	// In-degree will be calculated during topological sort
}

// DetectCycles detects cycles in the dependency graph using DFS
func (g *DependencyGraph) DetectCycles() ([]string, error) {
	// Reset visited state
	for _, node := range g.nodes {
		node.Visited = false
	}

	// Track nodes in current path for cycle detection
	path := make(map[string]bool)
	cyclePath := []string{}

	var dfs func(nodeID string) bool
	dfs = func(nodeID string) bool {
		node := g.nodes[nodeID]
		if node.Visited {
			return false
		}

		if path[nodeID] {
			// Found a cycle
			cyclePath = append(cyclePath, nodeID)
			return true
		}

		path[nodeID] = true
		for _, depID := range g.edges[nodeID] {
			if dfs(depID) {
				cyclePath = append(cyclePath, nodeID)
				return true
			}
		}
		delete(path, nodeID)
		node.Visited = true
		return false
	}

	for nodeID := range g.nodes {
		if !g.nodes[nodeID].Visited {
			if dfs(nodeID) {
				// Reverse cycle path to show correct order
				for i, j := 0, len(cyclePath)-1; i < j; i, j = i+1, j-1 {
					cyclePath[i], cyclePath[j] = cyclePath[j], cyclePath[i]
				}
				return cyclePath, fmt.Errorf("circular dependency detected: %s", strings.Join(cyclePath, " -> "))
			}
		}
	}

	return nil, nil
}

// TopologicalSort performs topological sort using Kahn's algorithm
func (g *DependencyGraph) TopologicalSort() ([]*backends.MigrationScript, error) {
	// Check for cycles first
	cyclePath, err := g.DetectCycles()
	if err != nil {
		return nil, fmt.Errorf("cycle detected: %v", err)
	}
	if len(cyclePath) > 0 {
		return nil, fmt.Errorf("circular dependency: %s", strings.Join(cyclePath, " -> "))
	}

	// Build reverse graph to calculate in-degree correctly
	// edges[from] = [to1, to2] means "from depends on to1 and to2"
	// In-degree of a node = number of dependencies it has = number of edges FROM it
	// So in-degree of "from" is len(edges[from])
	reverseEdges := make(map[string][]string) // to -> from (dependents)
	for from, toList := range g.edges {
		for _, to := range toList {
			reverseEdges[to] = append(reverseEdges[to], from)
		}
	}

	// Calculate in-degree: number of dependencies this node has
	// In-degree = number of edges FROM this node (how many things it depends on)
	for nodeID := range g.nodes {
		g.nodes[nodeID].InDegree = len(g.edges[nodeID])
	}

	// Find all nodes with in-degree 0 (no dependencies)
	queue := []string{}
	for nodeID, node := range g.nodes {
		if node.InDegree == 0 {
			queue = append(queue, nodeID)
		}
	}

	// Sort initial queue by version for deterministic ordering
	sort.Slice(queue, func(i, j int) bool {
		return g.nodes[queue[i]].Migration.Version < g.nodes[queue[j]].Migration.Version
	})

	sorted := []*backends.MigrationScript{}
	processed := make(map[string]bool)

	// Process queue
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		if processed[currentID] {
			continue
		}

		processed[currentID] = true
		sorted = append(sorted, g.nodes[currentID].Migration)

		// Reduce in-degree of nodes that depend on current node
		for _, dependentID := range reverseEdges[currentID] {
			if g.nodes[dependentID] != nil {
				g.nodes[dependentID].InDegree--
				if g.nodes[dependentID].InDegree == 0 && !processed[dependentID] {
					queue = append(queue, dependentID)
				}
			}
		}

		// Sort queue by version before next iteration
		sort.Slice(queue, func(i, j int) bool {
			return g.nodes[queue[i]].Migration.Version < g.nodes[queue[j]].Migration.Version
		})
	}

	// Check if all nodes were processed
	if len(sorted) < len(g.nodes) {
		var unprocessed []string
		for nodeID := range g.nodes {
			if !processed[nodeID] {
				unprocessed = append(unprocessed, nodeID)
			}
		}
		return nil, fmt.Errorf("not all migrations could be sorted (possible cycle): %s", strings.Join(unprocessed, ", "))
	}

	return sorted, nil
}

// DependencyResolver resolves migration dependencies and provides ordering
type DependencyResolver struct {
	registry     Registry
	stateTracker state.StateTracker
}

// NewDependencyResolver creates a new dependency resolver
func NewDependencyResolver(reg Registry, tracker state.StateTracker) *DependencyResolver {
	return &DependencyResolver{
		registry:     reg,
		stateTracker: tracker,
	}
}

// findDependencyTarget finds migration(s) matching a dependency specification
func (r *DependencyResolver) findDependencyTarget(dep backends.Dependency) ([]*backends.MigrationScript, error) {
	var candidates []*backends.MigrationScript

	// Get all migrations
	allMigrations := r.registry.GetAll()

	// Filter by target type
	for _, migration := range allMigrations {
		// Match connection if specified
		if dep.Connection != "" && migration.Connection != dep.Connection {
			continue
		}

		// Match schema if specified
		if dep.Schema != "" && migration.Schema != dep.Schema {
			continue
		}

		// Match target based on type
		if dep.TargetType == "version" {
			if migration.Version == dep.Target {
				candidates = append(candidates, migration)
			}
		} else {
			// Default to "name"
			if migration.Name == dep.Target {
				candidates = append(candidates, migration)
			}
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("dependency target not found: connection=%s, schema=%s, target=%s, type=%s",
			dep.Connection, dep.Schema, dep.Target, dep.TargetType)
	}

	return candidates, nil
}

// buildDependencyGraph builds a dependency graph from migrations
func (r *DependencyResolver) buildDependencyGraph(migrations []*backends.MigrationScript, getMigrationID func(*backends.MigrationScript) string) (*DependencyGraph, []string) {
	graph := NewDependencyGraph()
	var missingDeps []string

	// Add all migrations as nodes
	for _, migration := range migrations {
		migrationID := getMigrationID(migration)
		graph.AddNode(migration, migrationID)
	}

	// Process structured dependencies
	for _, migration := range migrations {
		migrationID := getMigrationID(migration)

		// Process structured dependencies
		for _, dep := range migration.StructuredDependencies {
			targetMigrations, err := r.findDependencyTarget(dep)
			if err != nil {
				missingDeps = append(missingDeps, fmt.Sprintf("%s depends on %s (not found)", migrationID, err.Error()))
				continue
			}

			// Add edges for each target migration that's in our set
			for _, targetMigration := range targetMigrations {
				targetID := getMigrationID(targetMigration)
				// Only add edge if target is in our current migration set
				if _, exists := graph.nodes[targetID]; exists {
					graph.AddEdge(migrationID, targetID)
				}
			}
		}

		// Process simple string dependencies (backward compatibility)
		for _, depName := range migration.Dependencies {
			// Find migrations by name
			targetMigrations := r.registry.GetMigrationByName(depName)
			if len(targetMigrations) == 0 {
				missingDeps = append(missingDeps, fmt.Sprintf("%s depends on %s (not found)", migrationID, depName))
				continue
			}

			// Add edges for each target migration that's in our set
			for _, targetMigration := range targetMigrations {
				targetID := getMigrationID(targetMigration)
				if _, exists := graph.nodes[targetID]; exists {
					graph.AddEdge(migrationID, targetID)
				}
			}
		}
	}

	return graph, missingDeps
}

// validateDependencyTargets ensures all dependency targets exist
func (r *DependencyResolver) validateDependencyTargets(migrations []*backends.MigrationScript) []string {
	var errors []string

	for _, migration := range migrations {
		// Validate structured dependencies
		for _, dep := range migration.StructuredDependencies {
			_, err := r.findDependencyTarget(dep)
			if err != nil {
				errors = append(errors, fmt.Sprintf("migration %s_%s: %v", migration.Version, migration.Name, err))
			}
		}

		// Validate simple dependencies
		for _, depName := range migration.Dependencies {
			targets := r.registry.GetMigrationByName(depName)
			if len(targets) == 0 {
				errors = append(errors, fmt.Sprintf("migration %s_%s: dependency '%s' not found", migration.Version, migration.Name, depName))
			}
		}
	}

	return errors
}

// ResolveDependencies resolves all dependencies and returns ordered list of migrations
func (r *DependencyResolver) ResolveDependencies(migrations []*backends.MigrationScript, getMigrationID func(*backends.MigrationScript) string) ([]*backends.MigrationScript, error) {
	if len(migrations) == 0 {
		return migrations, nil
	}

	// Validate dependency targets exist
	validationErrors := r.validateDependencyTargets(migrations)
	if len(validationErrors) > 0 {
		return nil, fmt.Errorf("dependency validation failed: %s", strings.Join(validationErrors, "; "))
	}

	// Build dependency graph
	graph, missingDeps := r.buildDependencyGraph(migrations, getMigrationID)
	if len(missingDeps) > 0 {
		return nil, fmt.Errorf("missing dependencies: %s", strings.Join(missingDeps, "; "))
	}

	// Perform topological sort
	sorted, err := graph.TopologicalSort()
	if err != nil {
		return nil, err
	}

	return sorted, nil
}
