//go:build !windows

//internal/fusefs/s3fs.go

package fusefs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"s3-proxy/internal/client"
	"s3-proxy/internal/crypto"
	"strings"
	"sync"
	"syscall"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3FS represents the S3 file system.
// This implementation provides read/write access to S3 buckets as a FUSE filesystem.
// S3 directories are simulated using prefixes - objects ending with '/' are treated as directories,
// while objects without trailing '/' are treated as files.
//
// Write operations are supported but note that S3 doesn't support true atomic operations
// or partial writes, so files are uploaded as complete objects on close/sync.
//
// Encryption/decryption is supported through the crypto field. When encryption is enabled,
// data is encrypted before upload and decrypted after download transparently.
type S3FS struct {
	s3Client *client.S3
	bucket   string
	crypto   crypto.Crypt // Optional encryption/decryption
	// Cache for metadata to reduce S3 API calls
	metadataCache map[string]*cachedMetadata
	cacheMux      sync.RWMutex
}

type cachedMetadata struct {
	size         int64
	lastModified time.Time
	isDir        bool
	cacheTime    time.Time
}

// NewS3FS creates a new S3FS instance with optional encryption support.
// If crypto is nil, data will be stored in plaintext.
func NewS3FS(s3Client *client.S3, bucket string, crypto crypto.Crypt) *S3FS {
	return &S3FS{
		s3Client:      s3Client,
		bucket:        bucket,
		crypto:        crypto,
		metadataCache: make(map[string]*cachedMetadata),
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
		a.Mode = os.ModeDir | 0755 // Read-write directory
		a.Size = 0
		a.Mtime = time.Now() // Use current time for directories
		return nil
	}

	// Check cache first
	if cached := node.getCachedMetadata(); cached != nil {
		a.Mode = 0644 // Read-write file
		a.Size = uint64(cached.size)
		a.Mtime = cached.lastModified
		return nil
	}

	// Fetch file metadata with proper error handling and context
	obj, err := node.fs.s3Client.Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &node.fs.bucket,
		Key:    &node.key,
	})
	if err != nil {
		return mapS3Error(err, node.key)
	}

	a.Mode = 0644 // Read-write file
	// For encrypted files, we store the encrypted size in S3 but report the decrypted size
	// Since we can't easily determine the decrypted size without downloading and decrypting,
	// we'll report the encrypted size for now. This is a limitation we can optimize later.
	a.Size = uint64(*obj.ContentLength)
	if obj.LastModified != nil {
		a.Mtime = *obj.LastModified
	} else {
		a.Mtime = time.Now()
	}

	// Cache the metadata
	node.cacheMetadata(*obj.ContentLength, a.Mtime, false)

	return nil
}

// getCachedMetadata retrieves cached metadata if valid (within 30 seconds)
func (node *S3Node) getCachedMetadata() *cachedMetadata {
	node.fs.cacheMux.RLock()
	defer node.fs.cacheMux.RUnlock()

	cached, exists := node.fs.metadataCache[node.key]
	if !exists {
		return nil
	}

	// Cache valid for 30 seconds
	if time.Since(cached.cacheTime) > 30*time.Second {
		return nil
	}

	return cached
}

// cacheMetadata stores metadata in cache
func (node *S3Node) cacheMetadata(size int64, lastModified time.Time, isDir bool) {
	node.fs.cacheMux.Lock()
	defer node.fs.cacheMux.Unlock()

	node.fs.metadataCache[node.key] = &cachedMetadata{
		size:         size,
		lastModified: lastModified,
		isDir:        isDir,
		cacheTime:    time.Now(),
	}
}

// invalidateCache removes cached metadata for this node
func (node *S3Node) invalidateCache() {
	node.fs.cacheMux.Lock()
	defer node.fs.cacheMux.Unlock()
	delete(node.fs.metadataCache, node.key)
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

// Open implements fs.NodeOpener, opening a file for reading or writing.
func (node *S3Node) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	if node.isDir {
		return node, nil // Return directory handle for directory
	}

	// For write access, we don't need to validate the file exists (it may be created)
	if req.Flags&fuse.OpenWriteOnly != 0 || req.Flags&fuse.OpenReadWrite != 0 {
		return &S3FileHandle{node: node, writable: true}, nil
	}

	// For read-only access, validate that the file exists
	_, err := node.fs.s3Client.Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &node.fs.bucket,
		Key:    &node.key,
	})
	if err != nil {
		return nil, mapS3Error(err, node.key)
	}

	return &S3FileHandle{node: node, writable: false}, nil
}

// Create implements fs.NodeCreater, creating new files.
func (node *S3Node) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	if !node.isDir {
		return nil, nil, fuse.Errno(syscall.ENOTDIR)
	}

	var fileKey string
	if node.key == "" {
		fileKey = req.Name
	} else {
		fileKey = node.key + req.Name
	}

	newNode := &S3Node{
		fs:    node.fs,
		key:   fileKey,
		isDir: false,
	}

	handle := &S3FileHandle{
		node:     newNode,
		writable: true,
		data:     new(bytes.Buffer),
		dirty:    true,
	}

	resp.Attr.Mode = 0644
	resp.Attr.Size = 0
	resp.Attr.Mtime = time.Now()

	log.Printf("Created new file: %s", fileKey)
	return newNode, handle, nil
}

// Mkdir implements fs.NodeMkdirer, creating new directories.
func (node *S3Node) Mkdir(ctx context.Context, req *fuse.MkdirRequest) (fs.Node, error) {
	if !node.isDir {
		return nil, fuse.Errno(syscall.ENOTDIR)
	}

	var dirKey string
	if node.key == "" {
		dirKey = req.Name + "/"
	} else {
		dirKey = node.key + req.Name + "/"
	}

	// Create an empty object with trailing slash to represent the directory
	// Note: Directory markers are not encrypted as they're empty
	_, err := node.fs.s3Client.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &node.fs.bucket,
		Key:    &dirKey,
		Body:   bytes.NewReader([]byte{}),
	})
	if err != nil {
		return nil, mapS3Error(err, dirKey)
	}

	newNode := &S3Node{
		fs:    node.fs,
		key:   dirKey,
		isDir: true,
	}

	log.Printf("Created new directory: %s", dirKey)
	return newNode, nil
}

// Remove implements fs.NodeRemover, removing files and directories.
func (node *S3Node) Remove(ctx context.Context, req *fuse.RemoveRequest) error {
	if !node.isDir {
		return fuse.Errno(syscall.ENOTDIR)
	}

	var targetKey string
	if node.key == "" {
		targetKey = req.Name
	} else {
		targetKey = node.key + req.Name
	}

	if req.Dir {
		// Removing a directory
		dirKey := targetKey + "/"

		// Check if directory is empty
		listOutput, err := node.fs.s3Client.Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:  &node.fs.bucket,
			Prefix:  &dirKey,
			MaxKeys: aws.Int32(1),
		})
		if err != nil {
			return mapS3Error(err, dirKey)
		}

		// Count objects excluding the directory marker itself
		nonEmptyCount := 0
		for _, obj := range listOutput.Contents {
			if *obj.Key != dirKey {
				nonEmptyCount++
			}
		}

		if nonEmptyCount > 0 {
			return fuse.Errno(syscall.ENOTEMPTY)
		}

		// Delete the directory marker
		_, err = node.fs.s3Client.Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: &node.fs.bucket,
			Key:    &dirKey,
		})
		if err != nil {
			return mapS3Error(err, dirKey)
		}

		log.Printf("Removed directory: %s", dirKey)
	} else {
		// Removing a file
		_, err := node.fs.s3Client.Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: &node.fs.bucket,
			Key:    &targetKey,
		})
		if err != nil {
			return mapS3Error(err, targetKey)
		}

		// Invalidate cache
		targetNode := &S3Node{fs: node.fs, key: targetKey}
		targetNode.invalidateCache()

		log.Printf("Removed file: %s", targetKey)
	}

	return nil
}

// Rename implements fs.NodeRenamer, renaming files and directories.
func (node *S3Node) Rename(ctx context.Context, req *fuse.RenameRequest, newDir fs.Node) error {
	if !node.isDir {
		return fuse.Errno(syscall.ENOTDIR)
	}

	newDirNode, ok := newDir.(*S3Node)
	if !ok {
		return fuse.Errno(syscall.EXDEV) // Cross-device link
	}

	var oldKey, newKey string
	if node.key == "" {
		oldKey = req.OldName
	} else {
		oldKey = node.key + req.OldName
	}

	if newDirNode.key == "" {
		newKey = req.NewName
	} else {
		newKey = newDirNode.key + req.NewName
	}

	// For S3, rename is implemented as copy + delete
	// Note: This preserves the encrypted state of the file
	// First, copy the object
	_, err := node.fs.s3Client.Client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     &node.fs.bucket,
		Key:        &newKey,
		CopySource: aws.String(node.fs.bucket + "/" + oldKey),
	})
	if err != nil {
		return mapS3Error(err, fmt.Sprintf("copy %s to %s", oldKey, newKey))
	}

	// Then delete the original
	_, err = node.fs.s3Client.Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &node.fs.bucket,
		Key:    &oldKey,
	})
	if err != nil {
		// Try to clean up the copy if delete fails
		node.fs.s3Client.Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: &node.fs.bucket,
			Key:    &newKey,
		})
		return mapS3Error(err, oldKey)
	}

	// Invalidate cache for both old and new keys
	oldNode := &S3Node{fs: node.fs, key: oldKey}
	newNode := &S3Node{fs: node.fs, key: newKey}
	oldNode.invalidateCache()
	newNode.invalidateCache()

	log.Printf("Renamed %s to %s", oldKey, newKey)
	return nil
}

// S3FileHandle represents an open file handle with read/write support.
type S3FileHandle struct {
	node     *S3Node
	writable bool
	data     *bytes.Buffer // Buffer for write operations
	dirty    bool          // Whether the buffer has been modified
	mu       sync.Mutex    // Protects concurrent access
}

// Read implements fs.HandleReader, reading file contents with range support.
// For encrypted files, we need to decrypt the entire file first, then slice the requested range.
func (h *S3FileHandle) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// If we have dirty data and we're reading, we need to read from our buffer
	if h.dirty && h.data != nil {
		bufferData := h.data.Bytes()
		start := req.Offset
		end := req.Offset + int64(req.Size)

		if start >= int64(len(bufferData)) {
			resp.Data = []byte{}
			return nil
		}

		if end > int64(len(bufferData)) {
			end = int64(len(bufferData))
		}

		resp.Data = bufferData[start:end]
		log.Printf("Read %d bytes from buffer for %s (offset: %d, requested: %d)", len(resp.Data), h.node.key, req.Offset, req.Size)
		return nil
	}

	// For encrypted files, we must read the entire object to decrypt it properly
	// This is a limitation that could be optimized with streaming encryption in the future
	input := &s3.GetObjectInput{
		Bucket: &h.node.fs.bucket,
		Key:    &h.node.key,
	}

	// If encryption is disabled, we can use range requests for efficiency
	if h.node.fs.crypto == nil && (req.Offset > 0 || req.Size > 0) {
		end := req.Offset + int64(req.Size) - 1
		rangeHeader := fmt.Sprintf("bytes=%d-%d", req.Offset, end)
		input.Range = &rangeHeader
	}

	obj, err := h.node.fs.s3Client.Client.GetObject(ctx, input)
	if err != nil {
		return mapS3Error(err, h.node.key)
	}
	defer obj.Body.Close()

	// Read the data from S3
	data, err := io.ReadAll(obj.Body)
	if err != nil {
		log.Printf("Error reading from S3 object %s: %v", h.node.key, err)
		return mapS3Error(err, h.node.key)
	}

	// Decrypt if encryption is enabled
	if h.node.fs.crypto != nil {
		data, err = h.node.fs.crypto.Decrypt(data)
		if err != nil {
			log.Printf("Error decrypting data for %s: %v", h.node.key, err)
			return mapCryptoError(err, h.node.key)
		}
		log.Printf("Successfully decrypted %d bytes for %s", len(data), h.node.key)
	}

	// Slice the requested range from the decrypted data
	start := req.Offset
	end := req.Offset + int64(req.Size)

	if start >= int64(len(data)) {
		resp.Data = []byte{}
		return nil
	}

	if end > int64(len(data)) {
		end = int64(len(data))
	}

	if start < 0 {
		start = 0
	}

	resp.Data = data[start:end]
	log.Printf("Read %d bytes from S3 for %s (offset: %d, requested: %d, decrypted size: %d)", len(resp.Data), h.node.key, req.Offset, req.Size, len(data))
	return nil
}

// Write implements fs.HandleWriter, writing file contents.
func (h *S3FileHandle) Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
	if !h.writable {
		return fuse.Errno(syscall.EBADF)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Initialize buffer if needed
	if h.data == nil {
		h.data = new(bytes.Buffer)
	}

	// For simplicity, we only support sequential writes or writes at the end
	// S3 doesn't support partial object updates, so we buffer everything
	currentSize := int64(h.data.Len())

	if req.Offset != currentSize {
		// For non-sequential writes, we need to read the existing content first
		if req.Offset > currentSize {
			// Pad with zeros if writing beyond current size
			padding := make([]byte, req.Offset-currentSize)
			h.data.Write(padding)
		} else {
			// For writes in the middle, we'd need to reconstruct the buffer
			// This is complex with S3's limitations, so we return an error
			return fuse.Errno(syscall.ESPIPE)
		}
	}

	// Write the data to the buffer
	n, err := h.data.Write(req.Data)
	if err != nil {
		return fuse.Errno(syscall.EIO)
	}

	h.dirty = true
	resp.Size = n

	log.Printf("Wrote %d bytes to buffer for %s (offset: %d)", n, h.node.key, req.Offset)
	return nil
}

// Flush implements fs.HandleFlusher, flushing changes to S3.
// This is where encryption happens before uploading to S3.
func (h *S3FileHandle) Flush(ctx context.Context, req *fuse.FlushRequest) error {
	if !h.writable || !h.dirty {
		return nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.data == nil {
		return nil
	}

	dataToUpload := h.data.Bytes()

	// Encrypt the data if encryption is enabled
	if h.node.fs.crypto != nil {
		encryptedData, err := h.node.fs.crypto.Encrypt(dataToUpload)
		if err != nil {
			log.Printf("Error encrypting data for %s: %v", h.node.key, err)
			return mapCryptoError(err, h.node.key)
		}
		dataToUpload = encryptedData
		log.Printf("Successfully encrypted %d bytes to %d bytes for %s", h.data.Len(), len(dataToUpload), h.node.key)
	}

	// Upload the data to S3
	_, err := h.node.fs.s3Client.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &h.node.fs.bucket,
		Key:    &h.node.key,
		Body:   bytes.NewReader(dataToUpload),
	})
	if err != nil {
		return mapS3Error(err, h.node.key)
	}

	h.dirty = false

	// Invalidate cache and update with new metadata
	// Note: We cache the decrypted size, not the encrypted size
	h.node.invalidateCache()
	h.node.cacheMetadata(int64(h.data.Len()), time.Now(), false)

	if h.node.fs.crypto != nil {
		log.Printf("Flushed %d bytes (encrypted to %d bytes) to S3 for %s", h.data.Len(), len(dataToUpload), h.node.key)
	} else {
		log.Printf("Flushed %d bytes to S3 for %s", h.data.Len(), h.node.key)
	}
	return nil
}

// Release implements fs.HandleReleaser, releasing the file handle.
func (h *S3FileHandle) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	// Ensure any pending writes are flushed
	if h.writable && h.dirty {
		return h.Flush(ctx, &fuse.FlushRequest{})
	}
	return nil
}

// mapS3Error maps S3 errors to appropriate FUSE/syscall errors with detailed context
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
	case *types.BucketAlreadyExists:
		return fuse.Errno(syscall.EEXIST)
	case *types.BucketAlreadyOwnedByYou:
		return fuse.Errno(syscall.EEXIST)
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
		case strings.Contains(errStr, "BucketAlreadyExists"):
			return fuse.Errno(syscall.EEXIST)
		case strings.Contains(errStr, "ObjectNotInActiveTierError"):
			return fuse.Errno(syscall.EACCES)
		default:
			return fuse.Errno(syscall.EIO) // Generic I/O error
		}
	}
}

// mapCryptoError maps encryption/decryption errors to appropriate FUSE errors
func mapCryptoError(err error, resource string) error {
	if err == nil {
		return nil
	}

	log.Printf("Crypto error for resource '%s': %v", resource, err)

	// Most crypto errors should be treated as I/O errors
	// We could be more specific based on error types in the future
	return fuse.Errno(syscall.EIO)
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
