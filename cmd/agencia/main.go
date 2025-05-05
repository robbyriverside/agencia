package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/robbyriverside/agencia"
	"github.com/robbyriverside/agencia/logs"

	"github.com/jessevdk/go-flags"
	"github.com/joho/godotenv"
)

var Version string = "unknown"

func main() {
	parser := flags.NewParser(agencia.GetOptions(), flags.Default)
	parser.AddCommand("run", "Run an agent", "Execute a named agent with input", &RunCommand{})
	parser.AddCommand("server", "Run the Agencia server", "Run the Agencia server", &ServerCommand{})
	parser.AddCommand("version", "Show the version", "Display Agencia version", &VersionCommand{})
	if _, err := parser.Parse(); err != nil {
		log.Fatalf("[FATAL] CLI error: %v", err)
	}
}

type ServerCommand struct {
	Addr string `short:"a" long:"addr" description:"Address to bind the server to" default:"0.0.0.0:8080"`
}

func (s *ServerCommand) Execute(args []string) error {
	_ = godotenv.Load()
	logs.InitLogger(os.Getenv("ENV"))
	log.Printf("Agencia server (version %s) running on %s", Version, s.Addr)
	ctx := context.Background()
	agencia.Server(ctx, s.Addr)
	return nil
}

type RunCommand struct {
	Name  string `short:"n" long:"name" required:"true" description:"Agent name to run"`
	Input string `short:"i" long:"input" required:"true" description:"Input string"`
	File  string `short:"f" long:"file" default:"agentic.yaml" description:"Agent definition YAML file"`
}

func (r *RunCommand) Execute(args []string) error {
	_ = godotenv.Load()
	logs.InitLogger(os.Getenv("ENV"))
	ctx := context.Background()
	registry, err := agencia.LoadRegistry(r.File)
	if err != nil {
		logs.Error(err)
		return errors.New("run command failed")
	}
	if err := registry.RunPrint(ctx, r.Name, r.Input); err != nil {
		logs.Error(err)
		return errors.New("run command failed")
	}
	return nil
}

type VersionCommand struct{}

func (v *VersionCommand) Execute(args []string) error {
	fmt.Printf("Agencia version: %s\n", Version)
	return nil
}
