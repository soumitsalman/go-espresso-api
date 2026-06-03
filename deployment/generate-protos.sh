#!/bin/bash
# Generate protobuf code from Hugging Face TEI proto

set -e

mkdir -p .tools/bin
mkdir -p nlp/.proto

# Download protoc if not present
if [ ! -x .tools/protoc/bin/protoc ]; then
  echo "Downloading protoc..."
  curl -L -o /tmp/protoc.zip https://github.com/protocolbuffers/protobuf/releases/download/v28.3/protoc-28.3-linux-x86_64.zip
  unzip -o /tmp/protoc.zip -d .tools/protoc
  rm /tmp/protoc.zip
fi

# Install Go plugins if not present
if [ ! -x .tools/bin/protoc-gen-go ]; then
  echo "Installing protoc-gen-go..."
  GOBIN=$PWD/.tools/bin go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
fi

if [ ! -x .tools/bin/protoc-gen-go-grpc ]; then
  echo "Installing protoc-gen-go-grpc..."
  GOBIN=$PWD/.tools/bin go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
fi

# Download latest TEI proto from Hugging Face repo
echo "Downloading latest TEI proto from Hugging Face..."
curl -L -o nlp/.proto/tei.proto https://raw.githubusercontent.com/huggingface/text-embeddings-inference/main/proto/tei.proto

# Add go_package option to tei.proto if not already present (insert after syntax line)
if ! grep -q "option go_package" nlp/.proto/tei.proto; then
  sed -i.bak '1 a option go_package = "github.com/soumitsalman/gobeansack/nlp";' nlp/.proto/tei.proto
  rm -f nlp/.proto/tei.proto.bak
fi

# Generate protos
echo "Generating protobuf code..."
PATH=$PWD/.tools/bin:$PWD/.tools/protoc/bin:$PATH protoc \
  --go_out=paths=source_relative:nlp \
  --go-grpc_out=paths=source_relative:nlp \
  -I nlp/.proto nlp/.proto/tei.proto

echo "✓ Proto generation complete"
