package nlp

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	_TIMEOUT          = 10 * time.Minute
	_DEFAULT_BASE_URL = "localhost"
)

type Embedder interface {
	EmbedQuery(ctx context.Context, query string) []float32
	EmbedDocuments(ctx context.Context, docs []string) [][]float32
	Close() error
}

type RemoteEmbedder struct {
	conn   *grpc.ClientConn
	client EmbedClient
}

// NewRemoteEmbedder creates a new embedder that connects to Hugging Face TEI via gRPC
// baseURL should be in format "hostname:port" (e.g., "localhost:10000")
// model and apiKey parameters are ignored for TEI but kept for backward compatibility
func NewRemoteEmbedder(baseURL, apiKey, model string) *RemoteEmbedder {
	if baseURL == "" {
		baseURL = _DEFAULT_BASE_URL
	}

	conn, err := grpc.NewClient(
		baseURL,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Error().Str("module", "EMBEDDER").Err(err).Msg("failed to connect to TEI gRPC server")
	}

	return &RemoteEmbedder{
		conn:   conn,
		client: NewEmbedClient(conn),
	}
}

// EmbedQuery embeds a single query string
func (e *RemoteEmbedder) EmbedQuery(ctx context.Context, query string) []float32 {
	ctx, cancel := context.WithTimeout(ctx, _TIMEOUT)
	defer cancel()

	resp, err := e.client.Embed(ctx, &EmbedRequest{
		Inputs: query,
	})
	if err != nil {
		log.Error().Str("module", "EMBEDDER").Err(err).Msg("failed to embed query")
		return nil
	}

	if len(resp.Embeddings) == 0 {
		log.Error().Str("module", "EMBEDDER").Msg("empty embedding response from TEI")
		return nil
	}

	return resp.Embeddings
}

// EmbedDocuments embeds multiple documents by calling Embed for each
// (TEI's gRPC Embed service processes one input at a time; use EmbedStream for batching)
func (e *RemoteEmbedder) EmbedDocuments(ctx context.Context, docs []string) [][]float32 {
	if len(docs) == 0 {
		return nil
	}

	result := make([][]float32, 0, len(docs))
	for _, doc := range docs {
		embedding := e.EmbedQuery(ctx, doc)
		if embedding != nil {
			result = append(result, embedding)
		}
	}

	return result
}

// Close closes the gRPC connection
func (e *RemoteEmbedder) Close() error {
	if e.conn != nil {
		return e.conn.Close()
	}
	return nil
}
