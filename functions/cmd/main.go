package main

import (
	"fmt"
	"log"
	"os"

	"github.com/GoogleCloudPlatform/functions-framework-go/funcframework"

	functions "upload-token.functions"
	"upload-token.functions/internal/util"
)

func main() {
	funcframework.RegisterHTTPFunction("/challenge", util.MakeTestHTTPHandler(functions.ChallengeHandler))
	// Use PORT environment variable, or default to 8080.
	port := "8080"
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
	}

	fmt.Println("Listening port:", port)
	if err := funcframework.Start(port); err != nil {
		log.Fatalf("funcframework.Start: %v\n", err)
	}
}
