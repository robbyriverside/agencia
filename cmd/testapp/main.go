package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"

	"github.com/joho/godotenv"
	"github.com/robbyriverside/agencia"
)

//go:embed agent.yaml
var spec string

func main() {
	_ = godotenv.Load()
	ctx := context.Background()

	// Initialize the registry with the spec
	registry, err := agencia.NewRegistry(spec)
	if err != nil {
		log.Fatalf("Failed to create registry: %v", err)
	}

	// Run the agent
	fmt.Println("Running coder agent with request: Write a hello world in Go")
	out, _ := registry.Run(ctx, nil, "coder", "Write a hello world in Go")

	// Print the output
	fmt.Printf("Output:\n%s\n", out)
}
