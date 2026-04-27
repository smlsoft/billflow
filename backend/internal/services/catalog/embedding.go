package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"time"

	"billflow/internal/models"
	"billflow/internal/repository"
)

const EmbeddingModel = "openai/text-embedding-3-small"
const embeddingAPIURL = "https://openrouter.ai/api/v1/embeddings"

// -------------------------------------------------------------------
// Types
// -------------------------------------------------------------------

type CatalogIndex struct {
	mu    sync.RWMutex
	items []indexedItem
}

type indexedItem struct {
	models.CatalogMatch
	embedding []float64
}

// -------------------------------------------------------------------
// Embedding Service — uses OpenRouter (openai/text-embedding-3-small)
// -------------------------------------------------------------------

type EmbeddingService struct {
	apiKey string
	client *http.Client
}

func NewEmbeddingService(apiKey string) *EmbeddingService {
	return &EmbeddingService{
		apiKey: apiKey,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *EmbeddingService) IsConfigured() bool {
	return e.apiKey != ""
}

// EmbedText calls OpenRouter text-embedding-3-small and returns a 1536-dim vector.
func (e *EmbeddingService) EmbedText(text string) ([]float64, error) {
	if !e.IsConfigured() {
		return nil, fmt.Errorf("OPENROUTER_API_KEY not configured")
	}

	reqBody, _ := json.Marshal(map[string]interface{}{
		"model": EmbeddingModel,
		"input": text,
	})

	req, err := http.NewRequest(http.MethodPost, embeddingAPIURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter embedding call: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openrouter embedding %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal embedding response: %w", err)
	}
	if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("empty embedding in response")
	}
	return result.Data[0].Embedding, nil
}

// -------------------------------------------------------------------
// Cosine Similarity
// -------------------------------------------------------------------

func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// -------------------------------------------------------------------
// CatalogIndex — in-memory search
// -------------------------------------------------------------------

func NewCatalogIndex() *CatalogIndex {
	return &CatalogIndex{}
}

func (idx *CatalogIndex) Reload(repo *repository.SMLCatalogRepo) error {
	dbItems, err := repo.LoadAllEmbeddings()
	if err != nil {
		return fmt.Errorf("load embeddings: %w", err)
	}

	items := make([]indexedItem, 0, len(dbItems))
	for _, d := range dbItems {
		price := 0.0
		if d.Price != nil {
			price = *d.Price
		}
		items = append(items, indexedItem{
			CatalogMatch: models.CatalogMatch{
				ItemCode:  d.ItemCode,
				ItemName:  d.ItemName,
				ItemName2: d.ItemName2,
				UnitCode:  d.UnitCode,
				WHCode:    d.WHCode,
				ShelfCode: d.ShelfCode,
				Price:     price,
			},
			embedding: d.Embedding,
		})
	}

	idx.mu.Lock()
	idx.items = items
	idx.mu.Unlock()
	return nil
}

func (idx *CatalogIndex) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.items)
}

func (idx *CatalogIndex) Search(queryEmb []float64, topK int) []models.CatalogMatch {
	idx.mu.RLock()
	items := idx.items
	idx.mu.RUnlock()

	type scored struct {
		idx   int
		score float64
	}

	scores := make([]scored, 0, len(items))
	for i, it := range items {
		s := cosineSimilarity(queryEmb, it.embedding)
		scores = append(scores, scored{i, s})
	}

	for i := 1; i < len(scores); i++ {
		for j := i; j > 0 && scores[j].score > scores[j-1].score; j-- {
			scores[j], scores[j-1] = scores[j-1], scores[j]
		}
	}

	n := topK
	if n > len(scores) {
		n = len(scores)
	}
	result := make([]models.CatalogMatch, n)
	for i := 0; i < n; i++ {
		result[i] = items[scores[i].idx].CatalogMatch
		result[i].Score = scores[i].score
	}
	return result
}
