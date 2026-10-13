package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"

	"ynab-mcp/internal/hosted"
	"ynab-mcp/internal/mcp"
	"ynab-mcp/internal/tools"
	"ynab-mcp/internal/ynab"
)

func main() {
	if err := loadEnv(); err != nil {
		log.Fatal(err)
	}
	if os.Getenv("MCP_TRANSPORT") == "http" {
		s, err := hosted.New(hosted.Config{
			BaseURL: os.Getenv("PUBLIC_BASE_URL"), YNABClientID: os.Getenv("YNAB_OAUTH_CLIENT_ID"),
			YNABSecret: os.Getenv("YNAB_OAUTH_CLIENT_SECRET"), DBPath: os.Getenv("OAUTH_DB_PATH"), Key: os.Getenv("TOKEN_ENCRYPTION_KEY"),
		})
		if err != nil {
			log.Fatal(err)
		}
		defer s.Close()
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		server := &http.Server{Addr: ":" + port, Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
		log.Printf("hosted MCP listening on port %s", port)
		log.Fatal(server.ListenAndServe())
		return
	}
	client := ynab.NewClient(os.Getenv("YNAB_ACCESS_TOKEN"), nil, "")
	server := mcp.NewServer(tools.New(client))
	if err := server.Serve(os.Stdin, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func loadEnv() error {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("could not load .env; check its permissions and syntax")
	}
	return nil
}
