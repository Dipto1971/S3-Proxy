# S3FS Testing Guide for Ubuntu/Linux

This guide provides comprehensive testing instructions for the native FUSE-based S3 filesystem implementation with full read/write support.

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

#### Test 1: Mount S3 Bucket (Read/Write Mode)

```bash
# Create mount point
mkdir -p ~/s3-mount

# Mount the S3 bucket with write support (default mode)
./s3-proxy s3fs-mount \
  -mount ~/s3-mount \
  -bucket test-bucket \
  -endpoint http://localhost:9000 \
  -access-key minioadmin \
  -secret-key minioadmin \
  -region us-east-1

# This should run in foreground - open another terminal for testing
# Look for log message: "Mounting with read/write support"
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

#### Test 3: Basic Read Operations

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

## Write Operation Tests

### Test 4: File Creation and Writing

```bash
# Test creating a new file
echo "Testing write operations" > ~/s3-mount/test-write.txt

# Verify the file was created
ls -la ~/s3-mount/test-write.txt
cat ~/s3-mount/test-write.txt
# Expected: "Testing write operations"

# Test appending to file (creates new content)
echo "Second line of content" > ~/s3-mount/test-write.txt
cat ~/s3-mount/test-write.txt
# Expected: "Second line of content" (overwrites previous content)

# Test creating files with different content types
echo '{"test": "json", "write": true}' > ~/s3-mount/test.json
echo "#!/bin/bash" > ~/s3-mount/test.sh
echo "Line 1" > ~/s3-mount/multiline.txt

# Verify all files exist
ls -la ~/s3-mount/test*
```

### Test 5: Directory Creation

```bash
# Create a new directory
mkdir ~/s3-mount/new-directory
ls -la ~/s3-mount/ | grep new-directory
# Should show directory with 'drwxr-xr-x' permissions

# Create nested directories
mkdir -p ~/s3-mount/deep/nested/structure
ls -la ~/s3-mount/deep/nested/
# Should show 'structure' directory

# Create files in new directories
echo "File in new directory" > ~/s3-mount/new-directory/file1.txt
echo "File in nested directory" > ~/s3-mount/deep/nested/structure/nested-file.txt

# Verify directory structure
find ~/s3-mount/new-directory/ -type f
find ~/s3-mount/deep/ -type f
```

### Test 6: File Modification

```bash
# Create a file with initial content
echo "Initial content" > ~/s3-mount/modify-test.txt

# Modify the file multiple times
echo "Modified content line 1" > ~/s3-mount/modify-test.txt
echo "Modified content line 2" >> ~/s3-mount/modify-test.txt  # Note: >> may not work as expected due to S3 limitations
echo "Final content" > ~/s3-mount/modify-test.txt

cat ~/s3-mount/modify-test.txt
# Expected: "Final content"

# Test with larger content
dd if=/dev/urandom of=~/s3-mount/random-data.bin bs=1024 count=10
ls -lh ~/s3-mount/random-data.bin
# Should show ~10KB file
```

### Test 7: File and Directory Deletion

```bash
# Create test files and directories for deletion
echo "To be deleted" > ~/s3-mount/delete-me.txt
mkdir ~/s3-mount/delete-me-dir
echo "File in directory" > ~/s3-mount/delete-me-dir/file.txt

# Test file deletion
rm ~/s3-mount/delete-me.txt
ls ~/s3-mount/delete-me.txt
# Expected: "No such file or directory"

# Test directory deletion (should fail if not empty)
rmdir ~/s3-mount/delete-me-dir
# Expected: "Directory not empty" error

# Remove file from directory first
rm ~/s3-mount/delete-me-dir/file.txt
ls ~/s3-mount/delete-me-dir/
# Should be empty

# Now remove the empty directory
rmdir ~/s3-mount/delete-me-dir
ls ~/s3-mount/delete-me-dir
# Expected: "No such file or directory"
```

### Test 8: Rename and Move Operations

```bash
# Create test files for renaming
echo "Original file" > ~/s3-mount/original-name.txt
mkdir ~/s3-mount/source-dir
echo "File to move" > ~/s3-mount/source-dir/moveme.txt

# Test file renaming in same directory
mv ~/s3-mount/original-name.txt ~/s3-mount/renamed-file.txt
ls ~/s3-mount/original-name.txt  # Should not exist
cat ~/s3-mount/renamed-file.txt  # Should show "Original file"

# Test moving file between directories
mkdir ~/s3-mount/target-dir
mv ~/s3-mount/source-dir/moveme.txt ~/s3-mount/target-dir/
ls ~/s3-mount/source-dir/moveme.txt  # Should not exist
cat ~/s3-mount/target-dir/moveme.txt  # Should show "File to move"

# Test directory renaming
mv ~/s3-mount/source-dir ~/s3-mount/renamed-source-dir
ls -la ~/s3-mount/ | grep source
ls -la ~/s3-mount/ | grep renamed-source-dir
```

### Test 9: Large File Write Operations

```bash
# Test writing larger files
dd if=/dev/zero of=~/s3-mount/large-write.bin bs=1M count=5
ls -lh ~/s3-mount/large-write.bin
# Should show 5MB file

# Test with random data
dd if=/dev/urandom of=~/s3-mount/random-large.bin bs=512k count=20
ls -lh ~/s3-mount/random-large.bin
# Should show ~10MB file

# Verify file integrity by copying back and comparing
cp ~/s3-mount/random-large.bin /tmp/copied-random.bin
# Files should be identical when read back
```

### Test 10: Concurrent Write Operations

```bash
# Test multiple files being written simultaneously
for i in {1..5}; do
  (echo "Concurrent file $i content" > ~/s3-mount/concurrent-$i.txt) &
done
wait

# Verify all files were created
ls ~/s3-mount/concurrent-*.txt
for i in {1..5}; do
  echo "File $i content:"
  cat ~/s3-mount/concurrent-$i.txt
done
```

### Test 11: Copy Operations (using standard tools)

```bash
# Test copying files from local to S3FS
cp ~/test-data/hello.txt ~/s3-mount/copied-hello.txt
diff ~/test-data/hello.txt ~/s3-mount/copied-hello.txt
# Should show no differences

# Test copying files from S3FS to local
cp ~/s3-mount/copied-hello.txt /tmp/copied-back.txt
diff ~/test-data/hello.txt /tmp/copied-back.txt
# Should show no differences

# Test copying directories (with rsync)
mkdir -p ~/local-test
echo "Local file 1" > ~/local-test/file1.txt
echo "Local file 2" > ~/local-test/file2.txt
rsync -av ~/local-test/ ~/s3-mount/synced-dir/

# Verify sync
ls -la ~/s3-mount/synced-dir/
cat ~/s3-mount/synced-dir/file1.txt
cat ~/s3-mount/synced-dir/file2.txt
```

### Test 12: Text Editor Operations

```bash
# Test using nano/vim with files (if available)
echo "Edit me" > ~/s3-mount/edit-test.txt

# Using nano (if available)
nano ~/s3-mount/edit-test.txt
# Add some content, save, and exit

# Verify changes
cat ~/s3-mount/edit-test.txt

# Test with echo redirection
echo "Line 1" > ~/s3-mount/multiline-edit.txt
echo "Line 2" > ~/s3-mount/temp-line2.txt
cat ~/s3-mount/temp-line2.txt >> ~/s3-mount/multiline-edit.txt  # Note: This might not work as expected
```

### Test 13: File Permissions and Attributes

```bash
# Test file permissions (basic support)
ls -la ~/s3-mount/test-write.txt
# Should show -rw-r--r-- (644 permissions)

# Test chmod (might not persist due to S3 limitations)
chmod 755 ~/s3-mount/test.sh
ls -la ~/s3-mount/test.sh

# Test file timestamps
stat ~/s3-mount/test-write.txt
# Should show creation/modification times
```

## Write Operation Performance Tests

### Test 14: Write Performance Benchmarks

```bash
# Test small file write performance
time for i in {1..100}; do
  echo "Small file $i" > ~/s3-mount/perf-small-$i.txt
done

# Count created files
ls ~/s3-mount/perf-small-*.txt | wc -l
# Should show 100 files

# Test larger file write performance
time dd if=/dev/zero of=~/s3-mount/perf-large.bin bs=1M count=50
ls -lh ~/s3-mount/perf-large.bin

# Clean up performance test files
rm ~/s3-mount/perf-small-*.txt
rm ~/s3-mount/perf-large.bin
```

### Test 15: Memory Usage During Writes

```bash
# Monitor memory usage during large file operations
pid=$(pgrep -f "s3fs-mount")
echo "S3FS Process PID: $pid"

# Start monitoring in background
(while kill -0 $pid 2>/dev/null; do
  ps -p $pid -o pid,ppid,cmd,%mem,%cpu
  sleep 1
done) &
monitor_pid=$!

# Perform large write operation
dd if=/dev/urandom of=~/s3-mount/memory-test.bin bs=1M count=100

# Stop monitoring
kill $monitor_pid 2>/dev/null || true

# Verify file
ls -lh ~/s3-mount/memory-test.bin
rm ~/s3-mount/memory-test.bin
```

## Write Operation Error Handling Tests

### Test 16: Disk Space Simulation

```bash
# Test extremely large file (may fail due to memory limits)
dd if=/dev/zero of=~/s3-mount/huge-file.bin bs=1G count=1
# This should either succeed or fail gracefully

# Clean up if created
rm -f ~/s3-mount/huge-file.bin
```

### Test 17: Invalid Operations

```bash
# Test writing to non-existent directory
echo "test" > ~/s3-mount/nonexistent/file.txt
# Expected: "No such file or directory" error

# Test removing non-empty directory
mkdir ~/s3-mount/test-non-empty
echo "content" > ~/s3-mount/test-non-empty/file.txt
rmdir ~/s3-mount/test-non-empty
# Expected: "Directory not empty" error

# Clean up
rm ~/s3-mount/test-non-empty/file.txt
rmdir ~/s3-mount/test-non-empty
```

### Test 18: Read-Only Mode Testing

```bash
# Unmount current filesystem
fusermount -u ~/s3-mount

# Mount in read-only mode
./s3-proxy s3fs-mount \
  -mount ~/s3-mount \
  -bucket test-bucket \
  -endpoint http://localhost:9000 \
  -access-key minioadmin \
  -secret-key minioadmin \
  -region us-east-1 \
  -read-only &

# Wait for mount
sleep 2

# Test that write operations fail
echo "Should fail" > ~/s3-mount/readonly-test.txt
# Expected: "Read-only file system" error

mkdir ~/s3-mount/readonly-dir
# Expected: "Read-only file system" error

# Verify read operations still work
cat ~/s3-mount/hello.txt
ls ~/s3-mount/

# Remount in read-write mode for further testing
fusermount -u ~/s3-mount
./s3-proxy s3fs-mount \
  -mount ~/s3-mount \
  -bucket test-bucket \
  -endpoint http://localhost:9000 \
  -access-key minioadmin \
  -secret-key minioadmin \
  -region us-east-1 &
sleep 2
```

## Advanced Write Operation Tests

### Test 19: S3 Backend Verification

```bash
# Create file through S3FS
echo "Created via S3FS" > ~/s3-mount/s3fs-created.txt

# Verify it appears in S3 backend
mc ls local/test-bucket/s3fs-created.txt
mc cat local/test-bucket/s3fs-created.txt
# Should show "Created via S3FS"

# Create file via S3 backend
echo "Created via S3" | mc pipe local/test-bucket/s3-created.txt

# Verify it appears in S3FS (may need to wait for cache invalidation)
sleep 5
ls ~/s3-mount/s3-created.txt
cat ~/s3-mount/s3-created.txt
# Should show "Created via S3"
```

### Test 20: Stress Testing Write Operations

```bash
# Create stress test directory
mkdir ~/s3-mount/stress-test

# Create many small files rapidly
for i in {1..200}; do
  echo "Stress test file $i with some content to make it interesting" > ~/s3-mount/stress-test/stress-$i.txt
done

# Verify all files were created
ls ~/s3-mount/stress-test/ | wc -l
# Should show 200 files

# Test rapid directory creation
for i in {1..50}; do
  mkdir ~/s3-mount/stress-test/dir-$i
  echo "File in dir $i" > ~/s3-mount/stress-test/dir-$i/file.txt
done

# Verify directory structure
find ~/s3-mount/stress-test/ -type d | wc -l
find ~/s3-mount/stress-test/ -type f | wc -l

# Clean up stress test
rm -rf ~/s3-mount/stress-test/
```

## Write Operation Results Validation

### Expected Write Operation Behaviors

1. **File Creation**: New files should appear immediately in directory listings
2. **File Writing**: Content should be buffered and uploaded on flush/close
3. **Directory Creation**: Directories should appear as S3 objects ending with "/"
4. **File Deletion**: Files should be removed from both filesystem and S3
5. **Directory Deletion**: Only empty directories can be removed
6. **Rename Operations**: Implemented as copy + delete in S3
7. **Permissions**: Files show 644, directories show 755 permissions

### Performance Expectations (Write Operations)

- **Small file writes** (< 1KB): Should complete in < 200ms
- **Medium file writes** (1-10MB): Should achieve > 20MB/s throughput  
- **Large file writes** (> 10MB): Limited by available memory for buffering
- **Directory operations**: Should complete in < 100ms
- **Rename operations**: Performance depends on file size (copy + delete)

### Write Operation Limitations

1. **Memory Usage**: Large files are buffered in memory during write
2. **No Partial Updates**: Files must be written completely
3. **No Atomic Operations**: S3 eventual consistency applies
4. **Sequential Writes**: Random access writes are limited
5. **No Hard Links**: Not supported by S3 object storage
6. **No Symbolic Links**: Not implemented in current version

## Error Handling Tests

### Test 4: Performance Tests

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
- **Small file writes** (< 1KB): Should complete in < 200ms
- **Large file writes** (10MB): Should achieve > 20MB/s throughput
- **Directory operations**: Should complete in < 100ms

### Error Scenarios That Should Work

1. **Authentication failures**: Clear error messages about access denied
2. **Network timeouts**: Graceful handling with appropriate FUSE errors
3. **Non-existent files**: Proper ENOENT errors
4. **Invalid paths**: Appropriate error responses
5. **Write permission errors**: Clear read-only filesystem errors
6. **Directory not empty**: Proper ENOTEMPTY errors for directory removal

### Write Operation Capabilities

1. **Full filesystem semantics**: Create, read, write, delete files and directories
2. **Standard tool compatibility**: Works with cp, mv, rm, mkdir, rmdir
3. **Editor support**: Can use text editors to modify files
4. **Large file support**: Limited by available system memory
5. **Concurrent operations**: Thread-safe write operations
6. **Data integrity**: Files uploaded to S3 match written content

### Known Limitations

1. **Memory-buffered writes**: Large files consume memory during write operations
2. **No partial updates**: Files must be written completely before upload
3. **No atomic operations**: S3 eventual consistency limitations apply
4. **Sequential writes preferred**: Random access writes have limitations
5. **Copy-based renames**: Rename operations copy then delete in S3

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

4. **High memory usage**: Monitor memory during large file writes
   ```bash
   watch -n 1 'ps aux | grep s3fs-mount'
   ```

5. **Write operations failing**: Check if mounted in read-only mode
   ```bash
   mount | grep s3fs | grep -o 'ro\|rw'
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

# Monitor S3FS logs in debug mode
./s3-proxy s3fs-mount ... -debug
```

This comprehensive test suite validates all aspects of the S3FS implementation including read operations, write operations, error handling, performance, and edge cases. The write operation tests ensure that the filesystem behaves correctly for file creation, modification, deletion, and directory operations while respecting S3's object storage limitations. 