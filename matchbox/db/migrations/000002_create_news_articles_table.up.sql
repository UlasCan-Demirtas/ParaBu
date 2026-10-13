CREATE TABLE IF NOT EXISTS news_articles (
    id SERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    link TEXT UNIQUE NOT NULL,
    source VARCHAR(255) NOT NULL,
    published_at TIMESTAMPTZ NOT NULL,
    content TEXT,
    related_coins TEXT[]
);

CREATE INDEX IF NOT EXISTS ix_news_articles_published_at ON news_articles (published_at DESC);
CREATE INDEX IF NOT EXISTS ix_news_articles_related_coins ON news_articles USING GIN (related_coins);
