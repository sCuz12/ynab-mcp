package main

import (
	"log"
	"os"

	"ynab-mcp/internal/mcp"
	"ynab-mcp/internal/tools"
	"ynab-mcp/internal/ynab"
)

func main() {
	client := ynab.NewClient(os.Getenv("YNAB_ACCESS_TOKEN"), nil, "")
	server := mcp.NewServer(tools.New(client))
	if err := server.Serve(os.Stdin, os.Stdout); err != nil {
		log.Fatal(err)
	}
}
