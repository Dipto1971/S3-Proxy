// cmd/s3_read.go
package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"s3-proxy/internal/client"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func S3Read() error {
	endpoint := flag.String("endpoint", "http://localhost:8080", "S3 endpoint")
	accessKey := flag.String("access-key", "", "S3 access key")
	secretKey := flag.String("secret-key", "", "S3 secret key")
	region := flag.String("region", "", "S3 region")
	bucket := flag.String("bucket", "test-bucket", "S3 bucket name")
	key := flag.String("key", "", "S3 object key")
	flag.Parse()

	s3Client, err := client.NewS3(*endpoint, *region, *accessKey, *secretKey)
	if err != nil {
		return fmt.Errorf("cannot create s3 client: %v", err)
	}

	robj, err := s3Client.Client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: bucket,
		Key:    key,
	})
	if err != nil {
		return fmt.Errorf("failed to get object %s from bucket %s: %v", *key, *bucket, err)
	}

	contents, err := io.ReadAll(robj.Body)
	if err != nil {
		return fmt.Errorf("failed to read object %s from bucket %s: %v", *key, *bucket, err)
	}

	fmt.Println(string(contents))
	return nil
}
