#!/bin/bash

set -e

echo "Building Docker Router Discovery Container..."

# Build the discovery container
cd discovery
docker build -t docker-router-discovery:latest .

echo "✓ Discovery container built successfully"

# Build for different architectures (optional)
if [ "$1" = "multi-arch" ]; then
    echo "Building multi-architecture images..."
    docker buildx build --platform linux/amd64,linux/arm64 -t docker-router-discovery:latest .
    echo "✓ Multi-architecture images built"
fi

echo "Build complete!"
echo ""
echo "To test the discovery service:"
echo "  cd examples/simple-test"
echo "  docker-compose up"
echo ""
echo "To test multi-stack discovery:"
echo "  cd examples/multi-stack"
echo "  docker-compose -f docker-compose.stack-a.yml up -d"
echo "  docker-compose -f docker-compose.stack-b.yml up -d"