// cmd/s3-proxy/main.go
package main

import (
	"log"
	"os"
	"s3-proxy/cmd"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  No .env file found or failed to load")
	}

	subcmds := map[string]func() error{
		"s3-proxy":        cmd.S3Proxy,
		"s3-write":        cmd.S3Write,
		"s3-read":         cmd.S3Read,
		"cription-keyset": cmd.GenerateKeyset,
	}

	for indx, arg := range os.Args {
		subcmd := subcmds[arg]
		if subcmd != nil {
			os.Args = os.Args[indx:]
			if err := subcmd(); err != nil {
				panic(err)
			}
			return
		}
	}
}
