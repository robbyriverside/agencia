package parley

import (
	"testing"
)

func TestValidator_BlockStructure(t *testing.T) {
	ctx := ValidationContext{
		Agents: map[string]AgentInfo{
			"agent": {Inputs: map[string]bool{"key": true}, Facts: nil},
		},
	}

	tests := []struct {
		name     string
		template string
		wantErr  bool
	}{
		{
			name:     "Valid IF block",
			template: `{{ IF INPUT key IS EMPTY THEN }} ... {{ END }}`,
			wantErr:  false,
		},
		{
			name:     "Valid IF/ELSE block",
			template: `{{ IF INPUT key IS EMPTY THEN }} ... {{ ELSE }} ... {{ END }}`,
			wantErr:  false,
		},
		{
			name: "Valid Nested IF block",
			template: `{{ IF INPUT key IS EMPTY THEN }} 
						{{ IF INPUT key IS foo THEN }} ... {{ END }}
					   {{ END }}`,
			wantErr: false,
		},
		{
			name:     "Invalid Unclosed IF",
			template: `{{ IF INPUT key IS EMPTY THEN }} ...`,
			wantErr:  true,
		},
		{
			name:     "Invalid Extra END",
			template: `{{ IF INPUT key IS EMPTY THEN }} ... {{ END }} {{ END }}`,
			wantErr:  true,
		},
		{
			name:     "Invalid ELSE outside IF",
			template: `{{ ELSE }} ... {{ END }}`,
			wantErr:  true,
		},
		{
			name:     "Invalid Mismatched IF/END",
			template: `{{ IF INPUT key IS EMPTY THEN }} ...`, // Missing END
			wantErr:  true,
		},
		{
			name:     "Valid Statement Form IF (No END required)",
			template: `{{ IF INPUT key IS EMPTY THEN "yes" ELSE "no" }}`,
			wantErr:  false,
		},
		{
			name:     "Valid SEND Block",
			template: `{{ SEND agent MESSAGE }} ... {{ END }}`,
			wantErr:  false,
		},
		{
			name:     "Valid LET Block",
			template: `{{ LET var BE }} ... {{ END }}`,
			wantErr:  false,
		},
		{
			name:     "Valid SEND Statement",
			template: `{{ SEND agent MESSAGE "hello" }}`,
			wantErr:  false,
		},
		{
			name:     "Valid LET Statement",
			template: `{{ LET var BE "value" }}`,
			wantErr:  false,
		},
		{
			name:     "Invalid Mixed Blocks",
			template: `{{ IF INPUT key IS val THEN }} {{ END }} {{ END }}`, // Extra END
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator(ctx)
			errors := validator.Validate(tt.template)
			if (len(errors) > 0) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", errors, tt.wantErr)
			}
			if len(errors) > 0 && tt.wantErr {
				// Optional: Check if error message relates to blocks if possible
				// t.Logf("Got expected errors: %v", errors)
			}
		})
	}
}
