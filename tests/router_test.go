package espressoapi_test

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/k0kubun/pp"
	"github.com/soumitsalman/espressoapi/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const defaultStressBaseURL = "http://localhost:8080"

const (
	minConcurrency = 100
	maxConcurrency = 10000
	httpTimeout    = 10 * time.Minute

	routeTags    = "/tags"
	routeEvents  = "/events"
	routeSignals = "/signals"
	routeRelated = "/related"

	relSameAs      = "same_as"
	relDerivedFrom = "derived_from"
)

var relatedRelationships = []string{relSameAs, relDerivedFrom}

// stressEndpoint describes one API endpoint and its optional query params (router/routes.go).
type stressEndpoint struct {
	path        string
	acceptsQ    bool
	acceptsTags bool
	acceptsFrom bool
	isRelated   bool
}

var stressEndpoints = []stressEndpoint{
	{path: routeEvents, acceptsQ: true, acceptsTags: true, acceptsFrom: true},
	{path: routeSignals, acceptsQ: true, acceptsTags: true, acceptsFrom: true},
	{path: routeTags},
	{path: routeRelated, isRelated: true},
}

var sampleQueries = []string{
	"artificial intelligence",
	"machine learning",
	"cloud computing",
	"cybersecurity breaches",
	"open source software",
	"startup funding",
	"climate change policy",
	"quantum computing",
	"electric vehicles",
	"blockchain technology",
}

var sampleTags = []string{
	"public_policy",
	"market_trends",
	"criminal_investigation",
	"OpenAI",
	"Google",
	"US",
	"Europe",
}

// --- in-process router integration tests (mirror tests/db_test.go) ---

func newTestHTTPServer(t *testing.T) *httptest.Server {
	t.Helper()
	db := setupTestDB()
	embedder := setupTestEmbedder()
	gin.SetMode(gin.TestMode)
	engine := router.NewRouter(db, embedder, nil, 0)
	srv := httptest.NewServer(engine)
	t.Cleanup(func() {
		srv.Close()
		embedder.Close()
		db.Close()
	})
	return srv
}

func relatedPath(relationship string) string {
	return routeRelated + "/" + relationship
}

func addRelatedIDs(params url.Values, ids []uuid.UUID) {
	for _, id := range ids {
		params.Add("ids", id.String())
	}
}

func routerURL(base, path string, params url.Values) string {
	raw := strings.TrimSuffix(base, "/") + path
	if enc := params.Encode(); enc != "" {
		raw += "?" + enc
	}
	return raw
}

func routerGET(t *testing.T, base, path string, params url.Values, apiKey string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, routerURL(base, path, params), nil)
	require.NoError(t, err)
	if apiKey != "" {
		req.Header.Set("X-API-KEY", apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, body
}

func requireStatus(t *testing.T, expected int, actual int, body []byte) {
	t.Helper()
	require.Equal(t, expected, actual, "response body: %s", string(body))
}

func parseSipDigestArray(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var items []map[string]any
	require.NoError(t, json.Unmarshal(body, &items))
	return items
}

func parseStringArray(t *testing.T, body []byte) []string {
	t.Helper()
	var items []string
	require.NoError(t, json.Unmarshal(body, &items))
	return items
}

func TestRouterGetTags(t *testing.T) {
	srv := newTestHTTPServer(t)
	params := url.Values{}
	params.Set("limit", "5")
	params.Set("offset", "10")

	status, body := routerGET(t, srv.URL, routeTags, params, "")
	requireStatus(t, http.StatusOK, status, body)
	tags := parseStringArray(t, body)
	assert.NotEmpty(t, tags)
	pp.Println("TAGS", tags)
}

func TestRouterRelatedSips(t *testing.T) {
	srv := newTestHTTPServer(t)
	params := url.Values{}
	addRelatedIDs(params, testRelatedIDs)

	status, body := routerGET(t, srv.URL, relatedPath(relSameAs), params, "")
	requireStatus(t, http.StatusOK, status, body)
	sips := parseSipDigestArray(t, body)
	assert.Greater(t, len(sips), 0)
	pp.Println("RELATED SIPS", sips)
}

func TestRouterScalarSearchEvents(t *testing.T) {
	srv := newTestHTTPServer(t)
	params := url.Values{}
	for _, tag := range testScalarTags {
		params.Add("tags", tag)
	}
	params.Set("from", testSearchFrom().Format("2006-01-02"))

	status, body := routerGET(t, srv.URL, routeEvents, params, "")
	requireStatus(t, http.StatusOK, status, body)
	sips := parseSipDigestArray(t, body)
	assert.Greater(t, len(sips), 0)
	pp.Println("SIPS", sips)
}

// Tag filter on /events (scalar tags &&). FTS is only available via the DB layer.
func TestRouterSearchSipsByTags(t *testing.T) {
	srv := newTestHTTPServer(t)
	params := url.Values{}
	for _, tag := range testTextTags {
		params.Add("tags", tag)
	}
	params.Set("from", testSearchFrom().Format("2006-01-02"))

	status, body := routerGET(t, srv.URL, routeEvents, params, "")
	requireStatus(t, http.StatusOK, status, body)
	sips := parseSipDigestArray(t, body)
	assert.Greater(t, len(sips), 0)
	pp.Println("SIPS", sips)
}

func TestRouterVectorSearchEvents(t *testing.T) {
	srv := newTestHTTPServer(t)
	params := url.Values{}
	params.Set("q", testVectorQuery)
	params.Set("acc", "0.6")
	params.Set("limit", "5")

	status, body := routerGET(t, srv.URL, routeEvents, params, "")
	requireStatus(t, http.StatusOK, status, body)
	sips := parseSipDigestArray(t, body)
	assert.Greater(t, len(sips), 0)
	pp.Println("SIPS", sips)
}

func TestRouterVectorSearchSignals(t *testing.T) {
	srv := newTestHTTPServer(t)
	params := url.Values{}
	params.Set("q", testVectorQuery)
	params.Set("acc", "0.6")
	params.Set("limit", "5")

	status, body := routerGET(t, srv.URL, routeSignals, params, "")
	requireStatus(t, http.StatusOK, status, body)
	sips := parseSipDigestArray(t, body)
	assert.Greater(t, len(sips), 0)
	pp.Println("SIPS", sips)
}

func TestRouterScalarSearchEventsText(t *testing.T) {
	srv := newTestHTTPServer(t)
	params := url.Values{}
	for _, tag := range testScalarTags {
		params.Add("tags", tag)
	}
	params.Set("from", testSearchFrom().Format("2006-01-02"))
	params.Set("response_type", "text")

	status, body := routerGET(t, srv.URL, routeEvents, params, "")
	requireStatus(t, http.StatusOK, status, body)
	text := string(body)
	assert.Contains(t, text, "reported:")
	pp.Println("TEXT", text)
}

func TestRouterVectorSearchSignalsText(t *testing.T) {
	srv := newTestHTTPServer(t)
	params := url.Values{}
	params.Set("q", testVectorQuery)
	params.Set("acc", "0.6")
	params.Set("limit", "5")
	params.Set("response_type", "text")

	status, body := routerGET(t, srv.URL, routeSignals, params, "")
	requireStatus(t, http.StatusOK, status, body)
	text := string(body)
	assert.Contains(t, text, "reported:")
	pp.Println("TEXT", text)
}

// --- stress tests against a live server ---

type stressResult struct {
	endpoint   string
	statusCode int
	latency    time.Duration
	err        error
	itemCount  int
}

func buildStressURL(baseURL string, ep stressEndpoint, rng *rand.Rand) string {
	params := url.Values{}
	path := ep.path

	if ep.isRelated {
		path = relatedPath(relatedRelationships[rng.Intn(len(relatedRelationships))])
		n := 1 + rng.Intn(len(testRelatedIDs))
		for i := 0; i < n; i++ {
			params.Add("ids", testRelatedIDs[i].String())
		}
	}

	if ep.acceptsQ && rng.Intn(2) == 0 {
		params.Set("q", sampleQueries[rng.Intn(len(sampleQueries))])
	}

	if ep.acceptsTags && rng.Intn(2) == 0 {
		n := 1 + rng.Intn(min(3, len(sampleTags)))
		perm := rng.Perm(len(sampleTags))
		for i := 0; i < n; i++ {
			params.Add("tags", sampleTags[perm[i]])
		}
	}

	if ep.acceptsFrom && rng.Intn(2) == 0 {
		daysAgo := 1 + rng.Intn(30)
		params.Set("from", time.Now().UTC().AddDate(0, 0, -daysAgo).Format("2006-01-02"))
	}

	params.Set("limit", strconv.Itoa(1+rng.Intn(50)))
	if rng.Intn(4) == 0 {
		params.Set("offset", strconv.Itoa(rng.Intn(20)))
	}

	raw := strings.TrimSuffix(baseURL, "/") + path
	if enc := params.Encode(); enc != "" {
		raw += "?" + enc
	}
	return raw
}

func runStressTest(baseURL string, concurrency int, apiKey string) []stressResult {
	results := make([]stressResult, concurrency)
	var wg sync.WaitGroup
	wg.Add(concurrency)

	client := &http.Client{Timeout: httpTimeout}
	masterRng := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec
	seeds := make([]int64, concurrency)
	for i := range seeds {
		seeds[i] = masterRng.Int63()
	}

	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			defer wg.Done()

			rng := rand.New(rand.NewSource(seeds[idx])) //nolint:gosec
			ep := stressEndpoints[rng.Intn(len(stressEndpoints))]
			rawURL := buildStressURL(baseURL, ep, rng)

			parsed, err := url.Parse(rawURL)
			if err != nil {
				results[idx] = stressResult{endpoint: ep.path, err: err}
				return
			}
			endpoint := parsed.Path

			req, err := http.NewRequest(http.MethodGet, rawURL, nil)
			if err != nil {
				results[idx] = stressResult{endpoint: endpoint, err: err}
				return
			}
			if apiKey != "" {
				req.Header.Set("X-API-KEY", apiKey)
			}

			start := time.Now()
			resp, err := client.Do(req)
			latency := time.Since(start)

			if err != nil {
				results[idx] = stressResult{endpoint: endpoint, latency: latency, err: err}
				return
			}

			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			count := 0
			var arr []any
			if json.Unmarshal(body, &arr) == nil {
				count = len(arr)
			}

			results[idx] = stressResult{
				endpoint:   endpoint,
				statusCode: resp.StatusCode,
				latency:    latency,
				itemCount:  count,
			}
		}(i)
	}

	wg.Wait()
	return results
}

func printStressSummary(t *testing.T, results []stressResult) {
	t.Helper()

	type epStats struct {
		total, success, failures int
		totalMs, totalItems      int64
	}

	stats := map[string]*epStats{}
	for _, ep := range stressEndpoints {
		stats[ep.path] = &epStats{}
	}
	stats[relatedPath(relSameAs)] = &epStats{}
	stats[relatedPath(relDerivedFrom)] = &epStats{}

	totalSuccess, totalFailure := 0, 0
	for _, r := range results {
		s, ok := stats[r.endpoint]
		if !ok {
			s = &epStats{}
			stats[r.endpoint] = s
		}
		s.total++
		s.totalMs += r.latency.Milliseconds()
		s.totalItems += int64(r.itemCount)

		if r.err != nil || r.statusCode >= 500 {
			s.failures++
			totalFailure++
		} else {
			s.success++
			totalSuccess++
		}
	}

	t.Log("=== Stress Test Summary ===")
	t.Logf("Total requests: %d | Success: %d | Failure: %d",
		len(results), totalSuccess, totalFailure)

	var totalItems int64
	for _, r := range results {
		totalItems += int64(r.itemCount)
	}
	t.Logf("Total items received: %d", totalItems)
	t.Log("--- Per-endpoint breakdown ---")

	seen := make([]string, 0, len(stats))
	for _, ep := range stressEndpoints {
		seen = append(seen, ep.path)
	}
	seen = append(seen, relatedPath(relSameAs), relatedPath(relDerivedFrom))

	for _, path := range seen {
		s := stats[path]
		if s == nil || s.total == 0 {
			continue
		}
		avgMs, avgItems := int64(0), int64(0)
		if s.total > 0 {
			avgMs = s.totalMs / int64(s.total)
			avgItems = s.totalItems / int64(s.total)
		}
		t.Logf("  %-32s  total=%-5d  ok=%-5d  err=%-5d  avg_latency=%dms  avg_items=%d",
			path, s.total, s.success, s.failures, avgMs, avgItems)
	}

	if len(results) > 0 {
		failRate := float64(totalFailure) / float64(len(results))
		if failRate > 0.10 {
			t.Errorf("stress test failure rate %.1f%% exceeds 10%% threshold", failRate*100)
		}
	}
}

func concurrencyFromEnv() int {
	raw := os.Getenv("STRESS_CONCURRENCY")
	if raw == "" {
		return 200
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < minConcurrency {
		return minConcurrency
	}
	if n > maxConcurrency {
		return maxConcurrency
	}
	return n
}

func stressBaseURL() string {
	if baseURL := os.Getenv("STRESS_BASE_URL"); baseURL != "" {
		return baseURL
	}
	return defaultStressBaseURL
}

func skipIfStressServerUnreachable(t *testing.T, baseURL string) {
	t.Helper()
	client := &http.Client{Timeout: httpTimeout}
	if _, err := client.Get(baseURL + "/health"); err != nil {
		t.Skipf("API server not reachable at %s (%v) — skipping stress test", baseURL, err)
	}
}

func stressEndpointFailures(results []stressResult) map[string]int {
	failures := map[string]int{}
	for _, r := range results {
		if r.err != nil || r.statusCode >= 500 {
			failures[r.endpoint]++
		}
	}
	return failures
}

func TestStressAPI(t *testing.T) {
	baseURL := stressBaseURL()
	apiKey := os.Getenv("STRESS_API_KEY")
	concurrency := concurrencyFromEnv()

	t.Logf("Stress testing %s with %d concurrent requests", baseURL, concurrency)
	skipIfStressServerUnreachable(t, baseURL)

	results := runStressTest(baseURL, concurrency, apiKey)
	printStressSummary(t, results)
}

func TestStressAPIEndpoints(t *testing.T) {
	baseURL := stressBaseURL()
	apiKey := os.Getenv("STRESS_API_KEY")
	skipIfStressServerUnreachable(t, baseURL)

	const requestsPerEndpoint = 10
	concurrency := len(stressEndpoints) * requestsPerEndpoint

	t.Logf("Endpoint smoke stress: %d endpoints × %d requests = %d total",
		len(stressEndpoints), requestsPerEndpoint, concurrency)

	results := runStressTest(baseURL, concurrency, apiKey)
	printStressSummary(t, results)

	failures := stressEndpointFailures(results)
	for _, ep := range stressEndpoints {
		paths := []string{ep.path}
		if ep.isRelated {
			paths = []string{relatedPath(relSameAs), relatedPath(relDerivedFrom)}
		}
		for _, path := range paths {
			path := path
			t.Run(fmt.Sprintf("endpoint=%s", path), func(t *testing.T) {
				if f := failures[path]; f > 0 {
					t.Errorf("%s had %d failure(s)", path, f)
				}
			})
		}
	}
}

func TestStressVectorSearch(t *testing.T) {
	baseURL := stressBaseURL()
	apiKey := os.Getenv("STRESS_API_KEY")
	concurrency := concurrencyFromEnv()
	skipIfStressServerUnreachable(t, baseURL)

	t.Logf("Vector search stress testing with %d concurrent requests", concurrency)

	results := make([]stressResult, concurrency)
	var wg sync.WaitGroup
	wg.Add(concurrency)

	client := &http.Client{Timeout: httpTimeout}
	masterRng := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec
	seeds := make([]int64, concurrency)
	for i := range seeds {
		seeds[i] = masterRng.Int63()
	}

	vectorEndpoints := []string{routeEvents, routeSignals}

	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			defer wg.Done()

			rng := rand.New(rand.NewSource(seeds[idx])) //nolint:gosec
			endpoint := vectorEndpoints[rng.Intn(len(vectorEndpoints))]

			params := url.Values{}
			params.Set("q", sampleQueries[rng.Intn(len(sampleQueries))])
			params.Set("limit", strconv.Itoa(1+rng.Intn(50)))

			rawURL := strings.TrimSuffix(baseURL, "/") + endpoint + "?" + params.Encode()
			req, err := http.NewRequest(http.MethodGet, rawURL, nil)
			if err != nil {
				results[idx] = stressResult{endpoint: endpoint, err: err}
				return
			}
			if apiKey != "" {
				req.Header.Set("X-API-KEY", apiKey)
			}

			start := time.Now()
			resp, err := client.Do(req)
			latency := time.Since(start)

			if err != nil {
				results[idx] = stressResult{endpoint: endpoint, latency: latency, err: err}
				return
			}

			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			count := 0
			var arr []any
			if json.Unmarshal(body, &arr) == nil {
				count = len(arr)
			}

			results[idx] = stressResult{
				endpoint:   endpoint,
				statusCode: resp.StatusCode,
				latency:    latency,
				itemCount:  count,
			}
		}(i)
	}

	wg.Wait()
	printStressSummary(t, results)
}
