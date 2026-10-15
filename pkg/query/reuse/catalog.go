package reuse

import "strings"

// EcosystemPackage represents a canonical package from crates.io, pkg.go.dev, npm, or PyPI.
type EcosystemPackage struct {
	Name        string
	PackageSpec string
	SourceURI   string
	ImportPath  string
	Description string
}

// LookupEcosystemPackage searches Tier 3 registry catalog for canonical packages by capability and language.
func LookupEcosystemPackage(capability, language string) (EcosystemPackage, bool) {
	normCap := strings.ToLower(strings.TrimSpace(capability))
	normLang := strings.ToLower(strings.TrimSpace(language))
	if normLang == "" {
		normLang = "rust"
	}

	packages := getCatalog()
	for _, entry := range packages {
		if entry.Language == normLang {
			for _, alias := range entry.Aliases {
				if strings.Contains(normCap, alias) || strings.Contains(alias, normCap) {
					return entry.Package, true
				}
			}
		}
	}

	return EcosystemPackage{}, false
}

type catalogEntry struct {
	Language string
	Aliases  []string
	Package  EcosystemPackage
}

func getCatalog() []catalogEntry {
	return []catalogEntry{
		// Rust (crates.io)
		{
			Language: "rust",
			Aliases:  []string{"linear_algebra", "matrix", "vector", "math", "geometry"},
			Package: EcosystemPackage{
				Name:        "nalgebra",
				PackageSpec: "nalgebra = \"0.33\"",
				SourceURI:   "https://crates.io/crates/nalgebra",
				ImportPath:  "use nalgebra::{DMatrix, Vector3};",
				Description: "General-purpose linear algebra library with statically and dynamically sized matrices.",
			},
		},
		{
			Language: "rust",
			Aliases:  []string{"serialization", "json", "deserialization", "serde"},
			Package: EcosystemPackage{
				Name:        "serde",
				PackageSpec: "serde = { version = \"1.0\", features = [\"derive\"] }",
				SourceURI:   "https://crates.io/crates/serde",
				ImportPath:  "use serde::{Serialize, Deserialize};",
				Description: "Generic serialization/deserialization framework for Rust.",
			},
		},
		{
			Language: "rust",
			Aliases:  []string{"database", "orm", "sql", "postgres", "sqlite"},
			Package: EcosystemPackage{
				Name:        "diesel",
				PackageSpec: "diesel = { version = \"2.2\", features = [\"postgres\", \"sqlite\"] }",
				SourceURI:   "https://crates.io/crates/diesel",
				ImportPath:  "use diesel::prelude::*;",
				Description: "Safe, extensible ORM and Query Builder for Rust.",
			},
		},
		{
			Language: "rust",
			Aliases:  []string{"async", "concurrency", "channels", "tokio", "runtime"},
			Package: EcosystemPackage{
				Name:        "tokio",
				PackageSpec: "tokio = { version = \"1.40\", features = [\"full\"] }",
				SourceURI:   "https://crates.io/crates/tokio",
				ImportPath:  "use tokio::sync::mpsc;",
				Description: "Event-driven, non-blocking I/O platform for writing asynchronous applications.",
			},
		},
		{
			Language: "rust",
			Aliases:  []string{"graphics", "rendering", "gpu", "wgpu"},
			Package: EcosystemPackage{
				Name:        "wgpu",
				PackageSpec: "wgpu = \"0.20\"",
				SourceURI:   "https://crates.io/crates/wgpu",
				ImportPath:  "use wgpu::*;",
				Description: "Safe and portable WebGPU graphics API implementation in Rust.",
			},
		},
		{
			Language: "rust",
			Aliases:  []string{"http", "client", "rest", "networking"},
			Package: EcosystemPackage{
				Name:        "reqwest",
				PackageSpec: "reqwest = { version = \"0.12\", features = [\"json\"] }",
				SourceURI:   "https://crates.io/crates/reqwest",
				ImportPath:  "use reqwest::Client;",
				Description: "Ergonomic HTTP Client for Rust.",
			},
		},
		{
			Language: "rust",
			Aliases:  []string{"cli", "args", "flags", "command"},
			Package: EcosystemPackage{
				Name:        "clap",
				PackageSpec: "clap = { version = \"4.5\", features = [\"derive\"] }",
				SourceURI:   "https://crates.io/crates/clap",
				ImportPath:  "use clap::Parser;",
				Description: "Command Line Argument Parser for Rust.",
			},
		},

		// Go (pkg.go.dev)
		{
			Language: "go",
			Aliases:  []string{"linear_algebra", "matrix", "vector", "math"},
			Package: EcosystemPackage{
				Name:        "gonum",
				PackageSpec: "gonum.org/v1/gonum v0.15.0",
				SourceURI:   "https://pkg.go.dev/gonum.org/v1/gonum/mat",
				ImportPath:  "import \"gonum.org/v1/gonum/mat\"",
				Description: "Set of numeric libraries for the Go programming language.",
			},
		},
		{
			Language: "go",
			Aliases:  []string{"database", "orm", "sql", "ent"},
			Package: EcosystemPackage{
				Name:        "ent",
				PackageSpec: "entgo.io/ent v0.14.0",
				SourceURI:   "https://pkg.go.dev/entgo.io/ent",
				ImportPath:  "import \"entgo.io/ent\"",
				Description: "An entity framework for Go. Simple, yet powerful ORM for modeling and querying data.",
			},
		},
		{
			Language: "go",
			Aliases:  []string{"http", "api", "routing", "rest", "gin"},
			Package: EcosystemPackage{
				Name:        "gin",
				PackageSpec: "github.com/gin-gonic/gin v1.10.0",
				SourceURI:   "https://pkg.go.dev/github.com/gin-gonic/gin",
				ImportPath:  "import \"github.com/gin-gonic/gin\"",
				Description: "Fast HTTP web framework featuring a martini-like API with performance.",
			},
		},
		{
			Language: "go",
			Aliases:  []string{"cli", "cobra", "flags", "command"},
			Package: EcosystemPackage{
				Name:        "cobra",
				PackageSpec: "github.com/spf13/cobra v1.8.1",
				SourceURI:   "https://pkg.go.dev/github.com/spf13/cobra",
				ImportPath:  "import \"github.com/spf13/cobra\"",
				Description: "A Commander for modern Go CLI interactions.",
			},
		},

		// TypeScript (npm)
		{
			Language: "typescript",
			Aliases:  []string{"linear_algebra", "graphics", "3d", "rendering"},
			Package: EcosystemPackage{
				Name:        "three",
				PackageSpec: "three@^0.165.0",
				SourceURI:   "https://www.npmjs.com/package/three",
				ImportPath:  "import * as THREE from 'three';",
				Description: "JavaScript 3D Library for WebGL/WebGPU.",
			},
		},
		{
			Language: "typescript",
			Aliases:  []string{"serialization", "schema", "validation", "json"},
			Package: EcosystemPackage{
				Name:        "zod",
				PackageSpec: "zod@^3.23.0",
				SourceURI:   "https://www.npmjs.com/package/zod",
				ImportPath:  "import { z } from 'zod';",
				Description: "TypeScript-first schema validation with static type inference.",
			},
		},

		// Python (PyPI)
		{
			Language: "python",
			Aliases:  []string{"linear_algebra", "matrix", "vector", "math"},
			Package: EcosystemPackage{
				Name:        "numpy",
				PackageSpec: "numpy>=2.0.0",
				SourceURI:   "https://pypi.org/project/numpy/",
				ImportPath:  "import numpy as np",
				Description: "Fundamental package for scientific computing with Python.",
			},
		},
		{
			Language: "python",
			Aliases:  []string{"serialization", "schema", "validation"},
			Package: EcosystemPackage{
				Name:        "pydantic",
				PackageSpec: "pydantic>=2.7.0",
				SourceURI:   "https://pypi.org/project/pydantic/",
				ImportPath:  "from pydantic import BaseModel",
				Description: "Data validation using Python type annotations.",
			},
		},
	}
}
