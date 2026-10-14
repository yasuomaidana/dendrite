package distcodebase

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// Registry manages distributed codebase sources, library catalogs, and priority resolution.
type Registry struct {
	mu        sync.RWMutex
	sources   map[string]CodebaseSource
	libraries map[string]LibraryEntry
}

// NewRegistry initializes an empty distributed codebase registry.
func NewRegistry() *Registry {
	return &Registry{
		sources:   make(map[string]CodebaseSource),
		libraries: make(map[string]LibraryEntry),
	}
}

// NewDefaultRegistry creates a registry pre-loaded with required ecosystem sources and libraries.
func NewDefaultRegistry() *Registry {
	r := NewRegistry()
	r.initDefaults()
	return r
}

func (r *Registry) initDefaults() {
	now := time.Now()

	// Priority 1: User's / Organization's Own Repositories (Highest Priority)
	ownSource := CodebaseSource{
		ID:          "src-own-yasuomaidana",
		Name:        "yasuomaidana-repositories",
		Type:        SourceTypeOwnRepo,
		Priority:    1,
		URL:         "https://github.com/yasuomaidana",
		Languages:   []string{"rust", "go", "typescript", "python"},
		Description: "User's primary repositories and internal tooling (Priority 1)",
		CreatedAt:   now,
	}
	_ = r.AddSource(ownSource)

	// User-owned projects & libraries
	ownedLibs := []LibraryEntry{
		{
			Name:            "Docula",
			SourceID:        ownSource.ID,
			RepositoryURL:   "https://github.com/yasuomaidana/docula",
			RegistryPackage: "docula",
			Language:        "rust",
			Description:     "Documentation reflection and AST translation engine",
			IsOwned:         true,
			Priority:        1,
			Capabilities:    []string{"docs", "ast", "markdown", "rust-translation", "polyglot"},
			Version:         "0.4.0",
		},
		{
			Name:            "semdag",
			SourceID:        ownSource.ID,
			RepositoryURL:   "https://github.com/yasuomaidana/semdag",
			RegistryPackage: "semdag",
			Language:        "rust",
			Description:     "Semantic directed acyclic graph and dependency engine",
			IsOwned:         true,
			Priority:        1,
			Capabilities:    []string{"dag", "graph", "dependency-graph", "semantics", "ast-dag"},
			Version:         "0.2.1",
		},
		{
			Name:            "Oxipix",
			SourceID:        ownSource.ID,
			RepositoryURL:   "https://github.com/yasuomaidana/oxipix",
			RegistryPackage: "oxipix",
			Language:        "rust",
			Description:     "High-performance SIMD pixel manipulation and image processing",
			IsOwned:         true,
			Priority:        1,
			Capabilities:    []string{"graphics", "pixels", "imaging", "rendering", "simd", "raster"},
			Version:         "0.3.5",
		},
	}
	for _, lib := range ownedLibs {
		_ = r.AddLibrary(lib)
	}

	// Priority 2: Curated & High-Performance Open Source Libraries
	curatedSources := []struct {
		source CodebaseSource
		lib    LibraryEntry
	}{
		{
			source: CodebaseSource{
				ID:          "src-oss-wgpu",
				Name:        "wgpu-ecosystem",
				Type:        SourceTypeCuratedOSS,
				Priority:    2,
				URL:         "https://github.com/gfx-rs/wgpu",
				Languages:   []string{"rust"},
				Description: "Cross-platform portable graphics abstraction library",
				CreatedAt:   now,
			},
			lib: LibraryEntry{
				Name:            "wgpu",
				SourceID:        "src-oss-wgpu",
				RepositoryURL:   "https://github.com/gfx-rs/wgpu",
				RegistryPackage: "wgpu",
				Language:        "rust",
				Description:     "Safe and portable GPU abstraction in Rust (Vulkan, Metal, DX12, WebGPU)",
				IsOwned:         false,
				Priority:        2,
				Capabilities:    []string{"gpu-rendering", "vulkan", "metal", "dx12", "webgpu", "shaders", "graphics"},
				Version:         "0.20.0",
			},
		},
		{
			source: CodebaseSource{
				ID:          "src-oss-turso",
				Name:        "turso-libsql",
				Type:        SourceTypeCuratedOSS,
				Priority:    2,
				URL:         "https://github.com/tursodatabase/libsql",
				Languages:   []string{"rust", "c"},
				Description: "Edge database engine and SQLite open fork",
				CreatedAt:   now,
			},
			lib: LibraryEntry{
				Name:            "turso",
				SourceID:        "src-oss-turso",
				RepositoryURL:   "https://github.com/tursodatabase/libsql",
				RegistryPackage: "libsql",
				Language:        "rust",
				Description:     "Open-source SQLite-compatible distributed edge database engine",
				IsOwned:         false,
				Priority:        2,
				Capabilities:    []string{"database", "libsql", "edge-db", "sqlite-compatible", "embedded-db", "storage"},
				Version:         "0.6.0",
			},
		},
		{
			source: CodebaseSource{
				ID:          "src-oss-diesel",
				Name:        "diesel-orm",
				Type:        SourceTypeCuratedOSS,
				Priority:    2,
				URL:         "https://github.com/diesel-rs/diesel",
				Languages:   []string{"rust"},
				Description: "Extensible query builder and relational ORM for Rust",
				CreatedAt:   now,
			},
			lib: LibraryEntry{
				Name:            "diesel",
				SourceID:        "src-oss-diesel",
				RepositoryURL:   "https://github.com/diesel-rs/diesel",
				RegistryPackage: "diesel",
				Language:        "rust",
				Description:     "Safe, extensible ORM and Query Builder for Rust",
				IsOwned:         false,
				Priority:        2,
				Capabilities:    []string{"orm", "query-builder", "sql", "postgres", "sqlite", "relational-db"},
				Version:         "2.2.0",
			},
		},
		{
			source: CodebaseSource{
				ID:          "src-oss-diesel-cte-ext",
				Name:        "diesel-cte-ext",
				Type:        SourceTypeCuratedOSS,
				Priority:    2,
				URL:         "https://github.com/leynos/diesel-cte-ext",
				Languages:   []string{"rust"},
				Description: "CTE (Common Table Expressions) and recursive query extension for Diesel",
				CreatedAt:   now,
			},
			lib: LibraryEntry{
				Name:            "diesel-cte-ext",
				SourceID:        "src-oss-diesel-cte-ext",
				RepositoryURL:   "https://github.com/leynos/diesel-cte-ext",
				RegistryPackage: "diesel-cte-ext",
				Language:        "rust",
				Description:     "Extensions to Diesel for Common Table Expressions (WITH / RECURSIVE)",
				IsOwned:         false,
				Priority:        2,
				Capabilities:    []string{"cte", "common-table-expressions", "recursive-queries", "hierarchical-sql", "diesel-extension"},
				Version:         "0.1.0",
			},
		},
	}

	for _, item := range curatedSources {
		_ = r.AddSource(item.source)
		_ = r.AddLibrary(item.lib)
	}

	// Priority 3: Well-known Public Registries
	registries := []CodebaseSource{
		{
			ID:          "src-reg-crates",
			Name:        "crates.io",
			Type:        SourceTypeRegistry,
			Priority:    3,
			URL:         "https://crates.io",
			Languages:   []string{"rust"},
			Description: "Rust Community Package Registry",
			CreatedAt:   now,
		},
		{
			ID:          "src-reg-npm",
			Name:        "npm-registry",
			Type:        SourceTypeRegistry,
			Priority:    3,
			URL:         "https://registry.npmjs.org",
			Languages:   []string{"typescript", "javascript"},
			Description: "Node and TypeScript Package Registry",
			CreatedAt:   now,
		},
		{
			ID:          "src-reg-maven",
			Name:        "maven-central",
			Type:        SourceTypeRegistry,
			Priority:    3,
			URL:         "https://repo1.maven.org/maven2",
			Languages:   []string{"java", "kotlin", "scala"},
			Description: "Apache Maven Central Repository",
			CreatedAt:   now,
		},
		{
			ID:          "src-reg-pypi",
			Name:        "pypi-registry",
			Type:        SourceTypeRegistry,
			Priority:    3,
			URL:         "https://pypi.org",
			Languages:   []string{"python"},
			Description: "Python Package Index",
			CreatedAt:   now,
		},
		{
			ID:          "src-reg-goproxy",
			Name:        "go-proxy",
			Type:        SourceTypeRegistry,
			Priority:    3,
			URL:         "https://proxy.golang.org",
			Languages:   []string{"go"},
			Description: "Go Module Mirror and Checksum Database",
			CreatedAt:   now,
		},
	}

	for _, reg := range registries {
		_ = r.AddSource(reg)
	}
}

// AddSource registers a new codebase host or registry.
func (r *Registry) AddSource(source CodebaseSource) error {
	if source.Priority <= 0 {
		return ErrInvalidPriority
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.sources[source.ID]; exists {
		return ErrDuplicateSource
	}
	if source.CreatedAt.IsZero() {
		source.CreatedAt = time.Now()
	}
	r.sources[source.ID] = source
	return nil
}

// GetSource retrieves a source by its unique ID.
func (r *Registry) GetSource(id string) (*CodebaseSource, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s, ok := r.sources[id]
	if !ok {
		return nil, ErrSourceNotFound
	}
	return &s, nil
}

// ListSources returns all registered codebase sources sorted by priority ascending (1 = highest).
func (r *Registry) ListSources() []CodebaseSource {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]CodebaseSource, 0, len(r.sources))
	for _, s := range r.sources {
		list = append(list, s)
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].Priority != list[j].Priority {
			return list[i].Priority < list[j].Priority
		}
		return list[i].ID < list[j].ID
	})
	return list
}

// AddLibrary registers a library or repository entry into the catalog.
func (r *Registry) AddLibrary(lib LibraryEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if lib.Priority <= 0 {
		if src, ok := r.sources[lib.SourceID]; ok {
			lib.Priority = src.Priority
		} else {
			lib.Priority = 3
		}
	}
	r.libraries[strings.ToLower(lib.Name)] = lib
	return nil
}

// ListLibraries returns all registered library entries sorted by priority ascending.
func (r *Registry) ListLibraries() []LibraryEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]LibraryEntry, 0, len(r.libraries))
	for _, l := range r.libraries {
		list = append(list, l)
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].Priority != list[j].Priority {
			return list[i].Priority < list[j].Priority
		}
		return list[i].Name < list[j].Name
	})
	return list
}

type capabilityMatch struct {
	lib   LibraryEntry
	exact bool
}

// ResolveCapability searches the library catalog for a capability matching language and returns the highest priority match.
func (r *Registry) ResolveCapability(capability string, lang string) (*LibraryEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	capLower := strings.ToLower(strings.TrimSpace(capability))
	langLower := strings.ToLower(strings.TrimSpace(lang))

	var candidates []capabilityMatch
	for _, lib := range r.libraries {
		if langLower != "" && strings.ToLower(lib.Language) != langLower {
			continue
		}
		exact := false
		matched := false
		for _, c := range lib.Capabilities {
			cLower := strings.ToLower(c)
			if cLower == capLower {
				exact = true
				matched = true
				break
			}
			if strings.Contains(cLower, capLower) || strings.Contains(capLower, cLower) {
				matched = true
			}
		}
		if matched {
			candidates = append(candidates, capabilityMatch{lib: lib, exact: exact})
		}
	}

	if len(candidates) == 0 {
		return nil, false
	}

	// Sort candidates:
	// 1. Exact capability match first (e.g. "gpu-rendering" on wgpu vs "rendering" on oxipix)
	// 2. Priority ascending (Priority 1 first)
	// 3. Name ascending
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].exact != candidates[j].exact {
			return candidates[i].exact
		}
		if candidates[i].lib.Priority != candidates[j].lib.Priority {
			return candidates[i].lib.Priority < candidates[j].lib.Priority
		}
		return candidates[i].lib.Name < candidates[j].lib.Name
	})

	best := candidates[0].lib
	return &best, true
}

// IsOwnedLibrary checks whether a given library name is owned by the user (Priority 1).
func (r *Registry) IsOwnedLibrary(libName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	lib, ok := r.libraries[strings.ToLower(strings.TrimSpace(libName))]
	if !ok {
		return false
	}
	return lib.IsOwned || lib.Priority == 1
}
