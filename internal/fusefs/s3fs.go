//go:build !windows

//internal/fusefs/s3fs.go

package fusefs

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"s3-proxy/internal/client"
	"strings"
	"syscall"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3FS represents the S3 file system.
// This implementation provides read-only access to S3 buckets as a FUSE filesystem.
// S3 directories are simulated using prefixes - objects ending with '/' are treated as directories,
// while objects without trailing '/' are treated as files.
type S3FS struct {
	s3Client *client.S3
	bucket   string
}

// NewS3FS creates a new S3FS instance.
func NewS3FS(s3Client *client.S3, bucket string) *S3FS {
	return &S3FS{
		s3Client: s3Client,
		bucket:   bucket,
	}
}

// ValidateBucket checks if the bucket exists and is accessible
func (fs *S3FS) ValidateBucket(ctx context.Context) error {
	_, err := fs.s3Client.Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: &fs.bucket,
	})
	if err != nil {
		return mapS3Error(err, fmt.Sprintf("bucket '%s'", fs.bucket))
	}
	return nil
}

// Root implements fs.FS, returning the root node of the file system.
func (fs *S3FS) Root() (fs.Node, error) {
	return &S3Node{
		fs:    fs,
		key:   "",
		isDir: true,
	}, nil
}

// S3Node represents a file or directory in the S3 bucket.
// For directories (isDir=true), the key typically ends with '/'
// For files (isDir=false), the key is the full object path without trailing '/'
type S3Node struct {
	fs    *S3FS
	key   string
	isDir bool
}

// Attr implements fs.Node, returning node attributes.
func (node *S3Node) Attr(ctx context.Context, a *fuse.Attr) error {
	if node.isDir {
		a.Mode = os.ModeDir | 0555 // Read-only directory
		a.Size = 0
		a.Mtime = time.Now() // Use current time for directories
		return nil
	}

	// Fetch file metadata with proper error handling
	obj, err := node.fs.s3Client.Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &node.fs.bucket,
		Key:    &node.key,
	})
	if err != nil {
		return mapS3Error(err, node.key)
	}

	a.Mode = 0444 // Read-only file
	a.Size = uint64(*obj.ContentLength)
	if obj.LastModified != nil {
		a.Mtime = *obj.LastModified
	} else {
		a.Mtime = time.Now()
	}

	// Set additional attributes if available
	if obj.ETag != nil {
		// Could use ETag for inode simulation, but keeping simple for now
	}

	return nil
}

// Lookup implements fs.NodeRequestLookuper, finding a child node by name.
func (node *S3Node) Lookup(ctx context.Context, name string) (fs.Node, error) {
	if !node.isDir {
		return nil, fuse.Errno(syscall.ENOTDIR)
	}

	var dirKey, fileKey string
	if node.key == "" {
		dirKey = name + "/"
		fileKey = name
	} else {
		dirKey = node.key + name + "/"
		fileKey = node.key + name
	}

	// Check if explicit directory exists (object with trailing /)
	_, err := node.fs.s3Client.Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &node.fs.bucket,
		Key:    &dirKey,
	})
	if err == nil {
		return &S3Node{fs: node.fs, key: dirKey, isDir: true}, nil
	}

	// Check if file exists
	_, err = node.fs.s3Client.Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &node.fs.bucket,
		Key:    &fileKey,
	})
	if err == nil {
		return &S3Node{fs: node.fs, key: fileKey, isDir: false}, nil
	}

	// Check for implicit directory (subobjects exist with this prefix)
	listOutput, err := node.fs.s3Client.Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  &node.fs.bucket,
		Prefix:  &dirKey,
		MaxKeys: aws.Int32(1),
	})
	if err != nil {
		return nil, mapS3Error(err, dirKey)
	}

	if len(listOutput.Contents) > 0 {
		return &S3Node{fs: node.fs, key: dirKey, isDir: true}, nil
	}

	return nil, fuse.ENOENT
}

// ReadDirAll implements fs.HandleReadDirAller, listing directory contents.
// This implementation includes pagination support for large directories.
func (node *S3Node) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	if !node.isDir {
		return nil, fuse.Errno(syscall.ENOTDIR)
	}

	var entries []fuse.Dirent
	var continuationToken *string

	// Paginate through all results for large directories
	for {
		listInput := &s3.ListObjectsV2Input{
			Bucket:    &node.fs.bucket,
			Prefix:    &node.key,
			Delimiter: aws.String("/"),
			MaxKeys:   aws.Int32(1000), // Process in chunks of 1000
		}

		if continuationToken != nil {
			listInput.ContinuationToken = continuationToken
		}

		listOutput, err := node.fs.s3Client.Client.ListObjectsV2(ctx, listInput)
		if err != nil {
			return nil, mapS3Error(err, node.key)
		}

		// Add subdirectories from CommonPrefixes
		for _, prefix := range listOutput.CommonPrefixes {
			name := strings.TrimSuffix(*prefix.Prefix, "/")
			if node.key != "" {
				name = name[len(node.key):]
			}
			if name != "" && !strings.Contains(name, "/") { // Skip empty names and nested paths
				entries = append(entries, fuse.Dirent{
					Name: name,
					Type: fuse.DT_Dir,
				})
			}
		}

		// Add files from Contents
		for _, obj := range listOutput.Contents {
			if *obj.Key == node.key || strings.HasSuffix(*obj.Key, "/") {
				continue // Skip directory object itself
			}
			name := (*obj.Key)[len(node.key):]
			if name != "" && !strings.Contains(name, "/") { // Skip empty names and nested paths
				entries = append(entries, fuse.Dirent{
					Name: name,
					Type: fuse.DT_File,
				})
			}
		}

		// Check if there are more results
		if listOutput.IsTruncated != nil && *listOutput.IsTruncated {
			continuationToken = listOutput.NextContinuationToken
		} else {
			break
		}
	}

	log.Printf("Listed %d entries for directory: %s", len(entries), node.key)
	return entries, nil
}

// Open implements fs.NodeOpener, opening a file for reading.
func (node *S3Node) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	if node.isDir {
		return node, nil // Return directory handle for directory
	}

	// Validate that the file exists before opening
	_, err := node.fs.s3Client.Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &node.fs.bucket,
		Key:    &node.key,
	})
	if err != nil {
		return nil, mapS3Error(err, node.key)
	}

	return &S3FileHandle{node: node}, nil
}

// S3FileHandle represents an open file handle.
type S3FileHandle struct {
	node *S3Node
}

// Read implements fs.HandleReader, reading file contents with range support.
func (h *S3FileHandle) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	// Use Range header for efficient partial reads
	input := &s3.GetObjectInput{
		Bucket: &h.node.fs.bucket,
		Key:    &h.node.key,
	}

	// Add range header if we're not reading from the beginning or have a specific size
	if req.Offset > 0 || req.Size > 0 {
		end := req.Offset + int64(req.Size) - 1
		rangeHeader := fmt.Sprintf("bytes=%d-%d", req.Offset, end)
		input.Range = &rangeHeader
	}

	obj, err := h.node.fs.s3Client.Client.GetObject(ctx, input)
	if err != nil {
		return mapS3Error(err, h.node.key)
	}
	defer obj.Body.Close()

	data := make([]byte, req.Size)
	n, err := io.ReadFull(obj.Body, data)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		log.Printf("Error reading from S3 object %s: %v", h.node.key, err)
		return mapS3Error(err, h.node.key)
	}

	resp.Data = data[:n]
	log.Printf("Read %d bytes from %s (offset: %d, requested: %d)", n, h.node.key, req.Offset, req.Size)
	return nil
}

// mapS3Error maps S3 errors to appropriate FUSE/syscall errors
func mapS3Error(err error, resource string) error {
	if err == nil {
		return nil
	}

	log.Printf("S3 error for resource '%s': %v", resource, err)

	// Handle AWS SDK v2 error types
	switch err.(type) {
	case *types.NoSuchBucket:
		return fuse.Errno(syscall.ENOENT)
	case *types.NoSuchKey:
		return fuse.ENOENT
	default:
		// Check error string for common patterns
		errStr := err.Error()
		switch {
		case strings.Contains(errStr, "NoSuchBucket"):
			return fuse.Errno(syscall.ENOENT)
		case strings.Contains(errStr, "NoSuchKey"):
			return fuse.ENOENT
		case strings.Contains(errStr, "AccessDenied"):
			return fuse.Errno(syscall.EACCES)
		case strings.Contains(errStr, "Forbidden"):
			return fuse.Errno(syscall.EACCES)
		case strings.Contains(errStr, "InvalidAccessKeyId"):
			return fuse.Errno(syscall.EACCES)
		case strings.Contains(errStr, "SignatureDoesNotMatch"):
			return fuse.Errno(syscall.EACCES)
		default:
			return fuse.Errno(syscall.EIO) // Generic I/O error
		}
	}
}

// Helper function to compute parent key
func parentKey(key string) string {
	if key == "" {
		return ""
	}
	if strings.HasSuffix(key, "/") {
		key = key[:len(key)-1]
	}
	lastSlash := strings.LastIndex(key, "/")
	if lastSlash == -1 {
		return ""
	}
	return key[:lastSlash+1]
}
