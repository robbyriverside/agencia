package agencia

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestHelplineProto(t *testing.T) {
	requireAPI(t)

	raw, err := os.ReadFile("helpline_spec.yaml")
	require.NoError(t, err)
	spec := string(raw)

	data, err := ioutil.ReadFile("helpline_data.yaml")
	if err != nil {
		log.Fatalf("Failed to read file: %v", err)
	}

	var root Root
	err = yaml.Unmarshal(data, &root)
	if err != nil {
		log.Fatalf("Failed to unmarshal YAML: %v", err)
	}

	tests := root.Scripts[1].Dialog
	// Load the spec

	// Chat starts with 'greeter'
	if defaultChat == nil {
		defaultChat = NewChat("mainmenu")
	}
	reg, err := defaultChat.NewRegistry(spec)
	require.NoError(t, err)

	for i, test := range tests {
		// First call should run greeter and change chat.Start
		out1, trace := reg.Run(context.Background(), defaultChat.StartAgent, test.Input)
		// if trace.Error != nil  {
		trace.SaveMarkdown(fmt.Sprintf("trace%d.md", i))
		facts := defaultChat.Facts["mainmenu.information"]
		if facts != nil {
			t.Logf("*** Facts: %s", facts)
		}
		// }
		t.Logf("Input: %s", test.Input)
		t.Logf("Result: %s", out1)
		t.Logf("\nCompare: %s\n", test.Output)
	}
}

func TestHelplinePhase1(t *testing.T) {
	requireAPI(t)

	spec := `---
agents:

  resources:
    inputs:
      type:
        description: |
          The type of the resources needed.
    description: |
      You are a resource agent. Your job is to provide resources to the user.
    template: |
      I have the resources you need using SKYBIRD.


  information:
    inputs:
      source:
        description: |
          The source of the information.
    description: |
      You are an information agent. Your job is to provide information to the user.
    template: |
      I have the information you need via direct connection.

  helpline:
    description: |
      determine what the user needs.
    prompt: |
      You are a helpline agent. Your job is to determine what the user needs.
      You will receive a message from the user. Your job is to determine if the user needs resources or information.
      If the user needs resources, you will send a message to the resource agent.
      If the user needs information, you will send a message to the information agent.
      {{ .Input }}

    listeners:
      - resources
      - information
`

	// helpline_test.go:51: Input: I need help with my mental health.
	// helpline_test.go:52: Result: I have the resources you need for mental health support. If you need further assistance, feel free to ask!
	// helpline_test.go:53: Compare: I'm here to help you. Can you tell me more about what you're experiencing?
	// helpline_test.go:51: Input: When is my next appointment with the doctor? I need a driver to take me there.
	// helpline_test.go:52: Result: I have both the information and resources you need. You can check your appointment schedule for the details of your next doctor's appointment, and I have arranged for transportation to take you there.
	// helpline_test.go:53: Compare: I have the information you need.
	// helpline_test.go:51: Input: I need a driver to take me to my doctor's appointment.
	// helpline_test.go:52: Result: I have arranged the transportation resources you need for your doctor's appointment.
	// helpline_test.go:53: Compare: I have the resources you need.

	tests := []struct {
		input  string
		output string
	}{
		{
			input:  "I need help with my mental health.",
			output: "using SKYBIRD",
		},
		{
			input:  "When is my next appointment with the doctor? I need a driver to take me there.",
			output: "next doctor's appointment",
		},
		{
			input:  "I need a driver to take me to my doctor's appointment.",
			output: "arranged a driver",
		},
	}
	// Load the spec

	// Chat starts with 'greeter'
	if defaultChat == nil {
		defaultChat = NewChat("helpline")
	}
	reg, err := defaultChat.NewRegistry(spec)
	require.NoError(t, err)

	for i, test := range tests {
		// First call should run greeter and change chat.Start
		out1, trace := reg.Run(context.Background(), defaultChat.StartAgent, test.input)
		if trace.Error != nil {
			trace.SaveMarkdown(fmt.Sprintf("trace%d.md", i))
		}
		//assert.Contains(t, out1, test.output)
		t.Logf("Input: %s", test.input)
		t.Logf("Result: %s", out1)
		t.Logf("Compare: %s\n", test.output)
	}
}
