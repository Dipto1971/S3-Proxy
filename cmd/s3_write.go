// cmd/s3_write.go
package cmd

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"s3-proxy/internal/client"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func S3Write() error {
	endpoint := flag.String("endpoint", "http://localhost:8080", "S3 endpoint")
	accessKey := flag.String("access-key", "", "S3 access key")
	secretKey := flag.String("secret-key", "", "S3 secret key")
	region := flag.String("region", "", "S3 region")
	bucket := flag.String("bucket", "test-bucket", "S3 bucket name")
	filePath := flag.String("file-path", "", "File to upload")
	key := flag.String("key", "", "S3 object key")
	flag.Parse()

	s3Client, err := client.NewS3(*endpoint, *region, *accessKey, *secretKey)
	if err != nil {
		return fmt.Errorf("cannot create s3 client: %v", err)
	}

	file, err := os.Open(*filePath)
	if err != nil {
		log.Fatalf("failed to open file %s: %v", *filePath, err)
	}
	defer file.Close()

	_, err = s3Client.Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: bucket,
		Key:    key,
		Body:   file,
	})
	if err != nil {
		return fmt.Errorf("failed to put object %s from bucket %s: %v", *key, *bucket, err)
	}

	log.Printf("successfully uploaded %s to bucket %s at %s", *key, *bucket, time.Now().Format(time.RFC3339))
	return nil
}
