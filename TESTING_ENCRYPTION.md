# S3FS Encryption/Decryption Testing Guide for Ubuntu Linux

This guide provides step-by-step instructions to test the newly implemented encryption/decryption functionality in the S3FS implementation.

**IMPORTANT**: This system uses the same authentication mechanism as the main S3-Proxy server, which expects AWS-style access keys and authorization headers.

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
sudo apt install -y jq curl openssl
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
# Generate encryption keys using the built-in command
echo "Generating encryption keysets..."
./s3-proxy cryption-keyset

# The above command will output 3 keysets. Copy them and set them as environment variables:
# Example output:
# TINK_KEYSET="CiQA..."
# AES_KEY="random-base64-string..."  
# CHACHA_KEY="another-random-base64-string..."

# Set the encryption keys (replace with actual output from above command)
export TINK_KEYSET="PASTE_TINK_KEYSET_HERE"
export AES_KEY="PASTE_AES_KEY_HERE"
export CHACHA_KEY="PASTE_CHACHA_KEY_HERE"

# S3 credentials for MinIO backend
export S3_ACCESS_KEY="minioadmin"
export S3_SECRET_KEY="minioadmin"

# Auth system configuration (these are the REAL access keys from the system)
export ALLOWED_ACCESS_KEY1="AKIAEXAMPLEACCESSKEY1"
export ALLOWED_ACCESS_KEY2="AKIAEXAMPLEACCESSKEY2"
export AUTH_HEADER_FORMAT="AWS4-HMAC-SHA256"

# Print keys for verification
echo "Environment variables set:"
echo "TINK_KEYSET=$TINK_KEYSET"
echo "AES_KEY=$AES_KEY"
echo "CHACHA_KEY=$CHACHA_KEY"
echo "ALLOWED_ACCESS_KEY1=$ALLOWED_ACCESS_KEY1"
echo "ALLOWED_ACCESS_KEY2=$ALLOWED_ACCESS_KEY2"
```

### 4. Create .env File

```bash
# Create .env file with all environment variables
cat > .env << EOF
# Encryption keys (replace with actual keys from cryption-keyset command)
TINK_KEYSET=$TINK_KEYSET
AES_KEY=$AES_KEY
CHACHA_KEY=$CHACHA_KEY

# S3 backend credentials
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin

# Additional backends (optional - for full multi-backend testing)
DO_SPACES_ACCESS_KEY=your-do-access-key
DO_SPACES_SECRET_KEY=your-do-secret-key
STORJ_ACCESS_KEY=your-storj-access-key
STORJ_SECRET_KEY=your-storj-secret-key

# Authentication - these are the proxy-level access keys
ALLOWED_ACCESS_KEY1=AKIAEXAMPLEACCESSKEY1
ALLOWED_ACCESS_KEY2=AKIAEXAMPLEACCESSKEY2
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

# Verify bucket configurations - note the multiple backends
echo "Checking bucket configurations:"
grep -A 15 "s3_buckets:" configs/main.yaml

# Verify auth configurations
echo "Checking auth configurations:"
grep -A 10 "auth:" configs/main.yaml

# The configuration shows test-bucket has 3 backends:
# 1. local (MinIO) - uses default-triple encryption
# 2. storj - uses default-triple encryption  
# 3. digitalocean - uses default encryption
echo "Note: test-bucket is configured with multiple backends for redundancy"
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

# Create test bucket (this maps to the 'local' backend in configs/main.yaml)
mc mb local/test-bucket

# Verify bucket creation
mc ls local/

echo "Created bucket 'test-bucket' which corresponds to the local backend in main.yaml"
```

## Testing Encryption/Decryption Functionality

### 8. Test 1: Mount with Encryption (Valid Access Key)

```bash
# Create mount point
mkdir -p ~/s3-encrypted-mount

# Mount with encryption enabled using the correct access key format
# Note: The new S3FS automatically uses the bucket configuration from main.yaml
./s3-proxy s3fs-mount \
  -mount ~/s3-encrypted-mount \
  -bucket test-bucket \
  -access-key AKIAEXAMPLEACCESSKEY1 \
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
echo "- 'Access key validated successfully: AKIAEXAMPLEACCESSKEY1'"
echo "- 'Using first backend for S3FS: s3_client_id=local, s3_bucket_name=test-bucket, crypto_id=default-triple'"
echo "- 'Encryption enabled using crypto ID: default-triple'"
echo "- 'Successfully validated access to backend bucket: test-bucket (logical bucket: test-bucket)'"
echo "- 'Backend Information: Physical bucket: test-bucket, S3 endpoint: http://localhost:9000, Encryption: default-triple'"
echo ""
echo "Important: The S3FS uses the first backend (local MinIO) with default-triple encryption"
```

### 9. Test 2: Compare with Main Proxy System

```bash
# First, test the main S3-Proxy system to understand the architecture
echo "=== Testing main S3-Proxy system for comparison ==="

# Start the main proxy server (in background)
./s3-proxy s3-proxy --config=configs/main.yaml &
PROXY_PID=$!
sleep 3

echo "S3-Proxy server started with PID: $PROXY_PID"

# Test with main proxy - this will replicate to ALL backends
echo "Testing PUT via main proxy (replicates to all backends):"
curl -X PUT \
  -H "Authorization: AWS4-HMAC-SHA256 Credential=AKIAEXAMPLEACCESSKEY1/20250516/us-east-1/s3/aws4_request" \
  -H "Content-Type: text/plain" \
  -d "This is test data from main proxy system" \
  http://localhost:8080/test-bucket/proxy-test.txt

echo ""
echo "Testing GET via main proxy (reads from available backends):"
curl -H "Authorization: AWS4-HMAC-SHA256 Credential=AKIAEXAMPLEACCESSKEY1/20250516/us-east-1/s3/aws4_request" \
  http://localhost:8080/test-bucket/proxy-test.txt

echo ""

# Stop proxy for now
kill $PROXY_PID
wait $PROXY_PID
```

### 10. Test 3: Write Files via S3FS (Single Backend)

```bash
# Test creating encrypted files via S3FS
echo "=== Testing S3FS file operations (single backend) ==="

echo "This is a test file from S3FS with encryption!" > ~/s3-encrypted-mount/s3fs-test.txt
echo "Another encrypted file with sensitive data: password123" > ~/s3-encrypted-mount/secret-data.txt

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
    "algorithm": "default-triple (ChaCha20-Poly1305 + AES-GCM + Tink)"
  }
}
EOF

# Verify files were created
ls -la ~/s3-encrypted-mount/
echo "Files created in S3FS encrypted filesystem"
```

### 11. Test 4: Verify Encryption in Backend Storage

```bash
# Check what's actually stored in S3 backend (should be encrypted)
echo "=== Checking raw data in MinIO backend (should be encrypted) ==="

# Download and examine the raw encrypted data from MinIO
mc cat local/test-bucket/s3fs-test.txt > /tmp/raw-s3fs-test.txt
mc cat local/test-bucket/config.json > /tmp/raw-config.json

echo "Raw encrypted content of s3fs-test.txt:"
xxd /tmp/raw-s3fs-test.txt | head -5
echo ""

echo "Raw encrypted content of config.json:"
xxd /tmp/raw-config.json | head -5
echo ""

# Try to read as text (should be gibberish)
echo "Attempting to read encrypted data as text (should be unreadable):"
head -c 100 /tmp/raw-s3fs-test.txt
echo ""
echo ""

# Verify it doesn't contain original text
if grep -q "test file from S3FS" /tmp/raw-s3fs-test.txt; then
    echo "❌ ERROR: Original text found in MinIO! Encryption may not be working!"
else
    echo "✅ SUCCESS: Original text not found in MinIO - data is encrypted!"
fi

if grep -q "super-secret-password" /tmp/raw-config.json; then
    echo "❌ ERROR: Secret data found in MinIO! Encryption may not be working!"
else
    echo "✅ SUCCESS: Secret data not found in MinIO - data is encrypted!"
fi
```

### 12. Test 5: Read and Verify Decryption

```bash
# Read files through S3FS (should be decrypted)
echo "=== Reading files through S3FS (should be decrypted) ==="

echo "Content of s3fs-test.txt (should be readable):"
cat ~/s3-encrypted-mount/s3fs-test.txt
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

### 13. Test 6: Authentication Failure Test

```bash
# Unmount current filesystem
fusermount -u ~/s3-encrypted-mount
wait $MOUNT_PID

# Try to mount with invalid access key (should fail)
echo "=== Testing authentication failure ==="
./s3-proxy s3fs-mount \
  -mount ~/s3-encrypted-mount \
  -bucket test-bucket \
  -access-key INVALIDACCESSKEY \
  -config configs/main.yaml

# Expected output: "invalid access key: INVALIDACCESSKEY"
echo "Expected: Authentication should fail with 'invalid access key' error"
```

### 14. Test 7: Mount Without Encryption (No Config)

```bash
# Test mounting without configuration for comparison
mkdir -p ~/s3-plaintext-mount

# Mount without config file (should disable encryption)
# Since config is required now, we'll test with no access key instead
./s3-proxy s3fs-mount \
  -mount ~/s3-plaintext-mount \
  -bucket test-bucket \
  -config configs/main.yaml \
  -debug &

PLAINTEXT_PID=$!
sleep 3

echo "Expected log messages:"
echo "- 'Warning: No access key provided. Proceeding without authentication validation.'"
echo "- 'Using first backend for S3FS: s3_client_id=local'"
echo "- 'Encryption enabled using crypto ID: default-triple' (encryption still works, just no auth)"

# Create a file (will still be encrypted because backend has crypto_id)
echo "This data will still be encrypted despite no access key" > ~/s3-plaintext-mount/no-auth-test.txt

# Verify it's stored as encrypted in MinIO (because backend config has crypto_id)
sleep 2
echo "Raw data in MinIO (should still be encrypted due to backend config):"
xxd <(mc cat local/test-bucket/no-auth-test.txt) | head -3

# Cleanup
fusermount -u ~/s3-plaintext-mount
wait $PLAINTEXT_PID
```

### 15. Test 8: Cross-System Compatibility

```bash
# Mount S3FS again
./s3-proxy s3fs-mount \
  -mount ~/s3-encrypted-mount \
  -bucket test-bucket \
  -access-key AKIAEXAMPLEACCESSKEY1 \
  -config configs/main.yaml &

ENCRYPTED_PID=$!
sleep 3

echo "=== Testing cross-system compatibility ==="

# Files created by S3FS should be readable by S3FS
echo "Reading S3FS-created files via S3FS:"
cat ~/s3-encrypted-mount/s3fs-test.txt
echo ""

# Files created by main proxy should NOT be readable by S3FS (if any exist)
# because S3FS only uses the first backend, while proxy replicates to all

# Start main proxy again
./s3-proxy s3-proxy --config=configs/main.yaml &
PROXY_PID=$!
sleep 3

echo "Reading S3FS-created files via main proxy:"
curl -s -H "Authorization: AWS4-HMAC-SHA256 Credential=AKIAEXAMPLEACCESSKEY1/20250516/us-east-1/s3/aws4_request" \
  http://localhost:8080/test-bucket/s3fs-test.txt
echo ""

# Stop services
kill $PROXY_PID
wait $PROXY_PID
fusermount -u ~/s3-encrypted-mount
wait $ENCRYPTED_PID
```

## Architecture Understanding

### Important Notes:

1. **Single vs Multi-Backend**:
   - **S3FS**: Uses only the FIRST backend (`local` MinIO) from the bucket configuration
   - **Main Proxy**: Replicates data to ALL backends (local + storj + digitalocean)

2. **Access Keys**:
   - **Proxy-level**: `AKIAEXAMPLEACCESSKEY1`, `AKIAEXAMPLEACCESSKEY2` (for authentication)
   - **Backend-level**: `minioadmin` (for MinIO), DO/Storj keys (for other backends)

3. **Encryption**:
   - **S3FS**: Uses the crypto_id from the first backend (`default-triple`)
   - **Main Proxy**: Uses crypto_id per backend (may differ between backends)

4. **Use Cases**:
   - **S3FS**: Fast single-backend access with encryption, good for file operations
   - **Main Proxy**: Multi-backend redundancy, better for API access and reliability

## Performance Testing

### 16. Test 9: Performance with Large Files

```bash
# Mount S3FS for performance testing
./s3-proxy s3fs-mount \
  -mount ~/s3-encrypted-mount \
  -bucket test-bucket \
  -access-key AKIAEXAMPLEACCESSKEY1 \
  -config configs/main.yaml &

PERF_PID=$!
sleep 3

echo "=== Performance Testing with Large Files ==="

# Create a 100MB file to test encryption performance
echo "Creating 100MB test file..."
time dd if=/dev/urandom of=~/s3-encrypted-mount/large-100mb.bin bs=1M count=100

# Test reading performance
echo "Testing read performance..."
time cp ~/s3-encrypted-mount/large-100mb.bin /tmp/decrypted-100mb.bin

# Verify file integrity
echo "Verifying file integrity..."
diff ~/s3-encrypted-mount/large-100mb.bin /tmp/decrypted-100mb.bin && echo "✅ File integrity verified" || echo "❌ File integrity check failed"

# Monitor memory usage
echo "Process memory usage:"
ps -p $PERF_PID -o pid,ppid,cmd,%mem,%cpu

# Cleanup
rm ~/s3-encrypted-mount/large-100mb.bin /tmp/decrypted-100mb.bin
fusermount -u ~/s3-encrypted-mount
wait $PERF_PID
```

## Cleanup and Final Verification

### 17. Final Security Verification

```bash
echo "=== Final Security Verification ==="

# List all files in MinIO
echo "All files in MinIO backend:"
mc ls local/test-bucket/

# Verify no sensitive data is visible in raw storage
echo "Checking for data leakage in MinIO backend:"
LEAK_FOUND=false

# Check a few files for plaintext leakage
for file in s3fs-test.txt config.json secret-data.txt; do
    if mc cat local/test-bucket/$file 2>/dev/null | grep -q -i "password\|secret\|sensitive"; then
        echo "❌ SECURITY ISSUE: Sensitive data found in $file!"
        LEAK_FOUND=true
    else
        echo "✅ $file: No sensitive data found in raw storage"
    fi
done

if [ "$LEAK_FOUND" = false ]; then
    echo "✅ SECURITY CHECK PASSED: No sensitive data found in backend storage"
fi

# Cleanup all test files
echo "Cleaning up test files..."
mc rm --recursive local/test-bucket/
rm -rf ~/s3-encrypted-mount ~/s3-plaintext-mount
rm -f /tmp/raw-*.txt /tmp/raw-*.json

# Stop MinIO
echo "Stopping MinIO server..."
kill $MINIO_PID
wait $MINIO_PID

echo "Testing completed!"
```

## Expected Results Summary

### ✅ Success Indicators:
1. **Authentication**: Valid access keys (`AKIAEXAMPLEACCESSKEY1`) work, invalid ones rejected
2. **Encryption**: Data in MinIO backend is unreadable gibberish
3. **Decryption**: Data read through S3FS is properly decrypted
4. **Single Backend**: S3FS operates on first backend only (local MinIO)
5. **Performance**: Large files encrypt/decrypt without crashes

### ❌ Failure Indicators:
1. Mount fails with valid `AKIAEXAMPLEACCESSKEY1`
2. Plaintext data visible in MinIO storage
3. Files cannot be read after encryption
4. Memory issues with large files

### 📋 Key Log Messages to Watch For:
- `"Access key validated successfully"`
- `"Encryption enabled using crypto ID: default-triple"`
- `"Encryption: ENABLED - Data will be encrypted before upload and decrypted after download"`
- `"Successfully encrypted X bytes to Y bytes"`
- `"Successfully decrypted X bytes"`

### 🔧 Troubleshooting Tips:
1. **Authentication fails**: Ensure you're using `AKIAEXAMPLEACCESSKEY1` not `test-access-key-1`
2. **Mount fails**: Check FUSE permissions: `sudo usermod -a -G fuse $USER`
3. **Crypto errors**: Run `./s3-proxy cryption-keyset` to generate valid keys
4. **Config errors**: Verify `configs/main.yaml` and `.env` file match expected format
5. **Backend confusion**: Remember S3FS uses only the first backend, not all backends

This guide tests the S3FS encryption functionality while understanding its role in the larger S3-Proxy architecture.