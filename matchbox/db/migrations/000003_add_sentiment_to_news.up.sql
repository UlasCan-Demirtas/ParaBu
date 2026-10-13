ALTER TABLE news_articles
ADD COLUMN IF NOT EXISTS sentiment VARCHAR(20) DEFAULT 'PENDING',
ADD COLUMN IF NOT EXISTS sentiment_confidence NUMERIC(4,3) DEFAULT 0.0,
ADD COLUMN IF NOT EXISTS sentiment_impact INTEGER DEFAULT 0;

CREATE INDEX IF NOT EXISTS ix_news_articles_sentiment ON news_articles (sentiment);
