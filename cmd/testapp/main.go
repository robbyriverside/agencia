package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"

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

	// Prepare the input prompt
	input := "Write a hello world in Go"
	if len(os.Args) > 1 {
		input = os.Args[1]
	}

	// Run the agent
	fmt.Printf("Running coder agent with request: %s\n", input)
	out, _ := registry.Run(ctx, nil, "coder", input)

	// Print the output
	fmt.Printf("Output:\n%s\n", out)
}
