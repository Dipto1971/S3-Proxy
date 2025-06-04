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
	endpoint := flagSet.String("endpoint", "http://localhost:9000", "S3 endpoint URL")
	accessKey := flagSet.String("access-key", "", "S3 access key")
	secretKey := flagSet.String("secret-key", "", "S3 secret key")
	region := flagSet.String("region", "us-east-1", "S3 region")
	bucket := flagSet.String("bucket", "test-bucket", "S3 bucket name")
	debug := flagSet.Bool("debug", false, "Enable debug logging")
	readOnly := flagSet.Bool("read-only", false, "Mount in read-only mode")
	configPath := flagSet.String("config", "configs/main.yaml", "Path to configuration file")

	if err := flagSet.Parse(os.Args[1:]); err != nil {
		return fmt.Errorf("failed to parse flags: %v", err)
	}

	if *mountPoint == "" {
		return fmt.Errorf("mount point is required (use -mount flag)")
	}

	if *bucket == "" {
		return fmt.Errorf("bucket name is required (use -bucket flag)")
	}

	if *accessKey == "" || *secretKey == "" {
		log.Printf("Warning: No access key or secret key provided. Will use default credentials or environment variables.")
	}

	// Load configuration for authentication and encryption
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Printf("Warning: Failed to load config from %s: %v. Proceeding without encryption and authentication.", *configPath, err)
		cfg = nil
	}

	// Validate access key if configuration is available and access key is provided
	var cryptoInstance crypto.Crypt
	if cfg != nil && *accessKey != "" {
		if !cfg.Auth.IsValidAccessKey(*accessKey) {
			return fmt.Errorf("invalid access key: %s", *accessKey)
		}
		log.Printf("Access key validated successfully")

		// Get crypto configuration for the bucket
		cryptoID := config.GetCryptoIDForBucket(cfg, *bucket)
		if cryptoID != "" {
			cryptoInstance, err = crypto.NewCryptFromConfig(cfg, cryptoID)
			if err != nil {
				return fmt.Errorf("failed to create crypto instance for ID %s: %v", cryptoID, err)
			}
			log.Printf("Encryption enabled using crypto ID: %s", cryptoID)
		} else {
			log.Printf("No encryption configured for bucket: %s", *bucket)
		}
	} else {
		log.Printf("Proceeding without authentication validation and encryption")
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

	// Create S3 client
	s3Client, err := client.NewS3(*endpoint, *region, *accessKey, *secretKey)
	if err != nil {
		return fmt.Errorf("cannot create S3 client: %v", err)
	}

	// Create S3FS instance with encryption support
	s3fs := fusefs.NewS3FS(s3Client, *bucket, cryptoInstance)

	ctx := context.Background()
	if err := s3fs.ValidateBucket(ctx); err != nil {
		return fmt.Errorf("bucket validation failed: %v", err)
	}

	log.Printf("Successfully validated access to bucket: %s", *bucket)

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

	log.Printf("Successfully mounted S3 bucket '%s' at '%s'", *bucket, *mountPoint)

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

	err = fs.Serve(c, s3fs)
	if err != nil {
		return fmt.Errorf("failed to serve file system: %v", err)
	}

	return nil
}
