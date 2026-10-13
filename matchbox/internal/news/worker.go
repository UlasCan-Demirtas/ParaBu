package news

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mmcdole/gofeed"
	"github.com/redis/go-redis/v9"
)

type NewsArticle struct {
	Title        string
	Link         string
	Source       string
	PublishedAt  time.Time
	Content      string
	RelatedCoins []string
}

type NewsWorker struct {
	redisClient *redis.Client
	dbPool      *pgxpool.Pool
	rssFeeds    []string
	coins       []string
}

func NewNewsWorker(redisClient *redis.Client, dbPool *pgxpool.Pool) *NewsWorker {
	return &NewsWorker{
		redisClient: redisClient,
		dbPool:      dbPool,
		rssFeeds: []string{
			"https://cointelegraph.com/rss",
			"https://www.coindesk.com/arc/outboundfeeds/rss/",
			"https://cryptoslate.com/feed/",
		},
		coins: []string{"BTC", "ETH", "XRP", "SOL", "ADA", "DOGE"},
	}
}

func (nw *NewsWorker) Start(ctx context.Context) {
	go nw.runLoop(ctx)
}

func (nw *NewsWorker) runLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	// Initial run
	nw.fetchAndProcess(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			nw.fetchAndProcess(ctx)
		}
	}
}

func (nw *NewsWorker) fetchAndProcess(ctx context.Context) {
	fp := gofeed.NewParser()
	for _, feedURL := range nw.rssFeeds {
		feed, err := fp.ParseURLWithContext(feedURL, ctx)
		if err != nil {
			log.Printf("Failed to parse feed %s: %v", feedURL, err)
			continue
		}

		// Use the Title of the feed as the source name, fallback to URL if empty
		source := feed.Title
		if source == "" {
			source = feedURL
		}

		for _, item := range feed.Items {
			// Check Redis for duplicate by link hash to prevent reprocessing
			hash := hashLink(item.Link)
			isNew, err := nw.redisClient.SAdd(ctx, "processed_news_links", hash).Result()
			if err != nil {
				log.Printf("Redis error: %v", err)
				continue
			}

			// 0 means it was already in the Set (duplicate)
			if isNew == 0 {
				continue
			}

			// It's a new article, parse and save
			article := nw.parseArticle(item, source)
			err = nw.saveToDB(ctx, article)
			if err != nil {
				log.Printf("Failed to save article '%s': %v", article.Title, err)
			} else {
				log.Printf("Saved new article: %s", article.Title)
			}
		}
	}
}

func hashLink(link string) string {
	h := sha256.New()
	h.Write([]byte(link))
	return hex.EncodeToString(h.Sum(nil))
}

func (nw *NewsWorker) parseArticle(item *gofeed.Item, source string) NewsArticle {
	content := item.Content
	if content == "" {
		content = item.Description
	}

	// Use goquery to extract plain text from HTML content (common in RSS feeds)
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	plainText := ""
	if err == nil {
		plainText = doc.Text()
	} else {
		plainText = content
	}

	// Clean up whitespace
	plainText = strings.Join(strings.Fields(plainText), " ")

	// Detect related coins by scanning title and content
	var related []string
	upperTitle := strings.ToUpper(item.Title)
	upperContent := strings.ToUpper(plainText)

	for _, coin := range nw.coins {
		if strings.Contains(upperTitle, coin) || strings.Contains(upperContent, coin) {
			related = append(related, coin)
		}
	}

	pubDate := time.Now()
	if item.PublishedParsed != nil {
		pubDate = *item.PublishedParsed
	}

	return NewsArticle{
		Title:        item.Title,
		Link:         item.Link,
		Source:       source,
		PublishedAt:  pubDate,
		Content:      plainText,
		RelatedCoins: related,
	}
}

func (nw *NewsWorker) saveToDB(ctx context.Context, a NewsArticle) error {
	query := `
		INSERT INTO news_articles (title, link, source, published_at, content, related_coins)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (link) DO NOTHING
		RETURNING id
	`
	var id int
	err := nw.dbPool.QueryRow(ctx, query, a.Title, a.Link, a.Source, a.PublishedAt, a.Content, a.RelatedCoins).Scan(&id)
	
	if err != nil {
		if err.Error() == "no rows in result set" {
			// This means ON CONFLICT triggered and no row was inserted, which is fine
			return nil
		}
		return err
	}

	// Publish to Redis Queue for Sentiment Analysis
	job := map[string]interface{}{
		"id":      id,
		"title":   a.Title,
		"content": a.Content,
	}
	jobJSON, _ := json.Marshal(job)
	nw.redisClient.RPush(ctx, "sentiment_jobs", jobJSON)

	return nil
}
