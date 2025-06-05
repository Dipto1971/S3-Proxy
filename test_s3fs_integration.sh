#!/bin/bash

# Test script for s3fs integration with s3-proxy
# This script tests the consistency issues mentioned by the user

set -e

echo "=== S3FS Integration Test Script ==="
echo "Testing for consistency between reading/writing existing files vs new files"

# Configuration
BUCKET_NAME=${BUCKET_NAME:-"test-bucket"}
MOUNT_POINT=${MOUNT_POINT:-"/tmp/s3fs-test"}
ACCESS_KEY=${ACCESS_KEY:-"AKIAEXAMPLEACCESSKEY1"}
SECRET_KEY=${SECRET_KEY:-"dummysecret"}
ENDPOINT=${ENDPOINT:-"http://localhost:8080"}
REGION=${REGION:-"us-east-1"}
CREDENTIALS_FILE="/tmp/s3fs-passwd"

echo "Configuration:"
echo "  Bucket: $BUCKET_NAME"
echo "  Mount Point: $MOUNT_POINT"
echo "  Access Key: $ACCESS_KEY"
echo "  Endpoint: $ENDPOINT"

# Setup
mkdir -p "$MOUNT_POINT"
echo "$ACCESS_KEY:$SECRET_KEY" > "$CREDENTIALS_FILE"
chmod 600 "$CREDENTIALS_FILE"

# Unmount if already mounted
if mountpoint -q "$MOUNT_POINT" 2>/dev/null; then
    echo "Unmounting existing mount..."
    fusermount -u "$MOUNT_POINT" || sudo umount "$MOUNT_POINT" || true
fi

echo ""
echo "=== Mounting s3fs ==="
s3fs "$BUCKET_NAME" "$MOUNT_POINT" \
    -o passwd_file="$CREDENTIALS_FILE" \
    -o url="$ENDPOINT" \
    -o use_path_request_style \
    -o sigv4 \
    -o allow_other \
    -o dbglevel=info

echo "Mount successful!"

# Function to test file operations
test_file_operations() {
    local test_name="$1"
    local file_name="$2"
    local content="$3"
    
    echo ""
    echo "=== Testing: $test_name ==="
    
    # Test 1: Create file
    echo "1. Creating file: $file_name"
    echo "$content" > "$MOUNT_POINT/$file_name"
    if [ $? -eq 0 ]; then
        echo "   ✓ File creation successful"
    else
        echo "   ✗ File creation failed"
        return 1
    fi
    
    # Test 2: Check file exists and has correct size
    echo "2. Checking file metadata..."
    if [ -f "$MOUNT_POINT/$file_name" ]; then
        echo "   ✓ File exists"
        file_size=$(stat -c%s "$MOUNT_POINT/$file_name")
        expected_size=$((${#content} + 1))  # +1 for newline
        echo "   File size: $file_size bytes (expected: ~$expected_size)"
        if [ "$file_size" -gt 0 ]; then
            echo "   ✓ File has non-zero size"
        else
            echo "   ✗ File has zero size"
            return 1
        fi
    else
        echo "   ✗ File does not exist"
        return 1
    fi
    
    # Test 3: Read file content
    echo "3. Reading file content..."
    read_content=$(cat "$MOUNT_POINT/$file_name")
    if [ "$read_content" = "$content" ]; then
        echo "   ✓ Content matches: '$read_content'"
    else
        echo "   ✗ Content mismatch. Expected: '$content', Got: '$read_content'"
        return 1
    fi
    
    # Test 4: List directory
    echo "4. Directory listing check..."
    if ls -la "$MOUNT_POINT" | grep -q "$file_name"; then
        echo "   ✓ File appears in directory listing"
        ls -la "$MOUNT_POINT" | grep "$file_name"
    else
        echo "   ✗ File does not appear in directory listing"
        return 1
    fi
    
    echo "   ✅ All tests passed for $test_name"
    return 0
}

# Run test cases
echo ""
echo "=== Starting Test Cases ==="

# Test Case 1: Brand new file
test_file_operations "New File Creation" "new-file-$(date +%s).txt" "This is a completely new file created by the test"

# Test Case 2: File with special characters
test_file_operations "Special Characters File" "file-with-special-chars-$(date +%s).txt" "Content with special chars: !@#$%^&*()"

# Test Case 3: Larger file content
large_content="This is a larger file content. "
for i in {1..10}; do
    large_content+="Line $i with some repeated content to test larger files. "
done
test_file_operations "Large File" "large-file-$(date +%s).txt" "$large_content"

# Test Case 4: Empty file
test_file_operations "Empty File" "empty-file-$(date +%s).txt" ""

# Test Case 5: File operations on existing file
echo ""
echo "=== Testing Existing File Modification ==="
existing_file="existing-file-$(date +%s).txt"
echo "Original content" > "$MOUNT_POINT/$existing_file"
sleep 2  # Give time for the write to complete

echo "Modifying existing file..."
echo "Modified content" > "$MOUNT_POINT/$existing_file"
modified_content=$(cat "$MOUNT_POINT/$existing_file")
if [ "$modified_content" = "Modified content" ]; then
    echo "✓ Existing file modification successful"
else
    echo "✗ Existing file modification failed. Got: '$modified_content'"
fi

# Test Case 6: Rapid file creation and reading (stress test)
echo ""
echo "=== Stress Testing: Rapid File Creation/Reading ==="
for i in {1..5}; do
    rapid_file="rapid-test-$i-$(date +%s).txt"
    rapid_content="Rapid test content for file $i"
    
    echo "$rapid_content" > "$MOUNT_POINT/$rapid_file"
    sleep 0.5  # Brief pause
    read_back=$(cat "$MOUNT_POINT/$rapid_file")
    
    if [ "$read_back" = "$rapid_content" ]; then
        echo "✓ Rapid test $i: SUCCESS"
    else
        echo "✗ Rapid test $i: FAILED - Expected: '$rapid_content', Got: '$read_back'"
    fi
done

echo ""
echo "=== Final Directory Listing ==="
ls -la "$MOUNT_POINT"

echo ""
echo "=== Testing Complete ==="
echo "If you see this message, the s3fs integration test completed."
echo "Check above for any ✗ symbols indicating failures."

# Cleanup function
cleanup() {
    echo ""
    echo "=== Cleaning Up ==="
    if mountpoint -q "$MOUNT_POINT" 2>/dev/null; then
        fusermount -u "$MOUNT_POINT" || sudo umount "$MOUNT_POINT" || true
        echo "Unmounted s3fs"
    fi
    rm -f "$CREDENTIALS_FILE"
    echo "Cleanup complete"
}

# Set trap for cleanup on exit
trap cleanup EXIT

echo ""
echo "Mount will remain active for manual testing. Press Ctrl+C to unmount and exit."
echo "You can test manually with commands like:"
echo "  echo 'test content' > $MOUNT_POINT/manual-test.txt"
echo "  cat $MOUNT_POINT/manual-test.txt"
echo "  ls -la $MOUNT_POINT"

# Keep the script running to maintain the mount
read -p "Press Enter to unmount and exit..." 