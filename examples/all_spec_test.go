package examples

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/robbyriverside/agencia"
	"github.com/robbyriverside/agencia/agents"
)

// loadAllSpecs parses a multi-doc YAML file and returns a slice of AgentSpec
func loadAllSpecs(filename string) ([]agencia.AgentSpec, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("cannot read file: %w", err)
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	var specs []agencia.AgentSpec
	for {
		var spec agencia.AgentSpec
		err := dec.Decode(&spec)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, fmt.Errorf("decode error: %w", err)
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

// captureSpecOutput is a reusable function that registers agents and runs each agent with a sample input
func captureSpecOutput(ctx context.Context, spec agencia.AgentSpec, input string) map[string]string {
	outputs := make(map[string]string)
	registry, err := agencia.RegisterAgents(spec)
	if err != nil {
		return nil
	}
	for name := range spec.Agents {
		res := registry.CallAgent(ctx, name, input)
		if res.Error != nil {
			outputs[name] = fmt.Sprintf("[ERROR] %v", res.Error)
		} else {
			outputs[name] = res.Output
		}
	}
	return outputs
}

func WriteSpecOutput(filename, outputFilename string) error {
	ctx := context.Background()
	specs, err := loadAllSpecs(filename)
	if err != nil {
		return fmt.Errorf("[LOAD ERROR] %w", err)
	}
	allOutputs := make(map[string]string)
	for _, spec := range specs {
		outputs := captureSpecOutput(ctx, spec, "World")
		for name, output := range outputs {
			allOutputs[name] = output
		}
	}
	yamlBytes, err := yaml.Marshal(allOutputs)
	if err != nil {
		return fmt.Errorf("failed to marshal outputs to YAML: %w", err)
	}
	return os.WriteFile(outputFilename, yamlBytes, 0644)
}

// TEST: Compare generated output to golden file, or generate if missing
func TestAllSpecsOutput(t *testing.T) {
	inputFile := "all.yaml"
	outputFile := "all_output.yaml"
	mockFile := "all_mock.yaml"

	ctx := context.Background()
	if err := agents.ConfigureAI(ctx, mockFile); err != nil {
		t.Fatalf("Failed to configure Agencia AI with mock: %v", err)
	}

	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		err := WriteSpecOutput(inputFile, outputFile)
		if err != nil {
			t.Fatalf("Failed to generate output file: %v", err)
		}
		t.Logf("Output file generated: %s", outputFile)
		return
	}

	expectedData, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read expected output file: %v", err)
	}
	var expected map[string]string
	if err := yaml.Unmarshal(expectedData, &expected); err != nil {
		t.Fatalf("Failed to unmarshal expected YAML: %v", err)
	}

	specs, err := loadAllSpecs(inputFile)
	if err != nil {
		t.Fatalf("Failed to load all specs: %v", err)
	}
	actual := make(map[string]string)
	for _, spec := range specs {
		outputs := captureSpecOutput(ctx, spec, "World")
		for k, v := range outputs {
			actual[k] = v
		}
	}

	if !reflect.DeepEqual(actual, expected) {
		actualYAML, _ := yaml.Marshal(actual)
		expectedYAML, _ := yaml.Marshal(expected)
		t.Errorf("Spec output does not match expected output.\n\nExpected:\n%s\n\nActual:\n%s", expectedYAML, actualYAML)
	}
}
