//go:build windows

// cmd/s3fs_mount_windows.go

package cmd

import (
	"fmt"
)

// S3FSMount returns an error on Windows since FUSE is not supported
func S3FSMount() error {
	return fmt.Errorf("S3FS mounting is not supported on Windows. Please use WSL2 with Ubuntu or a Linux environment")
}
