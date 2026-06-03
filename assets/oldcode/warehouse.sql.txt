INSTALL ducklake;
LOAD ducklake;
INSTALL httpfs;
LOAD httpfs;
INSTALL postgres;
LOAD postgres;

CREATE OR REPLACE SECRET s3secret (
    TYPE s3,
    PROVIDER config,
    ENDPOINT '{s3_endpoint}',
    REGION '{s3_region}',
    KEY_ID '{s3_access_key_id}',
    SECRET '{s3_secret_access_key}'
);

ATTACH 'ducklake:{catalog_path}' AS warehouse 
(METADATA_SCHEMA 'beansack', DATA_PATH '{data_path}');
USE warehouse;

CREATE TABLE IF NOT EXISTS bean_cores (
    url VARCHAR NOT NULL,
    kind VARCHAR NOT NULL,
    title VARCHAR,
    title_length UINT16,
    summary TEXT,
    summary_length UINT16,
    content TEXT,
    content_length UINT16,
    restricted_content BOOLEAN,  -- 0 for False, 1 for True
    author VARCHAR,
    source VARCHAR NOT NULL,  
    image_url VARCHAR,
    created TIMESTAMP NOT NULL,  -- ISO format datetime string
    collected TIMESTAMP NOT NULL  -- ISO format datetime string
);

CREATE TABLE IF NOT EXISTS bean_embeddings (
    url VARCHAR NOT NULL,  -- Foreign key to Bean.url
    embedding FLOAT[] NOT NULL
);

CREATE TABLE IF NOT EXISTS bean_gists (
    url VARCHAR NOT NULL,  -- Foreign key to Bean.url
    gist TEXT NOT NULL,
    regions VARCHAR[],
    entities VARCHAR[]
);

CREATE TABLE IF NOT EXISTS chatters (
    chatter_url VARCHAR NOT NULL,
    url VARCHAR NOT NULL,  -- Foreign key to Bean.url
    source VARCHAR,
    forum VARCHAR,
    collected TIMESTAMP, 
    likes UINT32,
    comments UINT32,
    subscribers UINT32
);

CREATE TABLE IF NOT EXISTS publishers (
    source VARCHAR NOT NULL,
    base_url VARCHAR NOT NULL,
    title VARCHAR DEFAULT NULL,
    summary TEXT DEFAULT NULL,    
    favicon VARCHAR DEFAULT NULL,
    rss_feed VARCHAR DEFAULT NULL
);

-- CREATE TABLE IF NOT EXISTS exported_beans (
--     url VARCHAR NOT NULL,
--     exported TIMESTAMP NOT NULL
-- );

-- THESE 2 ARE STATIC TABLES. ONCE INITIALIZED OR REGISTERED, THEY DO NOT CHANGE

CREATE TABLE IF NOT EXISTS fixed_categories AS
SELECT * FROM read_parquet('{factory}/categories.parquet');

CREATE TABLE IF NOT EXISTS fixed_sentiments AS
SELECT * FROM read_parquet('{factory}/sentiments.parquet');

-- THERE ARE COMPUTED TABLES/MATERIALIZED VIEWS THAT ARE REFRESHED PERIODICALLY

CREATE TABLE IF NOT EXISTS computed_bean_clusters (
    url VARCHAR NOT NULL,
    related VARCHAR NOT NULL,
    distance FLOAT DEFAULT 0.0
);

CREATE TABLE IF NOT EXISTS computed_bean_categories (
    url VARCHAR NOT NULL,
    categories VARCHAR[] NOT NULL
);

CREATE TABLE IF NOT EXISTS computed_bean_sentiments (
    url VARCHAR NOT NULL,
    sentiments VARCHAR[] NOT NULL
);

-- THERE ARE VIEWS/DYNAMIC QUERIES THAT ARE USED TO SIMPLIFY APP LEVEL QUERIES
-- NOTE: Technically this table is merger of bean_cores, bean_embeddings and bean_gists. so there are processed contents here
CREATE VIEW IF NOT EXISTS unprocessed_beans_view AS
SELECT * EXCLUDE(e.url, g.url) FROM bean_cores b
LEFT JOIN bean_embeddings e ON b.url = e.url
LEFT JOIN bean_gists g ON b.url = g.url
ORDER BY created DESC;

CREATE VIEW IF NOT EXISTS missing_clusters_view AS
SELECT * FROM bean_embeddings e
WHERE NOT EXISTS (
    SELECT 1 FROM warehouse.computed_bean_clusters cl 
    WHERE cl.url = e.url
);

CREATE VIEW IF NOT EXISTS missing_categories_view AS
SELECT * FROM bean_embeddings e
WHERE NOT EXISTS (
    SELECT 1 FROM warehouse.computed_bean_categories c 
    WHERE c.url = e.url
);

CREATE VIEW IF NOT EXISTS missing_sentiments_view AS
SELECT * FROM bean_embeddings e
WHERE NOT EXISTS (
    SELECT 1 FROM warehouse.computed_bean_sentiments s 
    WHERE s.url = e.url
);

CREATE VIEW IF NOT EXISTS bean_clusters_view AS
SELECT 
    url, 
    FIRST(cl.related ORDER BY cluster_size DESC) AS cluster_id, -- this is the larger cluster url is part of
    COUNT(*) AS cluster_size -- this is the number of related items
FROM computed_bean_clusters cl
INNER JOIN (
    SELECT related, count(*) AS cluster_size 
    FROM computed_bean_clusters GROUP BY related
) clsz ON cl.related = clsz.related
GROUP BY url;

CREATE VIEW IF NOT EXISTS indexed_beans_view AS
SELECT * EXCLUDE(e.url, c.url, s.url, cl.url) 
FROM bean_embeddings e
INNER JOIN bean_cores b ON b.url = e.url
INNER JOIN computed_bean_categories c ON b.url = c.url
INNER JOIN computed_bean_sentiments s ON b.url = s.url
INNER JOIN bean_clusters_view cl ON b.url = cl.url
ORDER BY created DESC;

CREATE VIEW IF NOT EXISTS unique_indexed_beans_view AS
WITH newest_beans AS (
    SELECT cluster_id, MAX(created) as created 
    FROM indexed_beans_view 
    GROUP BY cluster_id
)
SELECT pb.* FROM newest_beans nb
INNER JOIN indexed_beans_view pb 
ON nb.cluster_id = pb.cluster_id AND nb.created=pb.created;

CREATE VIEW IF NOT EXISTS processed_beans_view AS
SELECT * EXCLUDE(ib.url) 
FROM bean_gists g
INNER JOIN indexed_beans_view ib ON ib.url = g.url;

CREATE VIEW IF NOT EXISTS unique_processed_beans_view AS
SELECT * EXCLUDE(ib.url) 
FROM bean_gists g
INNER JOIN unique_indexed_beans_view ib ON ib.url = g.url;

-- TODO: look to see if it can be replaced with FIRST(collected ORDER BY likes DESC)
CREATE VIEW IF NOT EXISTS bean_chatters_view AS
WITH 
    max_stats AS (
        SELECT 
            chatter_url,
            MAX(likes) as likes, 
            MAX(comments) as comments
        FROM chatters 
        GROUP BY chatter_url
    ),
    first_seen AS (
        SELECT 
            fs.chatter_url,
            MIN(fs.collected) as collected
        FROM chatters fs 
        LEFT JOIN max_stats mx ON fs.chatter_url = mx.chatter_url
        WHERE fs.likes = mx.likes AND fs.comments = mx.comments
        GROUP BY fs.chatter_url
    )
SELECT 
    url,
    DATE(MAX(collected)) as collected,
    SUM(likes) as likes, 
    SUM(comments) as comments, 
    SUM(subscribers) as subscribers,
    COUNT(chatter_url) as shares
FROM(
    SELECT ch.* FROM chatters ch 
    LEFT JOIN first_seen fs ON fs.chatter_url = ch.chatter_url  
    WHERE fs.collected = ch.collected
) 
GROUP BY url
ORDER BY collected DESC;

CREATE VIEW IF NOT EXISTS trending_beans_view AS
SELECT * EXCLUDE (ch.url, ch.collected), ch.collected as updated FROM processed_beans_view pb
INNER JOIN bean_chatters_view ch ON pb.url = ch.url;