#!/bin/bash

set -e

echo "Testing Docker Router Discovery..."

# Function to clean up
cleanup() {
    echo "Cleaning up test environments..."
    cd ../examples/multi-stack
    docker-compose -f docker-compose.stack-a.yml down -v 2>/dev/null || true
    docker-compose -f docker-compose.stack-b.yml down -v 2>/dev/null || true
    cd ../simple-test
    docker-compose down -v 2>/dev/null || true
}

# Set up cleanup trap
trap cleanup EXIT

echo "Building discovery container..."
cd discovery
docker build -t docker-router-discovery:latest .

echo "Testing single stack discovery..."
cd ../examples/simple-test
docker-compose up -d

echo "Waiting for services to start..."
sleep 10

echo "Checking discovery data..."
docker-compose exec test-a cat /shared/discovery.json || echo "No discovery data yet"

echo "Testing multi-stack discovery..."
docker-compose down -v

cd ../multi-stack
docker-compose -f docker-compose.stack-a.yml up -d
sleep 5
docker-compose -f docker-compose.stack-b.yml up -d

echo "Waiting for peer discovery..."
sleep 20

echo "Stack A discovered peers:"
docker-compose -f docker-compose.stack-a.yml exec test-a cat /shared/discovery.json | grep -A 10 '"peers"' || echo "No peers found"

echo "Stack B discovered peers:"
docker-compose -f docker-compose.stack-b.yml exec test-b cat /shared/discovery.json | grep -A 10 '"peers"' || echo "No peers found"

echo "Test complete!"