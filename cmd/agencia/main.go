package main

import (
	"context"
	"fmt"
	"log"

	"github.com/robbyriverside/agencia"
	"github.com/robbyriverside/agencia/agents"

	"github.com/jessevdk/go-flags"
	"github.com/joho/godotenv"
)

func main() {
	parser := flags.NewParser(nil, flags.Default)
	parser.AddCommand("run", "Run an agent", "Execute a named agent with input", &RunCommand{})
	parser.AddCommand("server", "Run the Agencia server", "Run the Agencia server", &ServerCommand{})
	if _, err := parser.Parse(); err != nil {
		log.Fatalf("[FATAL] CLI error: %v", err)
	}
}

type ServerCommand struct{}

func (s *ServerCommand) Execute(args []string) error {
	_ = godotenv.Load()
	ctx := context.Background()
	agencia.Server(ctx, ":8080")
	return nil
}

type RunCommand struct {
	Name  string `short:"n" long:"name" required:"true" description:"Agent name to run"`
	Input string `short:"i" long:"input" required:"true" description:"Input string"`
	File  string `short:"f" long:"file" default:"agentic.yaml" description:"Agent definition YAML file"`
	Mock  string `short:"m" long:"mock" description:"Path to mock response YAML file"`
}

func (r *RunCommand) Execute(args []string) error {
	_ = godotenv.Load()
	ctx := context.Background()
	agents.ConfigureAI(ctx, r.Mock)
	registry, err := agents.CompileFile(r.File)
	if err != nil {
		return fmt.Errorf("[LOAD ERROR] %w", err)
	}
	return registry.RunPrint(ctx, r.Name, r.Input)
}
