package agentsearch

import (
	"math"
	"strings"
	"sync"
	"unicode"
)

const (
	defaultK1 = 1.5
	defaultB  = 0.75
)

// Document represents an indexed text entry in the BM25 corpus.
type Document struct {
	ID        string
	Tokens    []string
	TermFreqs map[string]int
	Length    int
}

// BM25Index implements an in-memory Okapi BM25 sparse lexical search index.
type BM25Index struct {
	mu        sync.RWMutex
	docs      map[string]*Document
	docFreqs  map[string]int
	totalDocs int
	totalLen  int
	k1        float64
	b         float64
}

// NewBM25Index constructs an empty BM25Index with default Okapi constants.
func NewBM25Index() *BM25Index {
	return &BM25Index{
		docs:     make(map[string]*Document),
		docFreqs: make(map[string]int),
		k1:       defaultK1,
		b:        defaultB,
	}
}

// Tokenize splits raw text into normalized words, stripping punctuation and delimiters.
func Tokenize(text string) []string {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, text)

	return strings.Fields(clean)
}

// AddDocument registers a document into the lexical index.
func (idx *BM25Index) AddDocument(id, text string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	tokens := Tokenize(text)
	tf := make(map[string]int, len(tokens))
	for _, tok := range tokens {
		tf[tok]++
	}

	doc := &Document{
		ID:        id,
		Tokens:    tokens,
		TermFreqs: tf,
		Length:    len(tokens),
	}

	// Update document frequencies
	for term := range tf {
		idx.docFreqs[term]++
	}

	idx.docs[id] = doc
	idx.totalDocs++
	idx.totalLen += len(tokens)
}

// Score computes BM25 relevance scores for all documents given a query string.
func (idx *BM25Index) Score(query string) map[string]float32 {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	scores := make(map[string]float32, len(idx.docs))
	if idx.totalDocs == 0 {
		return scores
	}

	queryTokens := Tokenize(query)
	if len(queryTokens) == 0 {
		return scores
	}

	avgdl := float64(idx.totalLen) / float64(idx.totalDocs)
	if avgdl == 0 {
		avgdl = 1.0
	}

	var maxScore float64

	for id, doc := range idx.docs {
		var docScore float64
		for _, q := range queryTokens {
			tf := float64(doc.TermFreqs[q])
			if tf == 0 {
				continue
			}

			df := float64(idx.docFreqs[q])
			// Probabilistic IDF with floor at 0
			n := float64(idx.totalDocs)
			idf := math.Log(1.0 + (n-df+0.5)/(df+0.5))

			numerator := tf * (idx.k1 + 1.0)
			denominator := tf + idx.k1*(1.0-idx.b+idx.b*(float64(doc.Length)/avgdl))

			docScore += idf * (numerator / denominator)
		}

		if docScore > 0 {
			scores[id] = float32(docScore)
			if docScore > maxScore {
				maxScore = docScore
			}
		} else {
			scores[id] = 0
		}
	}

	// Normalize scores into [0.0, 1.0]
	if maxScore > 0 {
		for id, s := range scores {
			scores[id] = float32(float64(s) / maxScore)
		}
	}

	return scores
}
