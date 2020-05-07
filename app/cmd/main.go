package main

import (
	"log"
	"os"

	"app"

	"github.com/GoogleCloudPlatform/functions-framework-go/funcframework"
)

func main() {
	funcframework.RegisterHTTPFunction("/challenge", app.ChallengeHandler)
	funcframework.RegisterHTTPFunction("/report", app.ReportHandler)
	// Use PORT environment variable, or default to 8088.
	port := "8088"
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
	}

	if err := funcframework.Start(port); err != nil {
		log.Fatalf("funcframework.Start: %v\n", err)
	}
}
