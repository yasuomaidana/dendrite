package store

import (
	"Dendrite/pkg/statedir"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Store provides persistent caching, hashing, and Architecture Decision Record management.
type Store struct {
	baseDir string
	adrDir  string
	mu      sync.RWMutex
}

// NewStore initializes a Store in the given base and ADR directories.
func NewStore(baseDir, adrDir string) *Store {
	if baseDir == "" {
		baseDir = statedir.Resolve(".dendrite/store")
	}
	baseDir = statedir.Resolve(baseDir)
	if adrDir == "" {
		adrDir = "docs/adr"
	}
	return &Store{
		baseDir: baseDir,
		adrDir:  adrDir,
	}
}

// ComputeHash computes a deterministic hex digest for caching and pre-checking.
func (s *Store) ComputeHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// Put stores arbitrary binary or json data under a given key.
func (s *Store) Put(key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.baseDir, 0755); err != nil {
		return fmt.Errorf("creating store directory: %w", err)
	}

	sanitizedKey := sanitizeKey(key)
	filePath := filepath.Join(s.baseDir, sanitizedKey+".dat")
	return os.WriteFile(filePath, data, 0644)
}

// Get retrieves data stored under the given key.
func (s *Store) Get(key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sanitizedKey := sanitizeKey(key)
	filePath := filepath.Join(s.baseDir, sanitizedKey+".dat")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrKeyNotFound
		}
		return nil, fmt.Errorf("reading key %s: %w", key, err)
	}
	return data, nil
}

// Has checks if a key exists in the store.
func (s *Store) Has(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sanitizedKey := sanitizeKey(key)
	filePath := filepath.Join(s.baseDir, sanitizedKey+".dat")
	_, err := os.Stat(filePath)
	return err == nil
}

// SaveADR formats and writes an ADR markdown document with YAML frontmatter.
func (s *Store) SaveADR(adr *ADR) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if adr.Title == "" || adr.TicketID == "" {
		return "", ErrInvalidADR
	}

	if err := os.MkdirAll(s.adrDir, 0755); err != nil {
		return "", fmt.Errorf("creating adr directory: %w", err)
	}

	if adr.Date.IsZero() {
		adr.Date = time.Now().UTC()
	}
	if adr.Status == "" {
		adr.Status = "Accepted"
	}

	nextID, err := s.nextADRIDLocked()
	if err != nil {
		return "", err
	}
	if adr.ID == "" {
		adr.ID = nextID
	}

	slug := slugify(adr.Title)
	filename := fmt.Sprintf("%s-%s.md", adr.ID, slug)
	fullPath := filepath.Join(s.adrDir, filename)
	adr.FilePath = fullPath

	content := formatADRMarkdown(adr)
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("writing adr file: %w", err)
	}

	_ = s.updateADRIndexLocked(adr)
	return fullPath, nil
}

// ListADRs returns all persisted ADR records ordered by identifier.
func (s *Store) ListADRs() ([]*ADR, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, err := os.Stat(s.adrDir); os.IsNotExist(err) {
		return []*ADR{}, nil
	}

	files, err := os.ReadDir(s.adrDir)
	if err != nil {
		return nil, fmt.Errorf("reading adr dir: %w", err)
	}

	var results []*ADR
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
			continue
		}
		path := filepath.Join(s.adrDir, f.Name())
		adr, err := parseADRFile(path)
		if err == nil && adr != nil {
			results = append(results, adr)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].ID < results[j].ID
	})
	return results, nil
}

// GetADR finds a specific ADR by ID (e.g. "ADR-0001").
func (s *Store) GetADR(id string) (*ADR, error) {
	adrs, err := s.ListADRs()
	if err != nil {
		return nil, err
	}
	for _, a := range adrs {
		if strings.EqualFold(a.ID, id) {
			return a, nil
		}
	}
	return nil, ErrADRNotFound
}

func (s *Store) nextADRIDLocked() (string, error) {
	if _, err := os.Stat(s.adrDir); os.IsNotExist(err) {
		return "ADR-0001", nil
	}
	files, err := os.ReadDir(s.adrDir)
	if err != nil {
		return "ADR-0001", nil
	}

	maxNum := 0
	re := regexp.MustCompile(`^ADR-(\d+)-`)
	for _, f := range files {
		m := re.FindStringSubmatch(f.Name())
		if len(m) == 2 {
			var n int
			fmt.Sscanf(m[1], "%d", &n)
			if n > maxNum {
				maxNum = n
			}
		}
	}
	return fmt.Sprintf("ADR-%04d", maxNum+1), nil
}

func (s *Store) updateADRIndexLocked(adr *ADR) error {
	_ = os.MkdirAll(s.baseDir, 0755)
	indexPath := filepath.Join(s.baseDir, "adr_index.json")

	var index []*ADR
	if data, err := os.ReadFile(indexPath); err == nil {
		_ = json.Unmarshal(data, &index)
	}

	index = append(index, adr)
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath, data, 0644)
}

func formatADRMarkdown(a *ADR) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %s\n", a.ID))
	sb.WriteString(fmt.Sprintf("ticket_id: %s\n", a.TicketID))
	sb.WriteString(fmt.Sprintf("title: \"%s\"\n", a.Title))
	sb.WriteString(fmt.Sprintf("status: %s\n", a.Status))
	sb.WriteString(fmt.Sprintf("date: %s\n", a.Date.Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("tier_selected: \"%s\"\n", a.TierSelected))
	sb.WriteString(fmt.Sprintf("package_name: \"%s\"\n", a.PackageName))
	sb.WriteString(fmt.Sprintf("source_uri: \"%s\"\n", a.SourceURI))
	sb.WriteString(fmt.Sprintf("rationale: \"%s\"\n", a.Rationale))
	sb.WriteString("---\n\n")

	sb.WriteString(fmt.Sprintf("# %s: %s\n\n", a.ID, a.Title))
	sb.WriteString("## Context\n")
	if a.Context != "" {
		sb.WriteString(a.Context + "\n\n")
	} else {
		sb.WriteString("Auditing architectural library reuse during intent modernization.\n\n")
	}

	sb.WriteString("## 3-Tier Library Reuse Evaluation\n")
	if a.Evaluation != "" {
		sb.WriteString(a.Evaluation + "\n\n")
	} else {
		sb.WriteString(fmt.Sprintf("- **Selected Tier**: %s\n- **Candidate Package**: `%s`\n- **Source URI**: %s\n\n",
			a.TierSelected, a.PackageName, a.SourceURI))
	}

	sb.WriteString("## Decision\n")
	sb.WriteString(fmt.Sprintf("Adopt `%s` via %s. %s\n\n", a.PackageName, a.TierSelected, a.Rationale))

	sb.WriteString("## Consequences\n")
	if a.Consequences != "" {
		sb.WriteString(a.Consequences + "\n")
	} else {
		sb.WriteString("- Avoids reinvention of established foundational algorithms.\n- Preserves strict dependency provenance across workspace members.\n")
	}
	return sb.String()
}

func parseADRFile(path string) (*ADR, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(data)
	if !strings.HasPrefix(text, "---") {
		return nil, fmt.Errorf("missing frontmatter")
	}

	parts := strings.SplitN(text, "---", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("malformed frontmatter")
	}

	frontmatter := parts[1]
	body := parts[2]

	adr := &ADR{FilePath: path}
	lines := strings.Split(frontmatter, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if idx := strings.Index(l, ":"); idx != -1 {
			k := strings.TrimSpace(l[:idx])
			v := strings.Trim(strings.TrimSpace(l[idx+1:]), "\"")
			switch k {
			case "id":
				adr.ID = v
			case "ticket_id":
				adr.TicketID = v
			case "title":
				adr.Title = v
			case "status":
				adr.Status = v
			case "tier_selected":
				adr.TierSelected = v
			case "package_name":
				adr.PackageName = v
			case "source_uri":
				adr.SourceURI = v
			case "rationale":
				adr.Rationale = v
			}
		}
	}
	adr.Context = body
	return adr, nil
}

func sanitizeKey(key string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9_\-\.]`)
	return re.ReplaceAllString(key, "_")
}

func slugify(title string) string {
	s := strings.ToLower(title)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}
