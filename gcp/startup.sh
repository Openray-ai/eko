#!/bin/bash

# ============================================================================
# Ekō VM Startup Script for GCP Compute Engine
# This script runs when the VM starts or restarts
# ============================================================================

set -e  # Exit on error

echo "Starting Ekō deployment..."

# ============================================================================
# Docker Configuration
# ============================================================================
# Note: Container-Optimized OS has Docker pre-installed
# For Ubuntu/Debian VMs, uncomment the following:
# curl -fsSL https://get.docker.com -o get-docker.sh && sh get-docker.sh

# Get project ID from instance metadata
PROJECT_ID=$(curl -s "http://metadata.google.internal/computeMetadata/v1/project/project-id" -H "Metadata-Flavor: Google")
REGION=${REGION}

echo "Project ID: $PROJECT_ID"
echo "Region: $REGION"

# ============================================================================
# Configure Docker Authentication for Artifact Registry
# ============================================================================
echo "Configuring Docker authentication..."
gcloud auth configure-docker ${REGION}-docker.pkg.dev --quiet

# ============================================================================
# Pull Latest Docker Image
# ============================================================================
echo "Pulling latest Docker image..."
docker pull ${REGION}-docker.pkg.dev/${PROJECT_ID}/eko-repo/eko:latest

# ============================================================================
# Fetch Configuration from Secret Manager
# ============================================================================
echo "Fetching configuration from Secret Manager..."
gcloud secrets versions access latest --secret="eko-config" > /tmp/config.yaml

# Optional: Fetch API keys if stored separately
# gcloud secrets versions access latest --secret="eko-openai-key" > /tmp/openai-key.txt
# export OPENAI_API_KEY=$(cat /tmp/openai-key.txt)

# ============================================================================
# Stop and Remove Existing Container (if any)
# ============================================================================
echo "Stopping existing container..."
docker stop eko 2>/dev/null || true
docker rm eko 2>/dev/null || true

# ============================================================================
# Run New Container
# ============================================================================
echo "Starting new container..."
docker run -d \
  --name eko \
  --restart unless-stopped \
  -p 8080:8080 \
  -v /tmp/config.yaml:/app/configs/config.yaml:ro \
  --log-driver=gcplogs \
  ${REGION}-docker.pkg.dev/${PROJECT_ID}/eko-repo/eko:latest

# ============================================================================
# Health Check
# ============================================================================
echo "Waiting for application to start..."
sleep 5

echo "Running health check..."
if curl -f http://localhost:8080/health; then
    echo "✅ Ekō deployment successful!"
    echo "Health check passed"
else
    echo "❌ Health check failed"
    echo "Checking container logs..."
    docker logs eko
    exit 1
fi

echo "Ekō is running and healthy"
