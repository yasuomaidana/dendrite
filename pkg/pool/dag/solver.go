package dag

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// DefaultPoolConfigFile is the default relative path for pool configuration.
const DefaultPoolConfigFile = ".dendrite/pool.json"

// PoolManager manages registration and persistent storage of project pool members.
type PoolManager struct {
	mu         sync.RWMutex
	configFile string
	scanner    *ManifestScanner
	config     PoolConfig
}

// NewPoolManager initializes a pool manager pointing to a specified or default config file.
func NewPoolManager(configFile string) *PoolManager {
	if configFile == "" {
		configFile = DefaultPoolConfigFile
	}
	pm := &PoolManager{
		configFile: configFile,
		scanner:    NewManifestScanner(),
		config: PoolConfig{
			Version:  "1.0.0",
			Projects: []ProjectInfo{},
		},
	}
	_ = pm.Load()
	return pm
}

// Load reads the pool configuration from disk if it exists.
func (pm *PoolManager) Load() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if !exists(pm.configFile) {
		return nil
	}

	data, err := os.ReadFile(pm.configFile)
	if err != nil {
		return fmt.Errorf("reading pool config: %w", err)
	}

	var cfg PoolConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("%w: %v", ErrManifestMalformed, err)
	}

	pm.config = cfg
	return nil
}

// Save writes the current pool configuration atomically to disk.
func (pm *PoolManager) Save() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	dir := filepath.Dir(pm.configFile)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating config directory: %w", err)
		}
	}

	pm.config.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(pm.config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling pool config: %w", err)
	}

	tmpFile := pm.configFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("writing temp config: %w", err)
	}

	if err := os.Rename(tmpFile, pm.configFile); err != nil {
		return fmt.Errorf("committing config file: %w", err)
	}

	return nil
}

// Add scans and registers directories into the pool.
func (pm *PoolManager) Add(dirs ...string) ([]ProjectInfo, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	existingPaths := make(map[string]bool)
	existingNames := make(map[string]bool)
	for _, p := range pm.config.Projects {
		existingPaths[p.Path] = true
		existingNames[p.Name] = true
	}

	var added []ProjectInfo
	for _, dir := range dirs {
		info, err := pm.scanner.ScanProject(dir)
		if err != nil {
			return nil, fmt.Errorf("scanning %s: %w", dir, err)
		}

		if existingPaths[info.Path] {
			return nil, fmt.Errorf("%w: path %s", ErrDuplicateProject, info.Path)
		}
		if existingNames[info.Name] {
			return nil, fmt.Errorf("%w: name %s", ErrDuplicateProject, info.Name)
		}

		existingPaths[info.Path] = true
		existingNames[info.Name] = true
		pm.config.Projects = append(pm.config.Projects, *info)
		added = append(added, *info)
	}

	return added, nil
}

// Remove unregisters a project by name or path.
func (pm *PoolManager) Remove(nameOrPath string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	idx := -1
	for i, p := range pm.config.Projects {
		if p.Name == nameOrPath || p.Path == nameOrPath || filepath.Base(p.Path) == nameOrPath {
			idx = i
			break
		}
	}

	if idx == -1 {
		return fmt.Errorf("%w: %s", ErrProjectNotFound, nameOrPath)
	}

	pm.config.Projects = append(pm.config.Projects[:idx], pm.config.Projects[idx+1:]...)
	return nil
}

// List returns a copy of all registered project infos.
func (pm *PoolManager) List() []ProjectInfo {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	res := make([]ProjectInfo, len(pm.config.Projects))
	copy(res, pm.config.Projects)
	return res
}

// DAGSolver computes deterministic dependency resolutions and detects cycles.
type DAGSolver struct{}

// NewDAGSolver creates a new DAG solver instance.
func NewDAGSolver() *DAGSolver {
	return &DAGSolver{}
}

// Resolve analyzes projects in the pool and returns a validated topological resolution.
func (s *DAGSolver) Resolve(projects []ProjectInfo) (*DAGResolution, error) {
	if len(projects) == 0 {
		return nil, ErrEmptyPool
	}

	// 1. Build lookup tables for project identification
	nameToID := make(map[string]string)
	nodes := make(map[string]*DAGNode)

	for _, p := range projects {
		node := &DAGNode{
			ID:          p.Name,
			Path:        p.Path,
			Ecosystems:  p.Ecosystems,
			DependsOn:   []string{},
			DependedBy:  []string{},
		}
		nodes[p.Name] = node
		nameToID[p.Name] = p.Name
		nameToID[filepath.Base(p.Path)] = p.Name
		nameToID[filepath.Base(p.Name)] = p.Name
	}

	// 2. Discover directed edges based on manifest dependencies
	var edges []DependencyEdge
	for _, p := range projects {
		for _, dep := range p.Dependencies {
			targetID, found := nameToID[dep]
			if !found {
				// Try matching base name of dependency
				targetID, found = nameToID[filepath.Base(dep)]
			}

			if found {
				// Project p depends on targetID
				eco := EcosystemUnknown
				if len(p.Ecosystems) > 0 {
					eco = p.Ecosystems[0]
				}

				// Check self-dependency
				if p.Name == targetID {
					return nil, &CyclicDependencyError{Cycle: []string{p.Name, targetID}}
				}

				if !contains(nodes[p.Name].DependsOn, targetID) {
					nodes[p.Name].DependsOn = append(nodes[p.Name].DependsOn, targetID)
					nodes[targetID].DependedBy = append(nodes[targetID].DependedBy, p.Name)
					edges = append(edges, DependencyEdge{
						From:      p.Name,
						To:        targetID,
						Ecosystem: eco,
					})
				}
			}
		}
	}

	// 3. Detect cycles using DFS with 3-color marking
	if cycle := detectCycle(nodes); len(cycle) > 0 {
		return nil, &CyclicDependencyError{Cycle: cycle}
	}

	// 4. Compute deterministic topological build order (Kahn's algorithm)
	topoOrder := computeDeterministicTopoOrder(nodes)

	// 5. Categorize root and leaf nodes
	var leafNodes, rootNodes []string
	for id, node := range nodes {
		if len(node.DependsOn) == 0 {
			leafNodes = append(leafNodes, id)
		}
		if len(node.DependedBy) == 0 {
			rootNodes = append(rootNodes, id)
		}
	}
	sort.Strings(leafNodes)
	sort.Strings(rootNodes)

	return &DAGResolution{
		Nodes:            nodes,
		TopologicalOrder: topoOrder,
		Edges:            edges,
		LeafNodes:        leafNodes,
		RootNodes:        rootNodes,
	}, nil
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// detectCycle runs DFS cycle detection returning the cycle path if found.
func detectCycle(nodes map[string]*DAGNode) []string {
	const (
		white = 0 // unvisited
		gray  = 1 // visiting
		black = 2 // visited
	)

	color := make(map[string]int)
	parent := make(map[string]string)

	var cyclePath []string

	// Sort keys for deterministic cycle reporting
	var keys []string
	for k := range nodes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var dfs func(u string) bool
	dfs = func(u string) bool {
		color[u] = gray

		var neighbors []string
		neighbors = append(neighbors, nodes[u].DependsOn...)
		sort.Strings(neighbors)

		for _, v := range neighbors {
			if color[v] == gray {
				// Cycle detected, reconstruct path
				cyclePath = []string{v, u}
				curr := u
				for curr != v && parent[curr] != "" {
					curr = parent[curr]
					cyclePath = append(cyclePath, curr)
				}
				// Reverse cyclePath
				for i, j := 0, len(cyclePath)-1; i < j; i, j = i+1, j-1 {
					cyclePath[i], cyclePath[j] = cyclePath[j], cyclePath[i]
				}
				return true
			}
			if color[v] == white {
				parent[v] = u
				if dfs(v) {
					return true
				}
			}
		}

		color[u] = black
		return false
	}

	for _, k := range keys {
		if color[k] == white {
			if dfs(k) {
				return cyclePath
			}
		}
	}

	return nil
}

// computeDeterministicTopoOrder sorts nodes so dependencies appear before dependents.
func computeDeterministicTopoOrder(nodes map[string]*DAGNode) []string {
	// inDegree counts how many unresolved dependencies each node still has
	inDegree := make(map[string]int)
	for id, node := range nodes {
		inDegree[id] = len(node.DependsOn)
	}

	// Ready queue contains nodes with 0 remaining dependencies
	var ready []string
	for id, deg := range inDegree {
		if deg == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)

	var order []string
	for len(ready) > 0 {
		curr := ready[0]
		ready = ready[1:]
		order = append(order, curr)

		// For each dependent that requires curr, decrement inDegree
		var dependents []string
		dependents = append(dependents, nodes[curr].DependedBy...)
		sort.Strings(dependents)

		for _, depID := range dependents {
			inDegree[depID]--
			if inDegree[depID] == 0 {
				ready = append(ready, depID)
				sort.Strings(ready)
			}
		}
	}

	return order
}
