#!/bin/bash

# S3FS Debug Script
# This script helps debug s3fs integration with s3-proxy

set -e

echo "=== S3FS Debug Script ==="

# Configuration
BUCKET_NAME=${BUCKET_NAME:-"test-bucket"}
MOUNT_POINT=${MOUNT_POINT:-"/tmp/s3fs-test"}
ACCESS_KEY=${ACCESS_KEY:-"AKIAEXAMPLEACCESSKEY1"}
SECRET_KEY=${SECRET_KEY:-"dummysecret"}
ENDPOINT=${ENDPOINT:-"http://localhost:8080"}
REGION=${REGION:-"us-east-1"}

echo "Configuration:"
echo "  Bucket: $BUCKET_NAME"
echo "  Mount Point: $MOUNT_POINT"
echo "  Access Key: $ACCESS_KEY"
echo "  Endpoint: $ENDPOINT"
echo "  Region: $REGION"

# Create mount point if it doesn't exist
mkdir -p "$MOUNT_POINT"

# Create s3fs credentials file
CREDENTIALS_FILE="/tmp/s3fs-passwd"
echo "$ACCESS_KEY:$SECRET_KEY" > "$CREDENTIALS_FILE"
chmod 600 "$CREDENTIALS_FILE"

echo ""
echo "=== Mounting s3fs ==="

# Unmount if already mounted
if mountpoint -q "$MOUNT_POINT"; then
    echo "Unmounting existing mount..."
    fusermount -u "$MOUNT_POINT" || sudo umount "$MOUNT_POINT"
fi

# Mount with debug options
echo "Mounting s3fs with debug options..."
s3fs "$BUCKET_NAME" "$MOUNT_POINT" \
    -o passwd_file="$CREDENTIALS_FILE" \
    -o url="$ENDPOINT" \
    -o use_path_request_style \
    -o sigv4 \
    -o dbglevel=info \
    -o curldbg \
    -o f2 \
    -o allow_other

echo "Mount successful!"

echo ""
echo "=== Testing Operations ==="

# Test 1: List directory
echo "1. Testing directory listing..."
ls -la "$MOUNT_POINT"

# Test 2: Write a file
echo "2. Testing file write..."
echo "Hello from s3fs debug test" > "$MOUNT_POINT/debug-test.txt"
echo "Write successful!"

# Test 3: List directory again
echo "3. Testing directory listing after write..."
ls -la "$MOUNT_POINT"

# Test 4: Read the file (this is where the issue likely occurs)
echo "4. Testing file read (cat)..."
echo "File content:"
cat "$MOUNT_POINT/debug-test.txt"

echo ""
echo "=== Debug Complete ==="
echo "If you see this message, all operations completed successfully!"
echo "Check the s3-proxy logs to see what requests were made."

# Cleanup
echo ""
echo "=== Cleanup ==="
echo "Unmounting..."
fusermount -u "$MOUNT_POINT" || sudo umount "$MOUNT_POINT"
rm -f "$CREDENTIALS_FILE"
echo "Cleanup complete!" 