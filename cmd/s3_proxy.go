// cmd/s3_proxy.go
package cmd

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"s3-proxy/internal/api"
	"s3-proxy/internal/config"
)

func S3Proxy() error {
	cfgPath := flag.String("config", "configs/main.yaml", "path to yaml config")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return fmt.Errorf("cannot load config: %w", err)
	}

	prx, err := api.New(cfg)
	if err != nil {
		return fmt.Errorf("cannot create proxy: %w", err)
	}

	log.Printf("listening on %s", cfg.ListenAddr)
	return http.ListenAndServe(cfg.ListenAddr, prx)
}
