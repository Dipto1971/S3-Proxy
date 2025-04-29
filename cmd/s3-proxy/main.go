// cmd/s3-proxy/main.go
package main

import (
	"os"
	"s3-proxy/cmd"
)

func main() {
	subcmds := map[string]func() error{
		"s3-proxy":    cmd.S3Proxy,
		"s3-write":    cmd.S3Write,
		"s3-read":     cmd.S3Read,
		"tink-keyset": cmd.GenerateTinkKeyset,
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
