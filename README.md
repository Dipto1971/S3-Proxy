# S3 Proxy with Advanced S3FS Mount Support

This project implements a comprehensive S3 proxy with support for mounting S3 buckets as local filesystems using a custom FUSE implementation that supports both read and write operations.

## Features

### Core Capabilities
- **Read/Write Access**: Full read and write support for files and directories
- **Directory Operations**: Create, list, and remove directories
- **File Operations**: Create, read, write, delete, and rename files
- **Advanced Error Handling**: Comprehensive S3 error mapping to FUSE errors
- **Performance Optimizations**: Metadata caching and pagination for large directories
- **Cross-Platform Support**: Works on Linux and macOS (Windows via WSL2)

### Write Operation Features
- **File Creation**: Create new files with standard filesystem semantics
- **File Writing**: Write data to files (buffered until flush/close)
- **Directory Creation**: Create directory hierarchies
- **File/Directory Deletion**: Remove files and empty directories
- **Rename Operations**: Rename and move files/directories
- **Atomic Uploads**: Complete files are uploaded to S3 on flush/close operations

### Performance Features
- **Metadata Caching**: 30-second cache for file metadata to reduce S3 API calls
- **Pagination Support**: Efficient handling of large directories using S3 ListObjectsV2
- **Range Requests**: Efficient partial file reads using S3 range headers
- **Context Propagation**: Proper context handling for cancellation and timeouts

## Prerequisites

- Go 1.19 or later
- FUSE support on your system:
  - **Linux**: libfuse-dev package (`sudo apt-get install libfuse-dev`)
  - **macOS**: macFUSE (`brew install macfuse`)
  - **Windows**: Use WSL2 with Ubuntu and install libfuse-dev

## Installation

```bash
git clone <repository-url>
cd s3-proxy
go mod download
go build -o s3-proxy cmd/s3-proxy/main.go
```

## Commands

### S3FS Mount (Recommended)
Mount an S3 bucket as a local filesystem with full read/write support:

```bash
./s3-proxy s3fs-mount \
  -bucket <bucket-name> \
  -mount <local-path> \
  -endpoint <s3-endpoint> \
  -access-key <access-key> \
  -secret-key <secret-key> \
  -region <region>
```

#### Options:
- `-bucket`: S3 bucket name (required)
- `-mount`: Local directory to mount the bucket (required)
- `-endpoint`: S3 endpoint URL (default: "http://localhost:9000")
- `-access-key`: AWS access key (optional, uses environment if not provided)
- `-secret-key`: AWS secret key (optional, uses environment if not provided)
- `-region`: S3 region (default: "us-east-1")
- `-read-only`: Mount in read-only mode (default: false)
- `-debug`: Enable debug logging (default: false)

#### Examples:

**Basic mount with write support:**
```bash
./s3-proxy s3fs-mount -bucket my-bucket -mount ./my-mount
```

**Read-only mount:**
```bash
./s3-proxy s3fs-mount -bucket my-bucket -mount ./my-mount -read-only
```

**AWS S3 mount:**
```bash
./s3-proxy s3fs-mount \
  -bucket my-aws-bucket \
  -mount ./aws-mount \
  -endpoint https://s3.amazonaws.com \
  -region us-west-2 \
  -access-key AKIAIOSFODNN7EXAMPLE \
  -secret-key wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
```

**MinIO mount:**
```bash
./s3-proxy s3fs-mount \
  -bucket test-bucket \
  -mount ./minio-mount \
  -endpoint http://localhost:9000 \
  -access-key minio \
  -secret-key minio123
```

### Unmounting
To unmount the filesystem:

**Linux:**
```bash
fusermount -u <mount-point>
```

**macOS:**
```bash
umount <mount-point>
```

### Legacy S3 Mount
Traditional s3fs-fuse based mounting (read-only):

```bash
./s3-proxy s3-mount \
  -bucket <bucket-name> \
  -mount-point <local-path> \
  -access-key <access-key> \
  -secret-key <secret-key>
```

### Other Commands
- `s3-proxy`: Start the S3 proxy server
- `s3-write`: Write files to S3 directly
- `s3-read`: Read files from S3 directly  
- `s3-delete`: Delete files from S3 directly
- `cryption-keyset`: Generate encryption keys

## Usage Examples

### Basic File Operations

Once mounted, you can use standard filesystem operations:

```bash
# Create a directory
mkdir /mount/point/new-directory

# Create a file
echo "Hello, S3!" > /mount/point/hello.txt

# Read a file
cat /mount/point/hello.txt

# Copy files
cp local-file.txt /mount/point/
cp /mount/point/remote-file.txt ./

# List directory contents
ls -la /mount/point/

# Remove files
rm /mount/point/unwanted-file.txt

# Remove empty directories
rmdir /mount/point/empty-directory

# Rename/move files
mv /mount/point/old-name.txt /mount/point/new-name.txt
```

### Advanced Operations

```bash
# Create nested directories
mkdir -p /mount/point/deep/nested/structure

# Bulk operations
tar -xzf archive.tar.gz -C /mount/point/
rsync -av local-folder/ /mount/point/remote-folder/

# File permissions (basic support)
chmod 644 /mount/point/file.txt
```

## Write Operation Behavior

### Important Limitations
- **No Partial Updates**: S3 doesn't support partial file modifications. Files are uploaded completely on flush/close.
- **No Atomic Operations**: S3 operations are eventually consistent, not atomic like traditional filesystems.
- **Sequential Writes Preferred**: Random access writes are limited due to S3's object nature.
- **Memory Usage**: Large files are buffered in memory during write operations.

### Performance Considerations
- **Batch Operations**: Group multiple file operations when possible.
- **Large Files**: Consider the memory impact of buffering large files.
- **Network Latency**: Write operations depend on network speed to S3.
- **Consistency**: File operations may take time to become visible due to S3 eventual consistency.

## Error Handling

The filesystem provides comprehensive error mapping:

- **ENOENT**: File or bucket not found
- **EACCES**: Access denied or authentication failure  
- **ENOTDIR**: Attempting directory operations on files
- **ENOTEMPTY**: Attempting to remove non-empty directories
- **EEXIST**: Resource already exists
- **EIO**: General I/O errors and S3 communication issues

## Troubleshooting

### Common Issues

**Mount fails with "Transport endpoint is not connected":**
- Ensure FUSE is properly installed and loaded
- Check that the mount point is empty
- Verify S3 credentials and connectivity

**High memory usage:**
- Large file writes buffer content in memory
- Consider using smaller files or streaming operations
- Monitor system memory when working with large files

**Slow performance:**
- Enable metadata caching (enabled by default)
- Use batch operations when possible
- Check network connectivity to S3 endpoint

**Permission denied errors:**
- Verify S3 credentials have proper permissions
- Check bucket policies and IAM roles
- Ensure the bucket exists and is accessible

### Debug Mode

Enable debug logging for troubleshooting:

```bash
./s3-proxy s3fs-mount -bucket my-bucket -mount ./mount -debug
```

This provides detailed logging of:
- S3 API calls and responses
- FUSE operations and timing
- Cache operations and hits/misses
- Error details and stack traces

## Configuration

### Environment Variables
The application supports standard AWS environment variables:
- `AWS_ACCESS_KEY_ID`
- `AWS_SECRET_ACCESS_KEY`
- `AWS_REGION`
- `AWS_ENDPOINT_URL`

### .env File Support
Create a `.env` file in the project root:
```
AWS_ACCESS_KEY_ID=your-access-key
AWS_SECRET_ACCESS_KEY=your-secret-key
AWS_REGION=us-east-1
AWS_ENDPOINT_URL=http://localhost:9000
```

## Architecture

### S3 Directory Mapping
- **Explicit Directories**: Objects ending with "/" are treated as directories
- **Implicit Directories**: Inferred from object prefixes during listing
- **Root Directory**: The bucket root is mapped to the mount point

### Write Operation Flow
1. **File Creation**: Creates in-memory buffer
2. **Write Operations**: Append to buffer
3. **Flush/Close**: Upload complete buffer to S3
4. **Cache Update**: Refresh metadata cache with new file info

### Error Mapping
S3 errors are carefully mapped to appropriate FUSE/POSIX errors to provide a native filesystem experience.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure all tests pass
5. Submit a pull request

## License

[Add your license information here]

## TODOs

### High Priority
- [ ] Multipart upload support for large files
- [ ] Write-through caching for better performance
- [ ] Comprehensive test suite
- [ ] File locking mechanisms

### Medium Priority  
- [ ] Extended attribute support
- [ ] Symbolic link handling
- [ ] Background sync operations
- [ ] Compression support

### Low Priority
- [ ] Encryption at rest options
- [ ] Alternative storage backends
- [ ] GUI management interface
- [ ] Performance monitoring dashboard
