package testgen

import (
	"strings"
)

// ClassifyDependencies inspects source code content and import statements to detect external infrastructure.
func ClassifyDependencies(sourceCode string) []DependencyType {
	var deps []DependencyType
	seen := make(map[DependencyType]bool)

	addDep := func(d DependencyType) {
		if !seen[d] {
			seen[d] = true
			deps = append(deps, d)
		}
	}

	lower := strings.ToLower(sourceCode)

	// Check PostgreSQL / SQL
	if strings.Contains(lower, "database/sql") ||
		strings.Contains(lower, "github.com/lib/pq") ||
		strings.Contains(lower, "pgx") ||
		strings.Contains(lower, "gorm.io") ||
		strings.Contains(lower, "sql.open") ||
		strings.Contains(lower, "db.query") ||
		strings.Contains(lower, "db.exec") {
		addDep(DepPostgres)
	}

	// Check SQLite
	if strings.Contains(lower, "github.com/mattn/go-sqlite3") ||
		strings.Contains(lower, "modernc.org/sqlite") ||
		strings.Contains(lower, ".db") && strings.Contains(lower, "sqlite") {
		addDep(DepSQLite)
	}

	// Check Redis
	if strings.Contains(lower, "github.com/redis/go-redis") ||
		strings.Contains(lower, "redis.client") ||
		strings.Contains(lower, "redis.newclient") ||
		strings.Contains(lower, "rdb.get") ||
		strings.Contains(lower, "rdb.set") {
		addDep(DepRedis)
	}

	// Check HTTP Client
	if strings.Contains(lower, "http.client") ||
		strings.Contains(lower, "http.get(") ||
		strings.Contains(lower, "http.post(") ||
		strings.Contains(lower, "http.newrequest") {
		addDep(DepHTTPClient)
	}

	if len(deps) == 0 {
		return []DependencyType{DepNone}
	}

	return deps
}

// RequiresTestcontainers determines if the detected dependencies mandate Testcontainers.
func RequiresTestcontainers(deps []DependencyType) bool {
	for _, d := range deps {
		if d == DepPostgres || d == DepRedis || d == DepSQLite {
			return true
		}
	}
	return false
}
