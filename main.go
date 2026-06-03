package main

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
	"github.com/soumitsalman/espressoapi/cupboard"
	_ "github.com/soumitsalman/espressoapi/docs"
	"github.com/soumitsalman/espressoapi/nlp"
	r "github.com/soumitsalman/espressoapi/router"
)

const (
	DEFAULT_PORT = "8080"
)

func main() {
	_ = godotenv.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	connStr := getEnv("PG_CONNECTION_STRING", "", true)
	db := cupboard.NewCupboard(ctx, connStr)
	defer db.Close()

	// determine concurrency limit from environment
	maxStr := getEnv("MAX_CONCURRENT_REQUESTS", "", false)
	max_requests, err := strconv.Atoi(maxStr)
	if err != nil && max_requests < 0 {
		max_requests = 0
	}

	api := r.NewRouter(
		db,
		nlp.NewRemoteEmbedder(getEnv("EMBEDDER_BASE_URL", "", true), getEnv("EMBEDDER_API_KEY", "", false), getEnv("EMBEDDER_MODEL", "", false)),
		parseAPIKeys(os.Getenv("API_KEYS")),
		max_requests,
	)

	port := getEnv("PORT", DEFAULT_PORT, false)
	addr := "0.0.0.0:" + port
	log.Info().Str("module", "MAIN").Str("addr", addr).Msg("Routes Initialized. Server starting")

	if err := api.Run(addr); err != nil {
		log.Fatal().Str("module", "MAIN").Err(err).Msg("server error")
	}
}

func getEnv(name, fallback string, must_exist bool) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	if must_exist {
		log.Fatal().Str("module", "MAIN").Msgf("%s is required", name)
	}
	return fallback
}

func parseAPIKeys(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	result := map[string]string{}
	for _, pair := range strings.Split(raw, ";") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		header := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if header != "" && value != "" {
			result[header] = value
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
