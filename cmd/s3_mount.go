package cmd

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
)

// S3Mount mounts an S3 bucket as a local file system using s3fs.
func S3Mount() error {
	fs := flag.NewFlagSet("s3-mount", flag.ExitOnError)
	bucket := fs.String("bucket", "", "S3 bucket name")
	mountPoint := fs.String("mount-point", "", "Local directory to mount the bucket")
	accessKey := fs.String("access-key", "", "AWS access key")
	secretKey := fs.String("secret-key", "", "AWS secret key")
	endpoint := fs.String("endpoint", "http://localhost:8080", "S3 proxy endpoint")

	if err := fs.Parse(os.Args[2:]); err != nil {
		return fmt.Errorf("failed to parse flags: %v", err)
	}

	// Validate required arguments
	if *bucket == "" || *mountPoint == "" || *accessKey == "" || *secretKey == "" {
		return fmt.Errorf("missing required arguments: bucket, mount-point, access-key, and secret-key are required")
	}

	// Check if s3fs is installed
	if _, err := exec.LookPath("s3fs"); err != nil {
		return fmt.Errorf("s3fs is not installed. Please install s3fs-fuse: https://github.com/s3fs-fuse/s3fs-fuse")
	}

	// Check if mount point is valid
	if info, err := os.Stat(*mountPoint); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("mount point %s is not a directory", *mountPoint)
		}
	} else if os.IsNotExist(err) {
		if err := os.MkdirAll(*mountPoint, 0755); err != nil {
			return fmt.Errorf("failed to create mount point directory %s: %v", *mountPoint, err)
		}
	} else {
		return fmt.Errorf("failed to check mount point %s: %v", *mountPoint, err)
	}

	// Create temporary password file
	passFile, err := os.CreateTemp("", "s3fs-passwd-")
	if err != nil {
		return fmt.Errorf("failed to create temporary password file: %v", err)
	}
	passFilePath := passFile.Name()
	defer os.Remove(passFilePath) // Clean up after use

	passContent := fmt.Sprintf("%s:%s", *accessKey, *secretKey)
	if err := os.WriteFile(passFilePath, []byte(passContent), 0600); err != nil {
		return fmt.Errorf("failed to write password file: %v", err)
	}

	// Construct s3fs command
	cmd := exec.Command("s3fs",
		*bucket,
		*mountPoint,
		"-o", "passwd_file="+passFilePath,
		"-o", "url="+*endpoint,
		"-o", "use_path_request_style",
		"-f",
	)
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = &stderr

	// Start s3fs
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to mount bucket %s to %s: %v, stderr: %s", *bucket, *mountPoint, err, stderr.String())
	}

	fmt.Printf("Mounted bucket %s successfully at %s. To unmount, run 'fusermount -u %s'\n", *bucket, *mountPoint, *mountPoint)
	log.Printf("Started s3fs process for bucket %s at %s", *bucket, *mountPoint)
	return nil
}