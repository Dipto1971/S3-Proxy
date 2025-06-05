#!/bin/bash

# Test GET operations with curl
# This helps verify that the s3-proxy GET functionality works correctly

set -e

echo "=== S3-Proxy GET Test ==="

# Configuration
BUCKET_NAME=${BUCKET_NAME:-"test-bucket"}
ACCESS_KEY=${ACCESS_KEY:-"AKIAEXAMPLEACCESSKEY1"}
ENDPOINT=${ENDPOINT:-"http://localhost:8080"}
TEST_FILE="test-get-file.txt"
TEST_CONTENT="Hello from GET test!"

echo "Configuration:"
echo "  Bucket: $BUCKET_NAME"
echo "  Access Key: $ACCESS_KEY"
echo "  Endpoint: $ENDPOINT"
echo "  Test File: $TEST_FILE"

# Create auth header
AUTH_HEADER="AWS4-HMAC-SHA256 Credential=${ACCESS_KEY}/20250516/us-east-1/s3/aws4_request"

echo ""
echo "=== Step 1: PUT test file ==="
echo "Uploading test content..."

curl -v -X PUT \
    -H "Authorization: ${AUTH_HEADER}" \
    -H "Content-Type: text/plain" \
    -d "${TEST_CONTENT}" \
    "${ENDPOINT}/${BUCKET_NAME}/${TEST_FILE}"

echo ""
echo "PUT completed!"

echo ""
echo "=== Step 2: GET test file ==="
echo "Downloading test content..."

RESPONSE=$(curl -v -X GET \
    -H "Authorization: ${AUTH_HEADER}" \
    "${ENDPOINT}/${BUCKET_NAME}/${TEST_FILE}")

echo ""
echo "GET completed!"
echo "Response content: ${RESPONSE}"

echo ""
echo "=== Step 3: Verify content ==="
if [ "${RESPONSE}" = "${TEST_CONTENT}" ]; then
    echo "✅ SUCCESS: Content matches!"
    echo "Expected: ${TEST_CONTENT}"
    echo "Received: ${RESPONSE}"
else
    echo "❌ FAILURE: Content mismatch!"
    echo "Expected: ${TEST_CONTENT}"
    echo "Received: ${RESPONSE}"
    exit 1
fi

echo ""
echo "=== Test Complete ==="
echo "The s3-proxy GET functionality is working correctly!"
echo "If s3fs still doesn't work, the issue is likely in s3fs configuration or behavior." 