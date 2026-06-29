package cupboard

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	pgxvec "github.com/pgvector/pgvector-go/pgx"
)

// pg connection configuration
const (
	_TIMEOUT        = 10
	_POOL_SIZE      = 32
	_CONN_LIFETIME  = 5
	_CONN_IDLE_TIME = 5
)

// query constants
const (
	SIPS      = "sips"
	SOURCES   = "sources"
	RELATIONS = "relations"

	_SIP_FROM      = "sips"
	_SIP_FIELDS    = "id, created, digest"
	_SOURCE_FIELDS = "id, base_url, domain_name, site_name, description, favicon"
	_LATEST_SIPS   = "sips.created DESC"
	_TRENDING_SIPS = "(SELECT count(*) FROM relations WHERE from_id = sips.id) DESC"
)

var (
	EVENTS        = []string{"event:blog", "event:news", "event:post", "event:site", "event:social"}
	SIGNALS       = []string{"signal"}
	RELATIONSHIPS = []string{"SAME_AS", "DERIVED_FROM"}
)

type Condition struct {
	IDs          []uuid.UUID
	Relationship string
	Kinds        []string
	Created      time.Time
	Tags         []string
	FTS          bool
	Embedding    []float32
	Distance     float64
	Extra        []string // CAUTION: This is a catch-all for any additional conditions. Use with care to avoid SQL injection.
}

type Pagination struct {
	Limit  int
	Offset int
}

type Cupboard struct {
	db *pgxpool.Pool
}

func NewCupboard(ctx context.Context, connString string) *Cupboard {
	config, err := pgxpool.ParseConfig(connString)
	NoError(err)

	config.MinConns = 1
	config.MaxConns = _POOL_SIZE
	config.MaxConnLifetime = time.Minute * _CONN_LIFETIME
	config.MaxConnIdleTime = time.Minute * _CONN_IDLE_TIME
	config.HealthCheckPeriod = time.Minute * _CONN_LIFETIME
	config.ConnConfig.ConnectTimeout = time.Minute * _TIMEOUT
	if config.ConnConfig.RuntimeParams == nil {
		config.ConnConfig.RuntimeParams = map[string]string{}
	}
	config.ConnConfig.RuntimeParams["statement_timeout"] = fmt.Sprintf("%dmin", _TIMEOUT)
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvec.RegisterTypes(ctx, conn)
	}

	db, err := pgxpool.NewWithConfig(ctx, config)
	NoError(err)
	NoError(db.Ping(ctx)) // Quick health check on startup

	return &Cupboard{db: db}
}

func (p *Cupboard) QuerySips(ctx context.Context, conditions Condition, page Pagination) ([]Sip, error) {
	query, args := p.buildSQL(SIPS, conditions, page, _SIP_FIELDS)
	return fetchAll[Sip](ctx, p.db, query, args)
}

const _RELATED_SIPS_QUERY = `
SELECT id, created, digest
FROM sips
WHERE EXISTS (
	SELECT 1 FROM relations
	WHERE relationship = @relationship
	AND ((from_id = ANY(@ids) AND to_id=sips.id) OR (to_id = ANY(@ids) AND from_id=sips.id))
)`

func (p *Cupboard) QueryRelatedSips(ctx context.Context, conditions Condition, page Pagination) ([]Sip, error) {
	expr_builder := strings.Builder{}
	params := pgx.NamedArgs{
		"relationship": conditions.Relationship,
		"ids":          conditions.IDs,
	}
	expr_builder.WriteString(_RELATED_SIPS_QUERY)
	buildPaginationExpr(page, &expr_builder, params)
	return fetchAll[Sip](ctx, p.db, expr_builder.String(), params)
}

const _DISTINCT_TAGS_QUERY = "SELECT DISTINCT unnest(tags) AS tag FROM sips ORDER BY tag"

func (p *Cupboard) GetTags(ctx context.Context, page Pagination) ([]string, error) {
	expr_builder := strings.Builder{}
	params := pgx.NamedArgs{}
	expr_builder.WriteString(_DISTINCT_TAGS_QUERY)
	buildPaginationExpr(page, &expr_builder, params)
	return fetchAllScalar[string](ctx, p.db, expr_builder.String(), params)
}

func (p *Cupboard) CountSips(ctx context.Context, conditions Condition) (int64, error) {
	query, args := p.buildSQL(SIPS, conditions, Pagination{}, "COUNT(*)")
	return fetchOneScalar[int64](ctx, p.db, query, args)
}

func (p *Cupboard) Close() {
	if p != nil && p.db != nil {
		p.db.Close()
	}
}

// SQL query string builder utilities
func (p *Cupboard) buildSQL(table string, conditions Condition, page Pagination, fields_expr string) (string, pgx.NamedArgs) {
	// select fields
	where_parts := make([]string, 0, 10)
	params := pgx.NamedArgs{}
	order_parts := []string{}

	// set fields and order parts based on table
	if table == SIPS {
		order_parts = []string{_LATEST_SIPS, _TRENDING_SIPS}
	}

	from_expr := table
	if table == SIPS {
		from_expr = _SIP_FROM
	}

	// when a set of IDs are given check if a relationship is set --> query the items that are related to the given IDs
	// otherwise query the items by the given IDs
	if len(conditions.IDs) > 0 {
		where_parts = append(where_parts, sipColumn(table, "id")+" = ANY(@ids)")
		params["ids"] = conditions.IDs
	}
	if len(conditions.Kinds) > 0 {
		where_parts = append(where_parts, sipColumn(table, "kind")+" = ANY(@kinds)")
		params["kinds"] = conditions.Kinds
	}
	if !conditions.Created.IsZero() {
		where_parts = append(where_parts, sipColumn(table, "created")+" >= @created")
		params["created"] = conditions.Created
	}
	if len(conditions.Tags) > 0 {
		if conditions.FTS {
			where_parts = append(where_parts, sipColumn(table, "tags_fts")+" @@ plainto_tsquery('simple', @tags)")
			params["tags"] = strings.Join(conditions.Tags, " & ")
		} else {
			where_parts = append(where_parts, sipColumn(table, "tags")+" && @tags")
			params["tags"] = conditions.Tags
		}
	}
	if len(conditions.Embedding) > 0 {
		embedding_col := sipColumn(table, "embedding")
		if conditions.Distance > 0 {
			where_parts = append(where_parts, embedding_col+" <=> @embedding <= @distance")
			params["distance"] = conditions.Distance
		} else {
			order_parts = []string{"(" + embedding_col + " <=> @embedding) ASC", _LATEST_SIPS}
		}
		params["embedding"] = pgvector.NewVector(conditions.Embedding)
	}
	if len(conditions.Extra) > 0 {
		where_parts = append(where_parts, conditions.Extra...)
	}

	expr_builder := strings.Builder{}
	expr_builder.WriteString(fmt.Sprintf("SELECT %s FROM %s", fields_expr, from_expr))
	if len(where_parts) > 0 {
		expr_builder.WriteString("\nWHERE ")
		expr_builder.WriteString(strings.Join(where_parts, " AND "))
	}
	if len(order_parts) > 0 {
		expr_builder.WriteString("\nORDER BY ")
		expr_builder.WriteString(strings.Join(order_parts, ", "))
	}
	buildPaginationExpr(page, &expr_builder, params)

	return expr_builder.String(), params
}

func sipColumn(table, column string) string {
	if table == SIPS {
		return "sips." + column
	}
	return column
}

func buildPaginationExpr(page Pagination, expr_builder *strings.Builder, params pgx.NamedArgs) {
	if page.Limit > 0 {
		expr_builder.WriteString("\nLIMIT @limit")
		params["limit"] = page.Limit
	}
	if page.Offset > 0 {
		expr_builder.WriteString("\nOFFSET @offset")
		params["offset"] = page.Offset
	}
}

// PGX fetch helpers
func fetchOne[T any](ctx context.Context, db *pgxpool.Pool, query string, args pgx.NamedArgs) (T, error) {
	rows, err := db.Query(ctx, query, args)
	if err != nil {
		var zero T
		return zero, err
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByNameLax[T])
}

func fetchOneScalar[T any](ctx context.Context, db *pgxpool.Pool, query string, args pgx.NamedArgs) (T, error) {
	rows, err := db.Query(ctx, query, args)
	if err != nil {
		var zero T
		return zero, err
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowTo[T])
}

func fetchAll[T any](ctx context.Context, db *pgxpool.Pool, query string, args pgx.NamedArgs) ([]T, error) {
	rows, err := db.Query(ctx, query, args)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByNameLax[T])
}

func fetchAllScalar[T any](ctx context.Context, db *pgxpool.Pool, query string, args pgx.NamedArgs) ([]T, error) {
	rows, err := db.Query(ctx, query, args)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowTo[T])
}
