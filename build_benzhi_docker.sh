#!/bin/bash
set -e

IMAGE_NAME=${1:-task160-migimpact}
DOCKER_PLATFORM=${2:-linux/amd64}

docker build --platform "$DOCKER_PLATFORM" -f benzhi.Dockerfile -t "$IMAGE_NAME" .

echo ""
echo "✅ Docker image '$IMAGE_NAME' built successfully for $DOCKER_PLATFORM!"
echo ""
echo "📋 Next steps (for testing):"
echo "  • smoke test：docker run --rm $IMAGE_NAME --smoke-test"
echo "  • interactive shell：docker run -it $IMAGE_NAME sh"
