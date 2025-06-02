# S3 Proxy with s3fs Mount Support

This project implements an S3 proxy with support for mounting S3 buckets as local filesystems using s3fs-fuse.

## Prerequisites

- Go 1.16 or later
- s3fs-fuse installed on your system
  - For Ubuntu/Debian: `sudo apt-get install s3fs`
  - For macOS: `brew install s3fs-fuse`
  - For Windows: Use WSL2 with Ubuntu and install s3fs there

## Commands

### S3 Mount
Mount an S3 bucket as a local filesystem:

```bash
./s3-proxy s3-mount -bucket <bucket-name> -mount-point <local-path> -access-key <access-key> -secret-key <secret-key>
```

Options:
- `-bucket`: S3 bucket name (required)
- `-mount-point`: Local directory to mount the bucket (required)
- `-access-key`: AWS access key (required)
- `-secret-key`: AWS secret key (required)

To unmount the bucket:
```bash
fusermount -u <mount-point>
```

### Other Commands
- `s3-proxy`: Start the S3 proxy server
- `s3-write`: Write files to S3
- `s3-read`: Read files from S3
- `s3-delete`: Delete files from S3
- `cryption-keyset`: Generate encryption keys

## Configuration

The S3 proxy runs on `http://localhost:8080` by default. The s3fs mount command is configured to use this endpoint automatically.

## Security

The credentials file for s3fs is stored at `~/.passwd-s3fs` with 600 permissions. The access key must match one of the allowed keys in the proxy's configuration.

# TODOs

1. Proper error handling
2. tests
3. Proper proxying
4. Auth
5. More encryption algorithms
6. Optimise
