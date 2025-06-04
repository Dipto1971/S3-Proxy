//go:build !windows

// cmd/s3fs_mount.go

package cmd

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"s3-proxy/internal/client"
	"s3-proxy/internal/config"
	"s3-proxy/internal/crypto"
	"s3-proxy/internal/fusefs"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

// S3FSMount mounts an S3 bucket as a FUSE filesystem with read/write support
func S3FSMount() error {
	flagSet := flag.NewFlagSet("s3fs-mount", flag.ExitOnError)
	mountPoint := flagSet.String("mount", "", "Mount point directory")
	bucket := flagSet.String("bucket", "test-bucket", "S3 bucket name (as defined in config)")
	debug := flagSet.Bool("debug", false, "Enable debug logging")
	readOnly := flagSet.Bool("read-only", false, "Mount in read-only mode")
	configPath := flagSet.String("config", "configs/main.yaml", "Path to configuration file")
	accessKey := flagSet.String("access-key", "", "Access key for authentication (proxy-level, not S3 backend)")

	if err := flagSet.Parse(os.Args[1:]); err != nil {
		return fmt.Errorf("failed to parse flags: %v", err)
	}

	if *mountPoint == "" {
		return fmt.Errorf("mount point is required (use -mount flag)")
	}

	if *bucket == "" {
		return fmt.Errorf("bucket name is required (use -bucket flag)")
	}

	// Load configuration - this is required for S3FS
	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("failed to load config from %s: %v. S3FS requires configuration for backend and encryption setup", *configPath, err)
	}

	// Validate access key if provided
	if *accessKey != "" {
		if !cfg.Auth.IsValidAccessKey(*accessKey) {
			return fmt.Errorf("invalid access key: %s", *accessKey)
		}
		log.Printf("Access key validated successfully: %s", *accessKey)
	} else {
		log.Printf("Warning: No access key provided. Proceeding without authentication validation.")
	}

	// Find the bucket configuration
	var bucketConfig *config.ConfigS3Bucket
	for _, b := range cfg.S3Buckets {
		if b.BucketName == *bucket {
			bucketConfig = &b
			break
		}
	}

	if bucketConfig == nil {
		return fmt.Errorf("bucket '%s' not found in configuration. Available buckets: %v", *bucket, getBucketNames(cfg))
	}

	if len(bucketConfig.Backends) == 0 {
		return fmt.Errorf("bucket '%s' has no backends configured", *bucket)
	}

	// Use the first backend for S3FS (same logic as the main proxy for single operations)
	firstBackend := bucketConfig.Backends[0]
	log.Printf("Using first backend for S3FS: s3_client_id=%s, s3_bucket_name=%s, crypto_id=%s",
		firstBackend.S3ClientID, firstBackend.S3BucketName, firstBackend.CryptoID)

	// Find the S3 client configuration
	var s3ClientConfig *config.ConfigS3Client
	for _, client := range cfg.S3Clients {
		if client.ID == firstBackend.S3ClientID {
			s3ClientConfig = &client
			break
		}
	}

	if s3ClientConfig == nil {
		return fmt.Errorf("S3 client '%s' not found in configuration", firstBackend.S3ClientID)
	}

	// Create S3 client using the backend configuration
	s3Client, err := client.NewS3(
		s3ClientConfig.Endpoint,
		s3ClientConfig.Region,
		s3ClientConfig.AccessKey.Get(),
		s3ClientConfig.SecretKey.Get(),
	)
	if err != nil {
		return fmt.Errorf("cannot create S3 client for backend '%s': %v", firstBackend.S3ClientID, err)
	}

	// Create crypto instance if specified
	var cryptoInstance crypto.Crypt
	if firstBackend.CryptoID != "" {
		cryptoInstance, err = crypto.NewCryptFromConfig(cfg, firstBackend.CryptoID)
		if err != nil {
			return fmt.Errorf("failed to create crypto instance for ID %s: %v", firstBackend.CryptoID, err)
		}
		log.Printf("Encryption enabled using crypto ID: %s", firstBackend.CryptoID)
	} else {
		log.Printf("No encryption configured for this backend")
	}

	// Check if mount point exists and is a directory
	if info, err := os.Stat(*mountPoint); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("mount point %s is not a directory", *mountPoint)
		}
		// Check if directory is empty
		entries, err := os.ReadDir(*mountPoint)
		if err != nil {
			return fmt.Errorf("failed to read mount point directory: %v", err)
		}
		if len(entries) > 0 {
			log.Printf("Warning: Mount point %s is not empty. Mounting may hide existing files.", *mountPoint)
		}
	} else if os.IsNotExist(err) {
		if err := os.MkdirAll(*mountPoint, 0755); err != nil {
			return fmt.Errorf("failed to create mount point directory %s: %v", *mountPoint, err)
		}
		log.Printf("Created mount point directory: %s", *mountPoint)
	} else {
		return fmt.Errorf("failed to check mount point %s: %v", *mountPoint, err)
	}

	// Create S3FS instance using the actual backend bucket name
	s3fs := fusefs.NewS3FS(s3Client, firstBackend.S3BucketName, cryptoInstance)

	ctx := context.Background()
	if err := s3fs.ValidateBucket(ctx); err != nil {
		return fmt.Errorf("bucket validation failed for backend bucket '%s': %v", firstBackend.S3BucketName, err)
	}

	log.Printf("Successfully validated access to backend bucket: %s (logical bucket: %s)", firstBackend.S3BucketName, *bucket)

	// Mount the filesystem
	mountOptions := []fuse.MountOption{
		fuse.FSName("s3fs"),
		fuse.Subtype("s3fs"),
	}

	// Add read-only option if specified
	if *readOnly {
		mountOptions = append(mountOptions, fuse.ReadOnly())
		log.Printf("Mounting in read-only mode")
	} else {
		log.Printf("Mounting with read/write support")
		log.Printf("Note: Write operations upload complete files to S3 on flush/close")
	}

	// Log encryption status
	if cryptoInstance != nil {
		log.Printf("Encryption: ENABLED - Data will be encrypted before upload and decrypted after download")
	} else {
		log.Printf("Encryption: DISABLED - Data will be stored in plaintext")
	}

	c, err := fuse.Mount(*mountPoint, mountOptions...)
	if err != nil {
		return fmt.Errorf("cannot mount file system: %v", err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			log.Printf("Error closing FUSE connection: %v", err)
		}
	}()

	log.Printf("Successfully mounted logical bucket '%s' (backend: %s) at '%s'", *bucket, firstBackend.S3BucketName, *mountPoint)

	// Provide platform-specific unmount instructions
	var unmountCmd string
	switch runtime.GOOS {
	case "linux":
		unmountCmd = fmt.Sprintf("fusermount -u %s", *mountPoint)
	case "darwin":
		unmountCmd = fmt.Sprintf("umount %s", *mountPoint)
	default:
		unmountCmd = fmt.Sprintf("umount %s", *mountPoint)
	}
	log.Printf("To unmount, run: %s", unmountCmd)

	// Serve the filesystem
	if *debug {
		log.Printf("Debug mode enabled.")
	}

	log.Printf("Filesystem is ready. Press Ctrl+C to unmount.")

	// Print feature summary
	if *readOnly {
		log.Printf("Features: Read-only access, directory listing, metadata caching")
	} else {
		log.Printf("Features: Read/write access, create/delete files/directories, rename operations, metadata caching")
		log.Printf("Write limitations: No partial file updates (full file uploads), no atomic operations")
	}

	// Print backend information
	log.Printf("Backend Information:")
	log.Printf("  - Logical bucket: %s", *bucket)
	log.Printf("  - Physical bucket: %s", firstBackend.S3BucketName)
	log.Printf("  - S3 endpoint: %s", s3ClientConfig.Endpoint)
	log.Printf("  - Encryption: %s", firstBackend.CryptoID)
	log.Printf("  - Total backends in config: %d (S3FS uses first backend only)", len(bucketConfig.Backends))

	err = fs.Serve(c, s3fs)
	if err != nil {
		return fmt.Errorf("failed to serve file system: %v", err)
	}

	return nil
}

// Helper function to get bucket names for error messages
func getBucketNames(cfg *config.Config) []string {
	names := make([]string, len(cfg.S3Buckets))
	for i, bucket := range cfg.S3Buckets {
		names[i] = bucket.BucketName
	}
	return names
}
