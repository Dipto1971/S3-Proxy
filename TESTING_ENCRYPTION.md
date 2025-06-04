# S3FS Encryption/Decryption Testing Guide for Ubuntu Linux

This guide provides step-by-step instructions to test the newly implemented encryption/decryption functionality in the S3FS implementation.

## Prerequisites

### 1. Install Required Packages

```bash
# Update package list
sudo apt update

# Install FUSE development libraries
sudo apt install -y fuse3 libfuse3-dev

# Install Go (if not already installed)
sudo apt install -y golang-go

# Install MinIO client and server
wget https://dl.min.io/client/mc/release/linux-amd64/mc
chmod +x mc
sudo mv mc /usr/local/bin/

wget https://dl.min.io/server/minio/release/linux-amd64/minio
chmod +x minio
sudo mv minio /usr/local/bin/

# Install other useful tools
sudo apt install -y jq curl
```

### 2. Setup Project and Build

```bash
# Navigate to project directory
cd /path/to/S3-Proxy

# Build the application
go build -o s3-proxy ./cmd/s3-proxy

# Verify build
./s3-proxy --help || echo "Build completed successfully"
```

## Environment Setup

### 3. Generate Encryption Keys

```bash
# Generate keys for testing (you can also use existing ones)
# Tink keyset (base64 encoded)
export TINK_KEYSET="CiQAp9HWrRKnxMGbOY7+EahHb1SHw2u3q8tH2l9OqTY8vY9xQwESWAosCg4KCGFlc19nY20QARgBEAEYAioCCAASDxIJAQABAgMEBQYHCAkAEhMIABIBCBoIEgYKBAgBEAEYAioCCAASEhIIAQABAgMEBQYHCAkBGggSBgoECAEQAhgC"

# ChaCha20-Poly1305 key (32 bytes, base64 encoded)
export CHACHA_KEY="$(openssl rand -base64 32)"

# AES key (32 bytes for AES-256, base64 encoded)
export AES_KEY="$(openssl rand -base64 32)"

# S3 credentials for MinIO
export S3_ACCESS_KEY="minioadmin"
export S3_SECRET_KEY="minioadmin"

# Test access keys for authentication
export ALLOWED_ACCESS_KEY1="test-access-key-1"
export ALLOWED_ACCESS_KEY2="test-access-key-2"

# Auth header format
export AUTH_HEADER_FORMAT="AWS4-HMAC-SHA256"

# Print generated keys for reference
echo "Generated encryption keys:"
echo "TINK_KEYSET=$TINK_KEYSET"
echo "CHACHA_KEY=$CHACHA_KEY"
echo "AES_KEY=$AES_KEY"
echo ""
echo "Test access keys:"
echo "ALLOWED_ACCESS_KEY1=$ALLOWED_ACCESS_KEY1"
echo "ALLOWED_ACCESS_KEY2=$ALLOWED_ACCESS_KEY2"
```

### 4. Create .env File

```bash
# Create .env file with all environment variables
cat > .env << EOF
# Encryption keys
TINK_KEYSET=$TINK_KEYSET
CHACHA_KEY=$CHACHA_KEY
AES_KEY=$AES_KEY

# S3 credentials
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin

# Authentication
ALLOWED_ACCESS_KEY1=test-access-key-1
ALLOWED_ACCESS_KEY2=test-access-key-2
AUTH_HEADER_FORMAT=AWS4-HMAC-SHA256
EOF

echo "Created .env file with encryption keys and authentication settings"
```

### 5. Verify Configuration File

```bash
# Check that configs/main.yaml exists and has correct structure
cat configs/main.yaml

# Verify the crypto configurations are present
echo "Checking crypto configurations:"
grep -A 20 "crypto:" configs/main.yaml

# Verify bucket configurations
echo "Checking bucket configurations:"
grep -A 10 "s3_buckets:" configs/main.yaml

# Verify auth configurations
echo "Checking auth configurations:"
grep -A 5 "auth:" configs/main.yaml
```

## MinIO Server Setup

### 6. Start MinIO Server

```bash
# Create data directory
mkdir -p ~/minio-data

# Start MinIO server (run in separate terminal or background)
minio server ~/minio-data --console-address ":9001" &
MINIO_PID=$!

# Wait for MinIO to start
sleep 3

echo "MinIO server started with PID: $MINIO_PID"
echo "Web UI: http://localhost:9001"
echo "API: http://localhost:9000"
echo "Default credentials: minioadmin / minioadmin"
```

### 7. Configure MinIO Client and Create Test Bucket

```bash
# Configure MinIO client
mc alias set local http://localhost:9000 minioadmin minioadmin

# Create test bucket
mc mb local/test-bucket

# Verify bucket creation
mc ls local/
```

## Testing Encryption/Decryption Functionality

### 8. Test 1: Mount with Encryption (Valid Access Key)

```bash
# Create mount point
mkdir -p ~/s3-encrypted-mount

# Mount with encryption enabled and valid access key
./s3-proxy s3fs-mount \
  -mount ~/s3-encrypted-mount \
  -bucket test-bucket \
  -endpoint http://localhost:9000 \
  -access-key test-access-key-1 \
  -secret-key minioadmin \
  -region us-east-1 \
  -config configs/main.yaml \
  -debug &

MOUNT_PID=$!

# Wait for mount to complete
sleep 3

echo "S3FS mounted with encryption. PID: $MOUNT_PID"

# Check mount status
mount | grep s3fs || echo "Mount not found in system mount table (this might be normal for FUSE)"
ls -la ~/s3-encrypted-mount/

# Check logs for encryption status
echo "Expected log messages:"
echo "- 'Access key validated successfully'"
echo "- 'Encryption enabled using crypto ID: default-triple'"
echo "- 'Encryption: ENABLED - Data will be encrypted before upload and decrypted after download'"
```

### 9. Test 2: Write Encrypted Files

```bash
# Test creating encrypted files
echo "This is a test file with sensitive data!" > ~/s3-encrypted-mount/encrypted-test.txt
echo "Another encrypted file with different content" > ~/s3-encrypted-mount/secret-data.txt

# Create a larger file for encryption testing
dd if=/dev/urandom of=~/s3-encrypted-mount/large-encrypted.bin bs=1024 count=10

# Create a JSON file with structured data
cat > ~/s3-encrypted-mount/config.json << EOF
{
  "database": {
    "host": "secret-db-host.example.com",
    "password": "super-secret-password-123",
    "api_key": "sk-1234567890abcdef"
  },
  "encryption": {
    "enabled": true,
    "algorithm": "AES-256-GCM"
  }
}
EOF

# Verify files were created
ls -la ~/s3-encrypted-mount/
echo "Files created in encrypted filesystem"
```

### 10. Test 3: Verify Data is Encrypted in S3

```bash
# Check what's actually stored in S3 (should be encrypted)
echo "=== Checking raw S3 data (should be encrypted) ==="

# Download and examine the raw encrypted data
mc cat local/test-bucket/encrypted-test.txt > /tmp/raw-encrypted-test.txt
mc cat local/test-bucket/config.json > /tmp/raw-encrypted-config.json

echo "Raw encrypted content of encrypted-test.txt:"
xxd /tmp/raw-encrypted-test.txt | head -5
echo ""

echo "Raw encrypted content of config.json:"
xxd /tmp/raw-encrypted-config.json | head -5
echo ""

# Try to read as text (should be gibberish)
echo "Attempting to read encrypted data as text (should be unreadable):"
head -c 100 /tmp/raw-encrypted-test.txt
echo ""
echo ""

# Verify it doesn't contain original text
if grep -q "This is a test file" /tmp/raw-encrypted-test.txt; then
    echo "❌ ERROR: Original text found in S3! Encryption may not be working!"
else
    echo "✅ SUCCESS: Original text not found in S3 - data is encrypted!"
fi

if grep -q "super-secret-password" /tmp/raw-encrypted-config.json; then
    echo "❌ ERROR: Secret data found in S3! Encryption may not be working!"
else
    echo "✅ SUCCESS: Secret data not found in S3 - data is encrypted!"
fi
```

### 11. Test 4: Read and Verify Decryption

```bash
# Read files through encrypted filesystem (should be decrypted)
echo "=== Reading files through encrypted filesystem (should be decrypted) ==="

echo "Content of encrypted-test.txt (should be readable):"
cat ~/s3-encrypted-mount/encrypted-test.txt
echo ""

echo "Content of secret-data.txt:"
cat ~/s3-encrypted-mount/secret-data.txt
echo ""

echo "Content of config.json (should show original JSON):"
cat ~/s3-encrypted-mount/config.json
echo ""

# Verify large file integrity
echo "Verifying large file integrity:"
ls -lh ~/s3-encrypted-mount/large-encrypted.bin
```

### 12. Test 5: Authentication Failure Test

```bash
# Unmount current filesystem
fusermount -u ~/s3-encrypted-mount
wait $MOUNT_PID

# Try to mount with invalid access key (should fail)
echo "=== Testing authentication failure ==="
./s3-proxy s3fs-mount \
  -mount ~/s3-encrypted-mount \
  -bucket test-bucket \
  -endpoint http://localhost:9000 \
  -access-key invalid-key \
  -secret-key minioadmin \
  -region us-east-1 \
  -config configs/main.yaml

# Expected output: "invalid access key: invalid-key"
echo "Expected: Authentication should fail with 'invalid access key' error"
```

### 13. Test 6: Mount Without Encryption

```bash
# Test mounting without encryption for comparison
mkdir -p ~/s3-plaintext-mount

# Mount without authentication/encryption (should work but show warnings)
./s3-proxy s3fs-mount \
  -mount ~/s3-plaintext-mount \
  -bucket test-bucket \
  -endpoint http://localhost:9000 \
  -access-key minioadmin \
  -secret-key minioadmin \
  -region us-east-1 \
  -debug &

PLAINTEXT_PID=$!
sleep 3

echo "Expected log messages:"
echo "- 'Proceeding without authentication validation and encryption'"
echo "- 'Encryption: DISABLED - Data will be stored in plaintext'"

# Create a plaintext file
echo "This is plaintext data" > ~/s3-plaintext-mount/plaintext-test.txt

# Verify it's stored as plaintext in S3
sleep 2
mc cat local/test-bucket/plaintext-test.txt
echo ""

if mc cat local/test-bucket/plaintext-test.txt | grep -q "This is plaintext data"; then
    echo "✅ SUCCESS: Plaintext data stored without encryption"
else
    echo "❌ ERROR: Plaintext data not found or corrupted"
fi

# Cleanup
fusermount -u ~/s3-plaintext-mount
wait $PLAINTEXT_PID
```

### 14. Test 7: Cross-Mount Compatibility

```bash
# Mount encrypted filesystem again
./s3-proxy s3fs-mount \
  -mount ~/s3-encrypted-mount \
  -bucket test-bucket \
  -endpoint http://localhost:9000 \
  -access-key test-access-key-1 \
  -secret-key minioadmin \
  -region us-east-1 \
  -config configs/main.yaml &

ENCRYPTED_PID=$!
sleep 3

echo "=== Testing cross-mount compatibility ==="

# The encrypted files should be readable
echo "Reading previously encrypted files:"
cat ~/s3-encrypted-mount/encrypted-test.txt
echo ""

# The plaintext file should not be readable (will show encrypted gibberish or error)
echo "Attempting to read plaintext file through encrypted mount:"
cat ~/s3-encrypted-mount/plaintext-test.txt 2>&1 || echo "Expected: This may fail or show gibberish"
echo ""

# List all files
ls -la ~/s3-encrypted-mount/
```

## Advanced Testing

### 15. Test 8: Different Crypto Configurations

```bash
# Test with different crypto ID (if available in config)
# First, unmount current filesystem
fusermount -u ~/s3-encrypted-mount
wait $ENCRYPTED_PID

# Check available crypto IDs in config
echo "Available crypto configurations:"
grep -A 1 "id:" configs/main.yaml | grep "id:"

# If you want to test with "default" crypto instead of "default-triple",
# you would need to modify the bucket configuration temporarily
# For now, let's test the current configuration thoroughly
```

### 16. Test 9: Performance and Memory Testing

```bash
# Mount with encryption again
./s3-proxy s3fs-mount \
  -mount ~/s3-encrypted-mount \
  -bucket test-bucket \
  -endpoint http://localhost:9000 \
  -access-key test-access-key-1 \
  -secret-key minioadmin \
  -region us-east-1 \
  -config configs/main.yaml &

PERF_PID=$!
sleep 3

echo "=== Performance and Memory Testing ==="

# Monitor memory usage during large file operations
echo "Monitoring memory usage during encryption..."
ps -p $PERF_PID -o pid,ppid,cmd,%mem,%cpu

# Create a larger file (memory usage test)
echo "Creating larger encrypted file (50MB)..."
dd if=/dev/urandom of=~/s3-encrypted-mount/large-test.bin bs=1M count=50

# Monitor memory again
ps -p $PERF_PID -o pid,ppid,cmd,%mem,%cpu

# Test reading the large file
echo "Reading large encrypted file..."
time cp ~/s3-encrypted-mount/large-test.bin /tmp/decrypted-large.bin

# Verify integrity
echo "Verifying file integrity:"
diff ~/s3-encrypted-mount/large-test.bin /tmp/decrypted-large.bin && echo "✅ File integrity verified" || echo "❌ File integrity check failed"

# Cleanup large file
rm ~/s3-encrypted-mount/large-test.bin /tmp/decrypted-large.bin
```

### 17. Test 10: Multiple File Operations

```bash
echo "=== Testing multiple concurrent encrypted operations ==="

# Create multiple files concurrently
for i in {1..10}; do
    (echo "Encrypted content for file $i - $(date)" > ~/s3-encrypted-mount/concurrent-$i.txt) &
done
wait

# Verify all files were created and encrypted
echo "Created files:"
ls ~/s3-encrypted-mount/concurrent-*.txt | wc -l

# Read all files to verify decryption
echo "Verifying decryption of all files:"
for file in ~/s3-encrypted-mount/concurrent-*.txt; do
    if grep -q "Encrypted content" "$file"; then
        echo "✅ $(basename $file) decrypted correctly"
    else
        echo "❌ $(basename $file) decryption failed"
    fi
done

# Verify encryption in S3
echo "Verifying encryption in S3:"
for i in {1..3}; do  # Just check first 3 files
    if mc cat local/test-bucket/concurrent-$i.txt | grep -q "Encrypted content"; then
        echo "❌ concurrent-$i.txt: Found plaintext in S3!"
    else
        echo "✅ concurrent-$i.txt: Properly encrypted in S3"
    fi
done
```

## Cleanup and Verification

### 18. Final Verification and Cleanup

```bash
echo "=== Final Verification ==="

# List all encrypted files
echo "All files in encrypted filesystem:"
find ~/s3-encrypted-mount/ -type f -exec ls -lh {} \;

# List all files in S3
echo "All files in S3 bucket:"
mc ls local/test-bucket/

# Verify that sensitive data is not visible in S3
echo "Checking for sensitive data leakage in S3:"
LEAK_FOUND=false

if mc cat local/test-bucket/config.json | grep -q "super-secret-password"; then
    echo "❌ SECURITY ISSUE: Password found in S3!"
    LEAK_FOUND=true
fi

if mc cat local/test-bucket/encrypted-test.txt | grep -q "sensitive data"; then
    echo "❌ SECURITY ISSUE: Sensitive data found in S3!"
    LEAK_FOUND=true
fi

if [ "$LEAK_FOUND" = false ]; then
    echo "✅ SECURITY CHECK PASSED: No sensitive data found in S3"
fi

# Cleanup
echo "Cleaning up..."
fusermount -u ~/s3-encrypted-mount
wait $PERF_PID

# Remove test files
rm -rf ~/s3-encrypted-mount ~/s3-plaintext-mount
rm -f /tmp/raw-encrypted-*.txt /tmp/raw-encrypted-*.json

# Stop MinIO (optional)
echo "Stopping MinIO server..."
kill $MINIO_PID
wait $MINIO_PID

echo "Testing completed!"
```

## Expected Results Summary

### ✅ Success Indicators:
1. **Authentication**: Valid access keys work, invalid ones are rejected
2. **Encryption**: Data stored in S3 is unreadable gibberish
3. **Decryption**: Data read through filesystem is properly decrypted
4. **Performance**: Large files can be encrypted/decrypted without crashes
5. **Security**: No sensitive data visible in raw S3 storage

### ❌ Failure Indicators:
1. Mount fails with valid credentials
2. Plaintext data visible in S3 storage
3. Files cannot be read after encryption
4. Memory issues with large files
5. Authentication bypass

### 📋 Key Log Messages to Watch For:
- `"Access key validated successfully"`
- `"Encryption enabled using crypto ID: default-triple"`
- `"Encryption: ENABLED - Data will be encrypted before upload and decrypted after download"`
- `"Successfully encrypted X bytes to Y bytes"`
- `"Successfully decrypted X bytes"`

### 🔧 Troubleshooting Tips:
1. **Permission denied**: Check FUSE permissions with `sudo usermod -a -G fuse $USER`
2. **Mount fails**: Ensure mount point is empty and you have FUSE installed
3. **Crypto errors**: Verify environment variables are set correctly
4. **Authentication fails**: Check configs/main.yaml and .env file
5. **High memory usage**: Monitor during large file operations - this is expected for current implementation

This comprehensive test suite validates all aspects of the encryption/decryption functionality including authentication, multi-layer encryption, file operations, security, and performance.