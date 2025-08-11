package agencia

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/robbyriverside/agencia/agents"
)

// observationAICall allows tests to stub AI calls.
var observationAICall = agents.CallOpenAI

// gatherObservationsFromInput asks AI to synthesize observations from the input
// according to the role's observation descriptions.
func (r *RunContext) gatherObservationsFromInput(ctx context.Context, role *agents.AgentRole, input string) (map[string][]string, error) {
	if role == nil || len(role.Observations) == 0 {
		return nil, nil
	}
	var b strings.Builder
	b.WriteString("Input:\n" + input + "\n\n")
	b.WriteString("For each of the following observation keys, extract zero or more observations as short active sentences. Avoid using the exact wording from the input. Return JSON mapping keys to arrays of observations.\n\n")
	b.WriteString("Observations:\n")
	for k, v := range role.Observations {
		b.WriteString(fmt.Sprintf("%s: %s\n", k, v))
	}
	b.WriteString("\nRespond with JSON only.")

	prompt := b.String()
	resp, err := observationAICall(ctx, prompt)
	if err != nil {
		return nil, err
	}
	resp = deleteMDBlock(resp)
	obs := make(map[string][]string)
	if err := json.Unmarshal([]byte(resp), &obs); err != nil {
		return nil, fmt.Errorf("unmarshal observations JSON: %w. response: %s. prompt: %s", err, resp, prompt)
	}
	return obs, nil
}

// mergeObservations combines existing and new observations using AI to remove duplicates.
func (r *RunContext) mergeObservations(ctx context.Context, existing, incoming map[string][]string) (map[string][]string, error) {
	if len(incoming) == 0 {
		return existing, nil
	}
	existingJSON, _ := json.Marshal(existing)
	incomingJSON, _ := json.Marshal(incoming)

	prompt := fmt.Sprintf("Existing observations: %s\nNew observations: %s\nAdd the new observations to the existing ones without creating redundant meaning or duplicates. Return only the combined observations as JSON.", existingJSON, incomingJSON)
	resp, err := observationAICall(ctx, prompt)
	if err != nil {
		return nil, err
	}
	resp = deleteMDBlock(resp)
	merged := make(map[string][]string)
	if err := json.Unmarshal([]byte(resp), &merged); err != nil {
		return nil, fmt.Errorf("unmarshal merged observations JSON: %w. response: %s. prompt: %s", err, resp, prompt)
	}
	return merged, nil
}

// deleteMDBlock removes the last JSON or YAML markdown block from a string, used to clean up AI responses.
func deleteMDBlock(resp string) string {
	if idx := strings.LastIndex(resp, "```json"); idx != -1 {
		resp = resp[idx+7:]
		if idx := strings.LastIndex(resp, "```"); idx != -1 {
			resp = resp[:idx]
		}
	} else if idx := strings.LastIndex(resp, "```yaml"); idx != -1 {
		resp = resp[idx+7:]
		if idx := strings.LastIndex(resp, "```"); idx != -1 {
			resp = resp[:idx]
		}
	}
	return resp
}

// buildObservationPrompt builds a sub-prompt string from observations.
func buildObservationPrompt(role string, obs map[string][]string) string {
	if len(obs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Use the following observations gathered for your role when responding.\n")
	for k, list := range obs {
		if len(list) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("%s:\n", k))
		for _, o := range list {
			b.WriteString("- " + o + "\n")
		}
	}
	return b.String()
}

// processObservations orchestrates gathering, merging, storing, and prompt creation.
func (r *RunContext) processObservations(ctx context.Context, agent *agents.Agent, input string) (string, error) {
	if r == nil || r.Registry == nil || r.Chat == nil || agent == nil || agent.Role == "" {
		return "", nil
	}
	role, ok := r.Registry.LookupRole(agent.Role)
	if !ok || len(role.Observations) == 0 {
		return "", nil
	}

	newObs, err := r.gatherObservationsFromInput(ctx, role, input)
	if err != nil {
		r.Errorf("gather observations error: %v", err)
		return "", nil
	}
	existing := r.Chat.ObservationsByRole(role.ID)
	merged, err := r.mergeObservations(ctx, existing, newObs)
	if err != nil {
		r.Errorf("merge observations error: %v", err)
		// fall back to naive merge
		merged = existing
		for k, list := range newObs {
			merged[k] = append(merged[k], list...)
		}
	}
	if r.Chat.Observations == nil {
		r.Chat.Observations = make(map[string]map[string][]string)
	}
	r.Chat.Observations[role.ID] = merged

	return buildObservationPrompt(role.ID, merged), nil
}
