# S3FS Integration Debug Guide

This guide helps debug the issue where s3fs doesn't send GET requests to s3-proxy during `cat` operations.

## Problem Description

- s3fs can successfully mount and perform `ls` (list) and write operations (`echo "123" > file.txt`)
- However, when performing `cat file.txt`, s3fs doesn't send a GET request to s3-proxy
- This prevents the proxy from decrypting and returning the file content

## Debugging Steps

### Step 1: Enhanced Logging

The s3-proxy has been enhanced with detailed logging. When you run it, you'll see:
- All incoming requests with full headers
- Path parsing details
- Bucket and key extraction
- Authentication flow

### Step 2: Use the Debug Server

Run the debug server to see exactly what requests s3fs sends:

```bash
# Terminal 1: Start debug server
go run ./cmd/s3-proxy/main.go debug --port=8081

# Terminal 2: Configure s3fs to use debug server
export ENDPOINT="http://localhost:8081"
./debug_s3fs.sh
```

This will show you exactly what HTTP requests s3fs is making.

### Step 3: Run S3FS Debug Script

The `debug_s3fs.sh` script performs a complete test cycle:

```bash
# Set your environment variables
export BUCKET_NAME="test-bucket"
export ACCESS_KEY="your-access-key"
export SECRET_KEY="your-secret-key"
export ENDPOINT="http://localhost:8080"

# Run the debug script
./debug_s3fs.sh
```

### Step 4: Check S3FS Configuration

Common s3fs configuration issues that can cause this problem:

1. **Missing sigv4 option**: Ensure you're using `-o sigv4`
2. **Wrong endpoint format**: Use `-o url="http://localhost:8080"`
3. **Path style requests**: Use `-o use_path_request_style`
4. **Region mismatch**: Ensure region matches your s3-proxy config

### Step 5: Analyze the Logs

Look for these patterns in the s3-proxy logs:

#### Expected for successful operations:
```
Received request: GET /test-bucket/file.txt
Path parsing - Original path: '/test-bucket/file.txt', Trimmed path: 'test-bucket/file.txt', Parts: [test-bucket file.txt]
Parsed - Bucket: 'test-bucket', Key: 'file.txt'
Found bucket configuration for: test-bucket
Starting GET operation for bucket: test-bucket, key: file.txt
```

#### Signs of problems:
```
No bucket configuration found for: [bucket-name]
Authentication failed: [error]
Starting PROXY operation (indicates fallback to proxy mode)
```

## Common Issues and Solutions

### Issue 1: Authentication Problems

**Symptoms**: 401 Unauthorized errors
**Solution**: Check that:
- `AUTH_HEADER_FORMAT` environment variable is set (e.g., "AWS4-HMAC-SHA256")
- Access keys in config match what s3fs is using
- s3fs is configured with correct credentials

### Issue 2: Bucket Not Found

**Symptoms**: Requests go to proxy handler instead of direct handlers
**Solution**: Verify:
- Bucket name in s3fs mount matches bucket name in config
- s3-proxy config has the bucket properly defined

### Issue 3: S3FS Caching

**Symptoms**: s3fs doesn't send GET requests for recently written files
**Solution**: Try mounting with cache-disabling options:
```bash
s3fs bucket /mount/point -o use_cache=/tmp -o ensure_diskfree=100
```

### Issue 4: Request Format Issues

**Symptoms**: Requests don't match expected path format
**Solution**: Ensure s3fs uses path-style requests:
```bash
s3fs bucket /mount/point -o use_path_request_style
```

## Advanced Debugging

### Enable s3fs Debug Logging

Mount s3fs with maximum debugging:
```bash
s3fs bucket /mount/point \
    -o dbglevel=info \
    -o curldbg \
    -o f2 \
    -o logfile=/tmp/s3fs.log
```

Then check `/tmp/s3fs.log` for detailed s3fs operations.

### Network Traffic Analysis

Use tcpdump or Wireshark to capture actual HTTP traffic:
```bash
sudo tcpdump -i lo -A -s 0 'port 8080'
```

### Test with curl

Verify the s3-proxy works correctly with direct HTTP requests:
```bash
# Test GET request
curl -v -H "Authorization: AWS4-HMAC-SHA256 Credential=AKIAEXAMPLEACCESSKEY1/20250516/us-east-1/s3/aws4_request" \
     http://localhost:8080/test-bucket/file.txt
```

## Expected Behavior

When working correctly, a `cat` operation should generate logs like:
```
Received request: GET /test-bucket/file.txt
Authentication successful for request: GET /test-bucket/file.txt
Starting GET operation for bucket: test-bucket, key: file.txt
Attempting to fetch from backend: [backend-endpoint], target bucket: [target-bucket]
Successfully fetched object from backend: [backend]
Decrypting data from backend: [backend]
GET operation completed successfully from backend: [backend] for bucket: test-bucket, key: file.txt
```

## Next Steps

If the issue persists after following this guide:

1. Share the complete logs from both s3-proxy and s3fs
2. Provide the exact s3fs mount command used
3. Show the s3-proxy configuration file
4. Include output from the debug server showing actual HTTP requests

This will help identify the specific cause of the issue. 