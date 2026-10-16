package rag

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"

	"Dendrite/pkg/ast"
)

const (
	bm25K1 = 1.5
	bm25B  = 0.75
)

// FileMeta tracks the modification timestamp, size, and chunk IDs for incremental caching.
type FileMeta struct {
	Path     string   `json:"path"`
	ModTime  int64    `json:"mod_time"`
	Size     int64    `json:"size"`
	ChunkIDs []string `json:"chunk_ids"`
}

// IndexCache encapsulates the persistent state of the RAG index on disk.
type IndexCache struct {
	Version   int                 `json:"version"`
	UpdatedAt int64               `json:"updated_at"`
	Files     map[string]FileMeta `json:"files"`
	Chunks    map[string]Chunk    `json:"chunks"`
}

// Indexer scans, chunks, and indexes project source and documentation files for lexical BM25 retrieval.
type Indexer struct {
	mu        sync.RWMutex
	chunks    map[string]*Chunk
	termFreqs map[string]map[string]int // chunkID -> term -> count
	docFreqs  map[string]int            // term -> number of chunks containing term
	lengths   map[string]int            // chunkID -> total token count
	totalLen  int
	files     map[string]FileMeta // filePath -> FileMeta
}

// NewIndexer initializes an empty RAG project indexer.
func NewIndexer() *Indexer {
	return &Indexer{
		chunks:    make(map[string]*Chunk),
		termFreqs: make(map[string]map[string]int),
		docFreqs:  make(map[string]int),
		lengths:   make(map[string]int),
		files:     make(map[string]FileMeta),
	}
}

// Tokenize normalizes text into alphanumeric lowercase tokens.
func Tokenize(text string) []string {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, text)
	return strings.Fields(clean)
}

// AddChunk registers an individual chunk into the index.
func (idx *Indexer) AddChunk(c Chunk) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	tokens := Tokenize(c.Content + " " + c.Heading + " " + filepath.Base(c.FilePath))
	if len(tokens) == 0 {
		return
	}

	tf := make(map[string]int)
	for _, tok := range tokens {
		tf[tok]++
	}

	chunkCopy := c
	idx.chunks[c.ID] = &chunkCopy
	idx.termFreqs[c.ID] = tf
	idx.lengths[c.ID] = len(tokens)
	idx.totalLen += len(tokens)

	for term := range tf {
		idx.docFreqs[term]++
	}

	meta, ok := idx.files[c.FilePath]
	if !ok {
		meta = FileMeta{Path: c.FilePath}
	}
	meta.ChunkIDs = append(meta.ChunkIDs, c.ID)
	idx.files[c.FilePath] = meta
}

// RemoveFile removes all chunks belonging to the given file path and updates the BM25 frequencies.
func (idx *Indexer) RemoveFile(filePath string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	meta, ok := idx.files[filePath]
	if !ok {
		return
	}

	for _, id := range meta.ChunkIDs {
		if tf, exists := idx.termFreqs[id]; exists {
			for term := range tf {
				idx.docFreqs[term]--
				if idx.docFreqs[term] <= 0 {
					delete(idx.docFreqs, term)
				}
			}
			delete(idx.termFreqs, id)
		}
		if l, exists := idx.lengths[id]; exists {
			idx.totalLen -= l
			delete(idx.lengths, id)
		}
		delete(idx.chunks, id)
	}

	delete(idx.files, filePath)
}

// ChunksCount returns the number of indexed chunks.
func (idx *Indexer) ChunksCount() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.chunks)
}

// SaveCache persists the current index state to a JSON file.
func (idx *Indexer) SaveCache(cachePath string) error {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	chunksMap := make(map[string]Chunk, len(idx.chunks))
	for k, v := range idx.chunks {
		chunksMap[k] = *v
	}

	cache := IndexCache{
		Version:   1,
		UpdatedAt: int64(os.Getpid()),
		Files:     idx.files,
		Chunks:    chunksMap,
	}

	dir := filepath.Dir(cachePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}

	return os.WriteFile(cachePath, data, 0644)
}

// LoadCache restores the index state from a persistent JSON cache file.
func (idx *Indexer) LoadCache(cachePath string) error {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return err
	}

	var cache IndexCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return err
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.chunks = make(map[string]*Chunk, len(cache.Chunks))
	idx.termFreqs = make(map[string]map[string]int)
	idx.docFreqs = make(map[string]int)
	idx.lengths = make(map[string]int)
	idx.totalLen = 0
	idx.files = make(map[string]FileMeta, len(cache.Files))

	for k, v := range cache.Files {
		idx.files[k] = v
	}

	for id, chunk := range cache.Chunks {
		cCopy := chunk
		idx.chunks[id] = &cCopy

		tokens := Tokenize(cCopy.Content + " " + cCopy.Heading + " " + filepath.Base(cCopy.FilePath))
		if len(tokens) == 0 {
			continue
		}
		tf := make(map[string]int)
		for _, tok := range tokens {
			tf[tok]++
		}
		idx.termFreqs[id] = tf
		idx.lengths[id] = len(tokens)
		idx.totalLen += len(tokens)

		for term := range tf {
			idx.docFreqs[term]++
		}
	}

	return nil
}

// IndexProject walks a project directory and indexes markdown docs and core code files.
func (idx *Indexer) IndexProject(ctx context.Context, rootDir string) error {
	_, err := idx.IndexProjectIncremental(ctx, rootDir, false)
	return err
}

// IndexProjectIncremental scans the project root, compares modification times against the cache,
// and updates only new or changed files. Returns the status ("cached", "refreshed", or "incremental").
func (idx *Indexer) IndexProjectIncremental(ctx context.Context, rootDir string, forceRefresh bool) (string, error) {
	if rootDir == "" {
		rootDir = "."
	}

	cacheFile := filepath.Join(rootDir, ".dendrite", "rag_cache.json")
	var status = "cached"
	modifiedCount := 0

	// 1. Attempt to load cache if not forcing refresh
	if !forceRefresh {
		if err := idx.LoadCache(cacheFile); err == nil && len(idx.chunks) > 0 {
			status = "cached"
		}
	} else {
		// Clear existing in-memory state on force refresh
		idx.mu.Lock()
		idx.chunks = make(map[string]*Chunk)
		idx.termFreqs = make(map[string]map[string]int)
		idx.docFreqs = make(map[string]int)
		idx.lengths = make(map[string]int)
		idx.totalLen = 0
		idx.files = make(map[string]FileMeta)
		idx.mu.Unlock()
		status = "refreshed"
	}

	info, err := os.Stat(rootDir)
	if err != nil {
		return status, fmt.Errorf("project root path inaccessible: %w", err)
	}
	if !info.IsDir() {
		return status, idx.indexFile(rootDir)
	}

	seenFiles := make(map[string]bool)

	walkErr := filepath.Walk(rootDir, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		name := fi.Name()
		if fi.IsDir() {
			if strings.HasPrefix(name, ".") && name != "." {
				return filepath.SkipDir
			}
			switch name {
			case "node_modules", "vendor", "target", "bin", "dist", "reflected_docs":
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".md", ".mdx", ".txt", ".go", ".yaml", ".yml", ".json":
			seenFiles[path] = true

			idx.mu.RLock()
			cachedMeta, exists := idx.files[path]
			idx.mu.RUnlock()

			if exists && !forceRefresh && cachedMeta.ModTime == fi.ModTime().UnixNano() && cachedMeta.Size == fi.Size() {
				// File unchanged; retain cached chunks
				return nil
			}

			// File is new or changed: re-index file
			idx.RemoveFile(path)
			if err := idx.indexFile(path); err == nil {
				idx.mu.Lock()
				meta := idx.files[path]
				meta.ModTime = fi.ModTime().UnixNano()
				meta.Size = fi.Size()
				idx.files[path] = meta
				idx.mu.Unlock()
				modifiedCount++
			}
		}

		return nil
	})

	if walkErr != nil {
		return status, walkErr
	}

	// 2. Prune files that no longer exist on disk
	idx.mu.RLock()
	var toDelete []string
	for p := range idx.files {
		if !seenFiles[p] {
			toDelete = append(toDelete, p)
		}
	}
	idx.mu.RUnlock()

	for _, p := range toDelete {
		idx.RemoveFile(p)
		modifiedCount++
	}

	if forceRefresh {
		status = "refreshed"
	} else if modifiedCount > 0 {
		status = "incremental"
	}

	// 3. Save updated cache to disk
	_ = idx.SaveCache(cacheFile)

	return status, nil
}

// indexFile parses a file into chunks and adds them to the index.
func (idx *Indexer) indexFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".md" || ext == ".mdx" {
		idx.chunkMarkdown(filePath, string(data))
	} else if ext == ".go" {
		if !idx.chunkGoAST(filePath, string(data)) {
			idx.chunkCodeOrText(filePath, string(data))
		}
	} else {
		idx.chunkCodeOrText(filePath, string(data))
	}

	return nil
}

// chunkGoAST parses Go source into syntactically-bounded AST chunks (package, functions, types).
func (idx *Indexer) chunkGoAST(filePath, content string) bool {
	fset := token.NewFileSet()
	fileNode, err := parser.ParseFile(fset, filePath, content, parser.ParseComments)
	if err != nil || fileNode == nil {
		return false
	}

	lines := strings.Split(content, "\n")
	totalLines := len(lines)
	chunksAdded := 0

	// 1. Index package doc & overview if present
	if fileNode.Doc != nil && len(fileNode.Doc.List) > 0 {
		startLine := fset.Position(fileNode.Doc.Pos()).Line
		endLine := fset.Position(fileNode.Package).Line
		if startLine >= 1 && endLine <= totalLines && startLine <= endLine {
			pkgContent := strings.TrimSpace(strings.Join(lines[startLine-1:endLine], "\n"))
			if len(pkgContent) > 20 {
				idx.AddChunk(Chunk{
					ID:        fmt.Sprintf("%s#package-%s", filePath, fileNode.Name.Name),
					FilePath:  filePath,
					Heading:   fmt.Sprintf("package %s overview", fileNode.Name.Name),
					Content:   pkgContent,
					LineStart: startLine,
					LineEnd:   endLine,
				})
				chunksAdded++
			}
		}
	}

	// 2. Index declarations: functions, methods, structs, interfaces
	for _, decl := range fileNode.Decls {
		switch d := decl.(type) {
		case *goast.FuncDecl:
			startLine := fset.Position(d.Pos()).Line
			endLine := fset.Position(d.End()).Line
			if startLine < 1 || startLine > totalLines || endLine > totalLines || startLine > endLine {
				continue
			}

			fnContent := strings.TrimSpace(strings.Join(lines[startLine-1:endLine], "\n"))
			if len(fnContent) < 10 {
				continue
			}

			cyclomatic, cognitive := ast.AnalyzeSourceComplexity(fnContent)

			receiver := ""
			recvClean := ""
			if d.Recv != nil && len(d.Recv.List) > 0 {
				recvField := d.Recv.List[0]
				switch t := recvField.Type.(type) {
				case *goast.StarExpr:
					if ident, ok := t.X.(*goast.Ident); ok {
						receiver = fmt.Sprintf("(*%s) ", ident.Name)
						recvClean = ident.Name + "."
					}
				case *goast.Ident:
					receiver = fmt.Sprintf("(%s) ", t.Name)
					recvClean = t.Name + "."
				}
			}

			sig := fmt.Sprintf("func %s%s [cyclomatic: %d, cognitive: %d]", receiver, d.Name.Name, cyclomatic, cognitive)
			chunkID := fmt.Sprintf("%s#func-%s%s", filePath, recvClean, d.Name.Name)

			idx.AddChunk(Chunk{
				ID:        chunkID,
				FilePath:  filePath,
				Heading:   sig,
				Content:   fnContent,
				LineStart: startLine,
				LineEnd:   endLine,
			})
			chunksAdded++

		case *goast.GenDecl:
			if d.Tok == token.TYPE {
				for _, spec := range d.Specs {
					if typeSpec, ok := spec.(*goast.TypeSpec); ok {
						startLine := fset.Position(d.Pos()).Line
						endLine := fset.Position(d.End()).Line
						if startLine < 1 || startLine > totalLines || endLine > totalLines || startLine > endLine {
							continue
						}

						typeContent := strings.TrimSpace(strings.Join(lines[startLine-1:endLine], "\n"))
						kind := "type"
						switch typeSpec.Type.(type) {
						case *goast.StructType:
							kind = "struct"
						case *goast.InterfaceType:
							kind = "interface"
						}

						heading := fmt.Sprintf("type %s %s", typeSpec.Name.Name, kind)
						chunkID := fmt.Sprintf("%s#type-%s", filePath, typeSpec.Name.Name)

						idx.AddChunk(Chunk{
							ID:        chunkID,
							FilePath:  filePath,
							Heading:   heading,
							Content:   typeContent,
							LineStart: startLine,
							LineEnd:   endLine,
						})
						chunksAdded++
					}
				}
			}
		}
	}

	return chunksAdded > 0
}

func (idx *Indexer) chunkMarkdown(filePath, content string) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var currentHeading string
	var currentLines []string
	startLine := 1
	lineNum := 0
	chunkIdx := 0

	flush := func(endLine int) {
		text := strings.TrimSpace(strings.Join(currentLines, "\n"))
		if len(text) > 30 {
			chunkIdx++
			c := Chunk{
				ID:        fmt.Sprintf("%s#chunk-%d", filePath, chunkIdx),
				FilePath:  filePath,
				Heading:   currentHeading,
				Content:   text,
				LineStart: startLine,
				LineEnd:   endLine,
			}
			idx.AddChunk(c)
		}
		currentLines = nil
		startLine = endLine + 1
	}

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ") {
			if len(currentLines) > 0 {
				flush(lineNum - 1)
			}
			currentHeading = strings.TrimLeft(line, "# ")
			startLine = lineNum
		}
		currentLines = append(currentLines, line)
		if len(currentLines) >= 60 {
			flush(lineNum)
		}
	}
	if len(currentLines) > 0 {
		flush(lineNum)
	}
}

func (idx *Indexer) chunkCodeOrText(filePath, content string) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var currentLines []string
	startLine := 1
	lineNum := 0
	chunkIdx := 0

	flush := func(endLine int) {
		text := strings.TrimSpace(strings.Join(currentLines, "\n"))
		if len(text) > 30 {
			chunkIdx++
			c := Chunk{
				ID:        fmt.Sprintf("%s#chunk-%d", filePath, chunkIdx),
				FilePath:  filePath,
				Content:   text,
				LineStart: startLine,
				LineEnd:   endLine,
			}
			idx.AddChunk(c)
		}
		currentLines = nil
		startLine = endLine + 1
	}

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		currentLines = append(currentLines, line)
		if len(currentLines) >= 50 {
			flush(lineNum)
		}
	}
	if len(currentLines) > 0 {
		flush(lineNum)
	}
}

// Retrieve scores all indexed chunks against the query using Okapi BM25 and returns top matching chunks.
func (idx *Indexer) Retrieve(ctx context.Context, query string, opts SearchOptions) ([]Chunk, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if len(idx.chunks) == 0 {
		return nil, nil
	}

	queryTokens := Tokenize(query)
	if len(queryTokens) == 0 {
		return nil, nil
	}

	totalDocs := float64(len(idx.chunks))
	avgdl := float64(idx.totalLen) / totalDocs
	if avgdl == 0 {
		avgdl = 1
	}

	type scoredChunk struct {
		chunk *Chunk
		score float32
	}

	var scored []scoredChunk

	// Common abbreviations expansion for better semantic retrieval
	lowerQuery := strings.ToLower(query)
	isLLMQuery := strings.Contains(lowerQuery, "llm") || strings.Contains(lowerQuery, "model") || strings.Contains(lowerQuery, "inference")

	for id, c := range idx.chunks {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		tfMap := idx.termFreqs[id]
		docLen := float64(idx.lengths[id])
		var score float64 = 0.0

		for _, q := range queryTokens {
			freq, ok := tfMap[q]
			if !ok {
				continue
			}

			df := float64(idx.docFreqs[q])
			// Okapi BM25 IDF
			idf := math.Log(1.0 + (totalDocs-df+0.5)/(df+0.5))
			if idf < 0.1 {
				idf = 0.1
			}

			// Okapi BM25 TF component
			tfNorm := (float64(freq) * (bm25K1 + 1.0)) / (float64(freq) + bm25K1*(1.0-bm25B+bm25B*(docLen/avgdl)))
			score += idf * tfNorm
		}

		// Domain boost for LLM / inference queries matching documentation or command definitions
		if isLLMQuery {
			lowerContent := strings.ToLower(c.Content + " " + c.FilePath)
			if strings.Contains(lowerContent, "inference") || strings.Contains(lowerContent, "ollama") || strings.Contains(lowerContent, "oxillama") {
				score += 2.0
			}
			if strings.Contains(lowerContent, "inference list") || strings.Contains(lowerContent, "zero-cost") {
				score += 3.5
			}
		}

		if float32(score) >= opts.MinScore {
			scored = append(scored, scoredChunk{
				chunk: c,
				score: float32(score),
			})
		}
	}

	// Sort descending by score
	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	topK := opts.TopK
	if topK <= 0 {
		topK = 4
	}
	if len(scored) < topK {
		topK = len(scored)
	}

	results := make([]Chunk, topK)
	for i := 0; i < topK; i++ {
		c := *scored[i].chunk
		c.Score = scored[i].score
		results[i] = c
	}

	return results, nil
}
