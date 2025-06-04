# S3FS Testing Guide for Ubuntu/Linux

This guide provides comprehensive testing instructions for the native FUSE-based S3 filesystem implementation.

## Prerequisites

### 1. Install Required Packages

```bash
# Update package list
sudo apt update

# Install FUSE development libraries
sudo apt install -y fuse3 libfuse3-dev

# Install Go (if not already installed)
sudo apt install -y golang-go

# Install MinIO client for testing (optional)
wget https://dl.min.io/client/mc/release/linux-amd64/mc
chmod +x mc
sudo mv mc /usr/local/bin/

# Install AWS CLI for testing (optional)
sudo apt install -y awscli
```

### 2. Set up MinIO Server (for local testing)

```bash
# Download and install MinIO server
wget https://dl.min.io/server/minio/release/linux-amd64/minio
chmod +x minio
sudo mv minio /usr/local/bin/

# Create data directory
mkdir -p ~/minio-data

# Start MinIO server (run in separate terminal)
minio server ~/minio-data --console-address ":9001"
# Default credentials: minioadmin / minioadmin
# Web UI: http://localhost:9001
# API: http://localhost:9000
```

### 3. Create Test Bucket and Data

```bash
# Configure MinIO client
mc alias set local http://localhost:9000 minioadmin minioadmin

# Create test bucket
mc mb local/test-bucket

# Create test directory structure
mkdir -p ~/test-data/documents/reports
mkdir -p ~/test-data/images
mkdir -p ~/test-data/config

# Create test files
echo "Hello World from S3FS!" > ~/test-data/hello.txt
echo "This is a document in a subfolder" > ~/test-data/documents/readme.txt
echo "This is a report" > ~/test-data/documents/reports/report1.txt
echo '{"server": "localhost", "port": 9000}' > ~/test-data/config/settings.json

# Create a larger file for performance testing
dd if=/dev/urandom of=~/test-data/large-file.bin bs=1M count=10

# Upload test data to MinIO
mc cp --recursive ~/test-data/ local/test-bucket/
```

## Building and Testing the S3FS Implementation

### 1. Build the Application

```bash
# Navigate to project directory
cd /path/to/S3-Proxy

# Build the application
go build -o s3-proxy ./cmd/s3-proxy

# Verify build
./s3-proxy --help || echo "Build completed"
```

### 2. Basic Functionality Tests

#### Test 1: Mount S3 Bucket

```bash
# Create mount point
mkdir -p ~/s3-mount

# Mount the S3 bucket (basic test)
./s3-proxy s3fs-mount \
  -mount ~/s3-mount \
  -bucket test-bucket \
  -endpoint http://localhost:9000 \
  -access-key minioadmin \
  -secret-key minioadmin \
  -region us-east-1

# This should run in foreground - open another terminal for testing
```

#### Test 2: Verify Mount (in another terminal)

```bash
# Check if mount is active
mount | grep s3fs
df -h | grep s3fs

# List root directory
ls -la ~/s3-mount/

# Expected output should show the uploaded files:
# hello.txt, large-file.bin, documents/, images/, config/
```

#### Test 3: Basic File Operations

```bash
# Read a simple file
cat ~/s3-mount/hello.txt
# Expected: "Hello World from S3FS!"

# Read configuration file
cat ~/s3-mount/config/settings.json
# Expected: {"server": "localhost", "port": 9000}

# Check file sizes
ls -lh ~/s3-mount/
ls -lh ~/s3-mount/documents/

# Navigate directories
cd ~/s3-mount/documents/
ls -la
cat readme.txt
cd reports/
cat report1.txt
```

#### Test 4: Performance Tests

```bash
# Test reading large file
time cp ~/s3-mount/large-file.bin /tmp/copied-large-file.bin

# Verify file integrity
md5sum ~/test-data/large-file.bin
md5sum /tmp/copied-large-file.bin
# MD5 sums should match

# Test partial reads
head -c 1024 ~/s3-mount/large-file.bin > /tmp/partial-read.bin
ls -l /tmp/partial-read.bin
# Should be exactly 1024 bytes

# Test directory listing performance
time ls -la ~/s3-mount/
time find ~/s3-mount/ -type f | wc -l
```

### 3. Error Handling Tests

#### Test 5: Invalid Credentials

```bash
# Unmount current filesystem
fusermount -u ~/s3-mount

# Try mounting with invalid credentials
./s3-proxy s3fs-mount \
  -mount ~/s3-mount \
  -bucket test-bucket \
  -endpoint http://localhost:9000 \
  -access-key invalid-key \
  -secret-key invalid-secret \
  -region us-east-1

# Expected: Should fail with authentication error
```

#### Test 6: Non-existent Bucket

```bash
# Try mounting non-existent bucket
./s3-proxy s3fs-mount \
  -mount ~/s3-mount \
  -bucket non-existent-bucket \
  -endpoint http://localhost:9000 \
  -access-key minioadmin \
  -secret-key minioadmin \
  -region us-east-1

# Expected: Should fail with bucket not found error
```

#### Test 7: Invalid Mount Point

```bash
# Try mounting to a file instead of directory
touch ~/not-a-directory

./s3-proxy s3fs-mount \
  -mount ~/not-a-directory \
  -bucket test-bucket \
  -endpoint http://localhost:9000 \
  -access-key minioadmin \
  -secret-key minioadmin \
  -region us-east-1

# Expected: Should fail with "not a directory" error

# Clean up
rm ~/not-a-directory
```

### 4. Advanced Tests

#### Test 8: Deep Directory Structure

```bash
# First, create deep directory structure in S3
mc cp ~/test-data/hello.txt local/test-bucket/level1/level2/level3/deep-file.txt

# Mount filesystem
./s3-proxy s3fs-mount \
  -mount ~/s3-mount \
  -bucket test-bucket \
  -endpoint http://localhost:9000 \
  -access-key minioadmin \
  -secret-key minioadmin \
  -region us-east-1 &

# Wait for mount
sleep 2

# Test deep navigation
ls ~/s3-mount/level1/
ls ~/s3-mount/level1/level2/
ls ~/s3-mount/level1/level2/level3/
cat ~/s3-mount/level1/level2/level3/deep-file.txt
```

#### Test 9: Large Directory Listing

```bash
# Create many files for pagination testing
for i in {1..150}; do
  echo "File number $i" | mc pipe local/test-bucket/many-files/file-$(printf "%03d" $i).txt
done

# Test listing large directory
time ls ~/s3-mount/many-files/ | wc -l
# Should show 150 files

# Test pagination by checking all files are accessible
for i in {1..150}; do
  file="file-$(printf "%03d" $i).txt"
  if [ ! -f "~/s3-mount/many-files/$file" ]; then
    echo "Missing file: $file"
  fi
done
```

#### Test 10: Debug Mode Testing

```bash
# Unmount current filesystem
fusermount -u ~/s3-mount

# Mount with debug mode
./s3-proxy s3fs-mount \
  -mount ~/s3-mount \
  -bucket test-bucket \
  -endpoint http://localhost:9000 \
  -access-key minioadmin \
  -secret-key minioadmin \
  -region us-east-1 \
  -debug &

# Perform operations and observe debug output
ls ~/s3-mount/
cat ~/s3-mount/hello.txt
```

### 5. Stress Tests

#### Test 11: Concurrent Access

```bash
# Test concurrent file reads
for i in {1..10}; do
  (cat ~/s3-mount/hello.txt > /dev/null) &
done
wait

# Test concurrent directory listings
for i in {1..5}; do
  (ls -la ~/s3-mount/ > /dev/null) &
done
wait
```

#### Test 12: Memory Usage Monitoring

```bash
# Monitor memory usage during operations
pid=$(pgrep -f "s3fs-mount")
echo "S3FS Process PID: $pid"

# Monitor memory while performing operations
watch -n 1 "ps -p $pid -o pid,ppid,cmd,%mem,%cpu"

# In another terminal, perform file operations
time cp ~/s3-mount/large-file.bin /tmp/stress-test.bin
```

### 6. Integration Tests with Other Tools

#### Test 13: Using with Standard Unix Tools

```bash
# Test with grep
grep -r "Hello" ~/s3-mount/

# Test with find
find ~/s3-mount/ -name "*.txt" -exec wc -l {} \;

# Test with file
file ~/s3-mount/large-file.bin
file ~/s3-mount/config/settings.json

# Test with diff
diff ~/test-data/hello.txt ~/s3-mount/hello.txt
# Should show no differences
```

#### Test 14: Backup/Sync Operations

```bash
# Test using rsync
mkdir -p ~/backup
rsync -av ~/s3-mount/ ~/backup/
diff -r ~/s3-mount/ ~/backup/

# Test using tar
tar -czf ~/s3-backup.tar.gz -C ~/s3-mount .
```

## Cleanup and Unmounting

### Proper Unmounting

```bash
# Unmount the filesystem
fusermount -u ~/s3-mount

# Verify unmount
mount | grep s3fs
# Should show no s3fs mounts

# Clean up test files
rm -rf ~/s3-mount ~/backup ~/test-data
rm -f /tmp/copied-large-file.bin /tmp/partial-read.bin /tmp/stress-test.bin ~/s3-backup.tar.gz
```

### Stop MinIO Server

```bash
# Stop MinIO server (Ctrl+C in the terminal where it's running)
# Or if running as background process:
pkill minio
```

## Expected Results and Benchmarks

### Performance Expectations

- **Small file reads** (< 1KB): Should complete in < 100ms
- **Large file reads** (10MB): Should achieve > 50MB/s throughput
- **Directory listings** (< 100 files): Should complete in < 500ms
- **Deep directory navigation**: Should work without issues up to 10+ levels

### Error Scenarios That Should Work

1. **Authentication failures**: Clear error messages about access denied
2. **Network timeouts**: Graceful handling with appropriate FUSE errors
3. **Non-existent files**: Proper ENOENT errors
4. **Invalid paths**: Appropriate error responses

### Known Limitations

1. **Read-only filesystem**: Write operations not supported
2. **No caching**: Each file access goes to S3 (intentional for simplicity)
3. **Sequential access optimized**: Random access may be slower
4. **Large directories**: May be slow due to S3 API limitations

## Troubleshooting

### Common Issues

1. **Permission denied**: Ensure user has access to FUSE
   ```bash
   sudo usermod -a -G fuse $USER
   # Log out and back in
   ```

2. **Mount point busy**: Ensure proper unmounting
   ```bash
   fusermount -u ~/s3-mount
   # If that fails:
   sudo umount -l ~/s3-mount
   ```

3. **FUSE not available**: Install FUSE development packages
   ```bash
   sudo apt install fuse3 libfuse3-dev
   ```

### Debug Information

```bash
# Check FUSE version
fusermount --version

# Check kernel modules
lsmod | grep fuse

# Check system logs
dmesg | grep -i fuse
journalctl -u fuse
```

This comprehensive test suite should validate all aspects of the S3FS implementation including basic functionality, error handling, performance, and edge cases. 