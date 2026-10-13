package news

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"matchbox/internal/ws"
)

type SentimentEngine struct {
	dbPool    *pgxpool.Pool
	redis     *redis.Client
	llmClient *LLMClient
	hub       *ws.Hub
}

type NewsJob struct {
	ID      int    `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

func NewSentimentEngine(db *pgxpool.Pool, rdb *redis.Client, apiKey string, hub *ws.Hub) *SentimentEngine {
	return &SentimentEngine{
		dbPool:    db,
		redis:     rdb,
		llmClient: NewLLMClient(apiKey),
		hub:       hub,
	}
}

func (se *SentimentEngine) Start(ctx context.Context, workers int) {
	for i := 0; i < workers; i++ {
		go se.workerLoop(ctx, i)
	}
}

func (se *SentimentEngine) workerLoop(ctx context.Context, workerID int) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		res, err := se.redis.BLPop(ctx, 5*time.Second, "sentiment_jobs").Result()
		if err != nil {
			if err != redis.Nil {
				time.Sleep(1 * time.Second)
			}
			continue
		}

		// res[0] is list name, res[1] is the JSON value
		var job NewsJob
		if err := json.Unmarshal([]byte(res[1]), &job); err != nil {
			log.Printf("[Worker %d] Failed to decode sentiment job: %v", workerID, err)
			continue
		}

		se.processJob(ctx, job, workerID)
	}
}

func (se *SentimentEngine) processJob(ctx context.Context, job NewsJob, workerID int) {
	// 1. Sanitize Data
	if len(job.Content) < 30 {
		se.updateStatus(ctx, job.ID, "NEUTRAL", 0.0, 1)
		return
	}

	contentToAnalyze := job.Content
	// Truncate to prevent max_token explosion (approx 1000 words)
	if len(contentToAnalyze) > 5000 {
		contentToAnalyze = contentToAnalyze[:5000] 
	}

	// 2. Analyze with LLM & Exponential Backoff Retry
	var sr *SentimentResult
	var err error
	maxRetries := 3

	for i := 0; i < maxRetries; i++ {
		fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		sr, err = se.llmClient.Analyze(fetchCtx, job.Title, contentToAnalyze)
		cancel()

		if err == nil {
			time.Sleep(5 * time.Second) // Respect Gemini Free Tier 15 RPM limit (60s/15=4s)
			break
		}
		
		log.Printf("[Worker %d] LLM error for news %d: %v. Retrying in %ds...", workerID, job.ID, err, 1<<i)
		time.Sleep(time.Duration(1<<i) * time.Second)
	}

	if err != nil {
		log.Printf("[Worker %d] Failed to analyze news %d after retries. Marking FAILED.", workerID, job.ID)
		se.updateStatus(ctx, job.ID, "FAILED", 0.0, 0)
		return
	}

	// 3. Update DB and Broadcast to WS Clients
	se.updateStatus(ctx, job.ID, sr.Sentiment, sr.Confidence, sr.Impact)
}

func (se *SentimentEngine) updateStatus(ctx context.Context, id int, sentiment string, confidence float64, impact int) {
	query := `
		UPDATE news_articles 
		SET sentiment = $1, sentiment_confidence = $2, sentiment_impact = $3 
		WHERE id = $4
	`
	_, err := se.dbPool.Exec(ctx, query, sentiment, confidence, impact, id)
	if err != nil {
		log.Printf("Failed to update sentiment for news %d: %v", id, err)
		return
	}
	
	log.Printf("News %d sentiment updated to %s (Impact: %d)", id, sentiment, impact)

	if se.hub != nil {
		// Fetch title, published_at, related_coins, content for enriched broadcast
		var title, content string
		var publishedAt time.Time
		var relatedCoins []string
		fetchErr := se.dbPool.QueryRow(ctx, `
			SELECT title, published_at, related_coins, content 
			FROM news_articles WHERE id = $1
		`, id).Scan(&title, &publishedAt, &relatedCoins, &content)

		summary := ""
		symbol := "BTC/USDT" // default
		if fetchErr == nil {
			summary = content
			if len(summary) > 120 {
				summary = summary[:120] + "..."
			}
			// Determine primary symbol from related coins
			for _, coin := range relatedCoins {
				switch coin {
				case "ETH", "ethereum":
					symbol = "ETH/USDT"
				case "XRP", "ripple":
					symbol = "XRP/USDT"
				case "BTC", "bitcoin":
					symbol = "BTC/USDT"
				}
				break // Use the first matched coin
			}
		}

		payload, _ := json.Marshal(map[string]interface{}{
			"id":         id,
			"title":      title,
			"sentiment":  sentiment,
			"confidence": confidence,
			"impact":     impact,
			"timestamp":  publishedAt.Unix(),
			"summary":    summary,
			"symbol":     symbol,
		})
		
		// Broadcast using WebSocket Hub
		se.hub.Broadcast("!news@sentiment", payload)
	}
}
