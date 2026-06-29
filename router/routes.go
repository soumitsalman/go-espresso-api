// @title 			Espresso API & MCP
// @version 		0.1
// @description 	Espresso is a curated business intelligence product suite. Espresso API & MCP provides access to the underlying data store.
// @description 	A **sip** is the basic unit of information: action (micro data such as market performance for a day), event (a self-contained set of micro actions and actions), and signal (larger derived intelligence from related events and actions).
// @description 	All sip identifiers are UUIDs (RFC 4122), for example `339366bc-464d-582f-8132-6875ccc814d2`. Pass them as strings in query parameters and path segments.
// @description 	List endpoints accept an optional `response_type` query parameter: `json` (default) or `text`. Both return the same underlying data; `text` renders it as flat plain text without JSON syntax, which reduces token cost for MCPs and AI agents.
// @schemes 		https
// @license.name 	MIT
// @contact.name 	Project Cafecito
// @contact.url  	http://cafecito.tech
// @contact.email 	soumitsrah@cafecito.tech
package router

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/maypok86/otter/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/soumitsalman/espressoapi/cupboard"
	"github.com/soumitsalman/espressoapi/nlp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

const (
	MIN_WINDOW          = 1
	DEFAULT_WINDOW      = 7 // DAYS
	DEFAULT_ACCURACY    = 0.75
	DEFAULT_LIMIT       = 16
	MAX_LIMIT           = 128
	FAVICON_PATH        = "images/espresso-insta-dark.png"
	DEFAULT_CONCURRENCY = 512
)

const (
	_CACHE_SIZE = 1000
	_CACHE_TTL  = 30 * time.Minute
)

const (
	_EMBEDDER_ERROR = "Embedder just died. Retry in a bit."
	_DB_ERROR       = "DB just died. Retry in a bit."
)

// baseQueryParams holds shared pagination query parameters for list endpoints.
type baseQueryParams struct {
	ResponseType string `form:"response_type,default=json" binding:"oneof=json text"`
	Limit        int    `form:"limit,default=16" binding:"min=1,max=128"`
	Offset       int    `form:"offset" binding:"min=0"`
}

// sipsQueryParams holds shared filter and search parameters for /events and /signals.
type sipsQueryParams struct {
	IDs  []string  `form:"ids" collection_format:"csv"`
	From time.Time `form:"from" time_format:"2006-01-02" swaggertype:"string" format:"date"`
	Q    string    `form:"q" binding:"max=1024"`
	Acc  float64   `form:"acc,default=0.75" binding:"min=0,max=1"`
	Tags []string  `form:"tags" collection_format:"csv"`
	baseQueryParams
}

type relatedURIParams struct {
	Relationship string `uri:"relationship" binding:"required,oneof=same_as derived_from"`
}

// relatedQueryParams holds query parameters for GET /related/{relationship}.
type relatedQueryParams struct {
	IDs []string `form:"ids" collection_format:"csv" binding:"required"`
	baseQueryParams
}

// Configuration wires database, embedding, auth, and caching dependencies into HTTP handlers.
type Configuration struct {
	DB       *cupboard.Cupboard
	Embedder nlp.Embedder
	APIKeys  map[string]string
	queue    chan int
	cache    *otter.Cache[string, []float32]
}

// health
func (r *Configuration) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "alive"})
}

// getTags godoc
// @Summary List tags for filtering events and signals
// @Description Returns a paginated, alphabetically sorted list of unique tag strings extracted from event and signal sips. Tags can be passed to `/events` and `/signals` for scalar filtering.
// @Description With `response_type=text`, tags are returned as a single comma-separated plain-text string instead of a JSON array.
// @Tags Tags
// @Produce json
// @Produce plain
// @Param response_type query string false "output format: json (default) returns a JSON string array; text returns the same tags as comma-separated plain text (lower token cost for MCPs and AI agents)" Enums(json, text) default(json)
// @Param limit query int false "page limit (items per page)" default(16) minimum(1) maximum(128)
// @Param offset query int false "pagination offset (number of items to skip)" minimum(0)
// @Success 200 {array} string "JSON string array when response_type=json (default)"
// @Success 200 {string} string "comma-separated tags when response_type=text"
// @Success 204 "no matching tags"
// @Failure 400 {object} ErrorResponse "invalid pagination parameters"
// @Failure 401 {object} ErrorResponse "missing or invalid API key"
// @Failure 429 {object} ErrorResponse "rate limit exceeded"
// @Failure 500 {object} ErrorResponse "database error"
// @ID listTags
// @Router /tags [get]
func (r *Configuration) getTags(c *gin.Context) {
	var params baseQueryParams
	if err := c.ShouldBindQuery(&params); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	data, err := r.DB.GetTags(c.Request.Context(), cupboard.Pagination{Limit: params.Limit, Offset: params.Offset})
	returnResponse(c, data, err, params.ResponseType)
}

func convertStringsToUUIDs(strings []string) ([]uuid.UUID, error) {
	errs := make([]error, 0, len(strings))
	uuids := make([]uuid.UUID, 0, len(strings))
	for _, raw := range strings {
		if id, err := uuid.Parse(raw); err != nil {
			errs = append(errs, errors.New("invalid id: "+raw))
		} else {
			uuids = append(uuids, id)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return uuids, nil
}

func (config *Configuration) extractSipsParams(c *gin.Context) (*cupboard.Condition, *cupboard.Pagination, string) {
	var input sipsQueryParams
	if err := c.ShouldBindQuery(&input); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return nil, nil, ""
	}
	conditions := cupboard.Condition{
		Created: input.From,
		Tags:    input.Tags,
	}
	if len(input.IDs) > 0 {
		ids, err := convertStringsToUUIDs(input.IDs)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return nil, nil, ""
		}
		conditions.IDs = ids
	}
	if input.Q != "" {
		conditions.Embedding = config.Embedder.EmbedQuery(c, input.Q)
		if len(conditions.Embedding) == 0 {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": _EMBEDDER_ERROR})
			return nil, nil, ""
		}
		config.cache.Set(input.Q, conditions.Embedding)
		conditions.Distance = 1 - input.Acc
	}
	return &conditions, &cupboard.Pagination{Limit: input.Limit, Offset: input.Offset}, input.ResponseType
}

// getEvents godoc
// @Summary Search events
// @Description Returns event-kind sips sorted by `created` descending (newest first).
// Each item is a flattened digest: the router merges `id` (UUID) and `created` into the digest before responding.
// Documented fields include `briefing`, `event_type`, `key_events`, `people`, `regions`, `cross_domain_impacts`, `future_outlook`, `impact_level`, and `tags`.
// Individual events may also carry additional pipeline-specific keys not listed in the schema.
// Filter by exact sip UUIDs (`ids`), tag intersection (`tags`), or semantic search (`q` + `acc`). When `from` is omitted, results are limited to roughly the last 7 days.
// @Description With `response_type=text`, each event is rendered as a flat plain-text digest (field-per-line) instead of a JSON object — same data, fewer tokens for MCPs and AI agents.
// @Tags Events
// @Produce json
// @Produce plain
// @Param ids query []string false "fetch specific event sips by UUID (RFC 4122), e.g. 339366bc-464d-582f-8132-6875ccc814d2" collectionFormat(csv)
// @Param tags query []string false "scalar tag filters (inclusive AND across supplied tags)" collectionFormat(csv)
// @Param q query string false "semantic search query (max 1024 characters; requires embedder)" maxlength(1024)
// @Param acc query number false "minimum embedding similarity for `q` (0.0-1.0, higher = stricter)" default(0.75) minimum(0) maximum(1)
// @Param from query string false "include events created on or after this date (YYYY-MM-DD)" format(date)
// @Param response_type query string false "output format: json (default) returns a JSON array of flattened digests; text returns the same data as flat plain text without JSON syntax (lower token cost for MCPs and AI agents)" Enums(json, text) default(json)
// @Param limit query int false "page size" default(16) minimum(1) maximum(128)
// @Param offset query int false "pagination offset" minimum(0)
// @Success 200 {array} Event "JSON array of flattened event digests when response_type=json (default)"
// @Success 200 {string} string "plain-text event digests (one record per block) when response_type=text"
// @Success 204 "no matching events"
// @Failure 400 {object} ErrorResponse "invalid query parameters or malformed UUID in ids"
// @Failure 401 {object} ErrorResponse "missing or invalid API key"
// @Failure 429 {object} ErrorResponse "rate limit exceeded"
// @Failure 500 {object} ErrorResponse "database or embedder error"
// @ID searchEvents
// @Router /events [get]
func (r *Configuration) getEvents(c *gin.Context) {
	conditions, page, response_type := r.extractSipsParams(c)
	if conditions == nil || page == nil {
		return
	}
	conditions.Kinds = cupboard.EVENTS
	items, err := r.DB.QuerySips(c.Request.Context(), *conditions, *page)
	returnResponse(c, items, err, response_type)
}

// getSignals godoc
// @Summary Search signals
// @Description Returns signal-kind sips sorted by `created` descending (newest first).
// Signals are derived intelligence synthesized from related events and actions.
// Each item is a flattened digest: the router merges `id` (UUID) and `created` into the digest before responding.
// Documented fields include `briefing`, `events`, `drivers`, `impacts`, `impacted_domains`, `forecast`, `impact_level`, and `tags`.
// Individual signals may also carry additional pipeline-specific keys not listed in the schema.
// Filter by exact sip UUIDs (`ids`), tag intersection (`tags`), or semantic search (`q` + `acc`). When `from` is omitted, results are limited to roughly the last 7 days.
// @Description With `response_type=text`, each signal is rendered as a flat plain-text digest (field-per-line) instead of a JSON object — same data, fewer tokens for MCPs and AI agents.
// @Tags Signals
// @Produce json
// @Produce plain
// @Param ids query []string false "fetch specific signal sips by UUID (RFC 4122), e.g. e7d7571a-13f0-56f0-8563-50863b79c781" collectionFormat(csv)
// @Param tags query []string false "scalar tag filters (inclusive AND across supplied tags)" collectionFormat(csv)
// @Param q query string false "semantic search query (max 1024 characters; requires embedder)" maxlength(1024)
// @Param acc query number false "minimum embedding similarity for `q` (0.0-1.0, higher = stricter)" default(0.75) minimum(0) maximum(1)
// @Param from query string false "include signals created on or after this date (YYYY-MM-DD)" format(date)
// @Param response_type query string false "output format: json (default) returns a JSON array of flattened digests; text returns the same data as flat plain text without JSON syntax (lower token cost for MCPs and AI agents)" Enums(json, text) default(json)
// @Param limit query int false "page size" default(16) minimum(1) maximum(128)
// @Param offset query int false "pagination offset" minimum(0)
// @Success 200 {array} Signal "JSON array of flattened signal digests when response_type=json (default)"
// @Success 200 {string} string "plain-text signal digests (one record per block) when response_type=text"
// @Success 204 "no matching signals"
// @Failure 400 {object} ErrorResponse "invalid query parameters or malformed UUID in ids"
// @Failure 401 {object} ErrorResponse "missing or invalid API key"
// @Failure 429 {object} ErrorResponse "rate limit exceeded"
// @Failure 500 {object} ErrorResponse "database or embedder error"
// @ID searchSignals
// @Router /signals [get]
func (r *Configuration) getSignals(c *gin.Context) {
	conditions, page, response_type := r.extractSipsParams(c)
	if conditions == nil || page == nil {
		return
	}
	conditions.Kinds = cupboard.SIGNALS
	items, err := r.DB.QuerySips(c.Request.Context(), *conditions, *page)
	returnResponse(c, items, err, response_type)
}

// getRelated godoc
// @Summary Get related sips by relationship
// @Description Returns sips linked to the supplied UUIDs through the requested relationship.
// `same_as` finds equivalent or duplicate records; `derived_from` finds downstream records generated from the source sip.
// Each result is a flattened digest with `id` (UUID) and `created` merged in.
// Remaining fields follow the Event or Signal response shape depending on the related record's kind; additional digest keys may be present.
// @Description With `response_type=text`, each related sip is rendered as a flat plain-text digest (field-per-line) instead of a JSON object — same data, fewer tokens for MCPs and AI agents.
// @Tags Related
// @Produce json
// @Produce plain
// @Param relationship path string true "relationship to traverse" Enums(same_as, derived_from)
// @Param ids query []string true "source sip UUIDs (RFC 4122), e.g. b07049b5-54c0-50b0-a620-d3aea3f8a173" collectionFormat(csv)
// @Param response_type query string false "output format: json (default) returns a JSON array of flattened digests; text returns the same data as flat plain text without JSON syntax (lower token cost for MCPs and AI agents)" Enums(json, text) default(json)
// @Param limit query int false "page size" default(16) minimum(1) maximum(128)
// @Param offset query int false "pagination offset" minimum(0)
// @Success 200 {array} Event "JSON array of flattened related-sip digests when response_type=json (default)"
// @Success 200 {string} string "plain-text related-sip digests (one record per block) when response_type=text"
// @Success 204 "no related sips found"
// @Failure 400 {object} ErrorResponse "missing ids, invalid relationship, or malformed UUID"
// @Failure 401 {object} ErrorResponse "missing or invalid API key"
// @Failure 429 {object} ErrorResponse "rate limit exceeded"
// @Failure 500 {object} ErrorResponse "database error"
// @ID getRelatedSips
// @Router /related/{relationship} [get]
func (r *Configuration) getRelated(c *gin.Context) {
	var uri relatedURIParams
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var query relatedQueryParams
	if err := c.ShouldBindQuery(&query); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(query.IDs) == 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "ids is required"})
		return
	}
	ids, err := convertStringsToUUIDs(query.IDs)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	items, err := r.DB.QueryRelatedSips(
		c.Request.Context(),
		cupboard.Condition{
			IDs:          ids,
			Relationship: strings.ToUpper(uri.Relationship),
		},
		cupboard.Pagination{Limit: query.Limit, Offset: query.Offset},
	)
	returnResponse(c, items, err, query.ResponseType)
}

func returnResponse[T any](c *gin.Context, items []T, err error, response_type string) {
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": _DB_ERROR})
		return
	}
	if len(items) == 0 {
		c.Status(http.StatusNoContent)
		return
	}
	if sips, ok := any(items).([]cupboard.Sip); ok {
		if response_type == "text" {
			c.String(http.StatusOK, SipsToText(sips))
			return
		}
		c.JSON(http.StatusOK, sipsToDigest(sips))
		return
	} else if tags, ok := any(items).([]string); ok {
		if response_type == "text" {
			c.String(http.StatusOK, strings.Join(tags, ", "))
			return
		}
	}
	c.JSON(http.StatusOK, items)
}

func NewRouter(db *cupboard.Cupboard, embedder nlp.Embedder, api_keys map[string]string, max_concurrent_requests int) *gin.Engine {
	if max_concurrent_requests <= 0 {
		max_concurrent_requests = DEFAULT_CONCURRENCY // default to 100 if not set or invalid
	}
	config := &Configuration{
		DB:       db,
		Embedder: embedder,
		APIKeys:  api_keys,
		queue:    make(chan int, max_concurrent_requests),
		cache: otter.Must(&otter.Options[string, []float32]{
			MaximumSize:      _CACHE_SIZE,
			ExpiryCalculator: otter.ExpiryAccessing[string, []float32](_CACHE_TTL),
		}),
	}

	router := gin.New()
	// JSON access logs and recovery using zerolog
	router.Use(
		// logger
		requestLogger,
		// recovery
		gin.Recovery(),
		// cors
		cors.New(cors.Config{
			AllowAllOrigins:  true,
			AllowMethods:     []string{"GET", "OPTIONS"},
			AllowHeaders:     []string{"*"},
			AllowCredentials: false,
			MaxAge:           24 * time.Hour,
		}),
	)

	// Swagger / OpenAPI endpoints
	// NOTE: run `swag init` to generate docs (package `docs`) before using the UI.
	// Serve Swagger UI and point it at the generated spec in assets/docs
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	router.GET("/health", config.health)
	router.StaticFile("favicon.ico", FAVICON_PATH)

	// protected group
	protected := router.Group("/")
	protected.Use(config.apiKeyMiddleware, config.concurrencyMiddleware)
	protected.GET("/tags", config.getTags)
	protected.GET("/events", config.getEvents)
	protected.GET("/signals", config.getSignals)
	protected.GET("/related/:relationship", config.getRelated)
	return router
}

// Middleware
func (r *Configuration) apiKeyMiddleware(c *gin.Context) {
	if len(r.APIKeys) == 0 {
		c.Next()
		return
	}
	for header, expected := range r.APIKeys {
		if strings.TrimSpace(c.GetHeader(header)) == expected {
			c.Next()
			return
		}
	}
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Missing API Key"})
}

func (r *Configuration) concurrencyMiddleware(c *gin.Context) {
	if r.queue != nil {
		r.queue <- 1
		defer func() { <-r.queue }()
	}
	c.Next()
}

// requestLogger logs request path, query parameters, status and latency in JSON via zerolog
func requestLogger(c *gin.Context) {
	start := time.Now()
	c.Next()

	status := c.Writer.Status()

	var evt *zerolog.Event
	if len(c.Errors) > 0 || status >= 500 {
		evt = log.Error()
	} else if status >= 400 {
		evt = log.Warn()
	} else {
		evt = log.Info()
	}
	evt.Str("module", "ROUTER").Str("method", c.Request.Method).
		Str("path", c.Request.URL.Path).
		Interface("query", c.Request.URL.Query()).
		Int("status", status).
		Float64("latency", time.Since(start).Seconds())

	if len(c.Errors) > 0 {
		evt.Str("error", c.Errors.String())
	}
	evt.Msg("incoming")
}
