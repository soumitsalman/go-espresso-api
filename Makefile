.PHONY: generate-protos build clean help

help:
	@echo "Available targets:"
	@echo "  make generate-protos  - Regenerate protobuf code from .proto files"
	@echo "  make build            - Build the application"
	@echo "  make docker-build     - Build Docker image"
	@echo "  make clean            - Remove build artifacts and .tools directory"
	@echo "  make run              - Run the application locally"

generate-protos:
	@./deployment/generate-protos.sh

build: generate-protos
	@go build -o espressoapi .

docker-build: generate-protos
	@docker build -t soumitsr/go-beans-api:latest .

run: generate-protos
	@go run main.go

clean:
	@rm -rf espressoapi .tools
	@echo "✓ Cleaned build artifacts"
