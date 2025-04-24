package rag

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/qdrant/go-client/qdrant"
	"github.com/robbyriverside/agencia/agents"
	"github.com/sashabaranov/go-openai"
)

var RAGAgents = map[string]agents.Agent{
	"search": {
		Description: "Search the knowledge base for relevant passages.",
		InputPrompt: map[string]string{
			"query": "What are you looking for?",
			"limit": "Max number of results to return.",
		},
		Function: Search,
	},

	"answer_with_sources": {
		Description: "Return an answer with cited source excerpts.",
		InputPrompt: map[string]string{
			"question": "What do you want to know?",
		},
		Function: AnswerWithSources,
	},

	"extract_facts": {
		Description: "Return bullet-pointed facts from the most relevant documents.",
		InputPrompt: map[string]string{
			"query": "What information are you trying to gather?",
		},
		Function: ExtractFacts,
	},
}

// Assumes QDrant client and embedder are set up globally
var (
	qdrantClient *qdrant.Client
	openaiClient *openai.Client
)

func init() {

	client, err := qdrant.NewClient(&qdrant.Config{
		Host: "localhost",
		Port: 6334,
	})
	if err != nil {
		panic("failed to connect to Qdrant: " + err.Error())
	}
	qdrantClient = client
}

func createCollection(name string) {
	qdrantClient.CreateCollection(context.Background(), &qdrant.CreateCollection{
		CollectionName: name,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     4,
			Distance: qdrant.Distance_Cosine,
		}),
	})
}

func upsertVectors(name string, vec []float32) (*qdrant.UpdateResult, error) {

	operationInfo, err := qdrantClient.Upsert(context.Background(), &qdrant.UpsertPoints{
		CollectionName: name,
		Points: []*qdrant.PointStruct{
			{
				Id:      qdrant.NewIDNum(1),
				Vectors: qdrant.NewVectors(0.05, 0.61, 0.76, 0.74),
				Payload: qdrant.NewValueMap(map[string]any{"city": "Berlin"}),
			},
			{
				Id:      qdrant.NewIDNum(2),
				Vectors: qdrant.NewVectors(0.19, 0.81, 0.75, 0.11),
				Payload: qdrant.NewValueMap(map[string]any{"city": "London"}),
			},
			{
				Id:      qdrant.NewIDNum(3),
				Vectors: qdrant.NewVectors(0.36, 0.55, 0.47, 0.94),
				Payload: qdrant.NewValueMap(map[string]any{"city": "Moscow"}),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return operationInfo, nil
}

func createFullIndex(collection string) (*qdrant.UpdateResult, error) {

	result, err := qdrantClient.CreateFieldIndex(context.Background(), &qdrant.CreateFieldIndexCollection{
		CollectionName: collection,
		FieldName:      "text",
		FieldType:      qdrant.FieldType_FieldTypeText.Enum(),
		FieldIndexParams: qdrant.NewPayloadIndexParamsText(
			&qdrant.TextIndexParams{
				Tokenizer:   qdrant.TokenizerType_Whitespace,
				MinTokenLen: qdrant.PtrOf(uint64(2)),
				MaxTokenLen: qdrant.PtrOf(uint64(10)),
				Lowercase:   qdrant.PtrOf(true),
			}),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create index: %v", err)
	}
	return result, nil
}

func createIndex(collection, field string) (*qdrant.UpdateResult, error) {

	results, err := qdrantClient.CreateFieldIndex(context.Background(), &qdrant.CreateFieldIndexCollection{
		CollectionName: collection,
		FieldName:      field,
		FieldType:      qdrant.FieldType_FieldTypeKeyword.Enum(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create index: %v", err)
	}
	return results, nil
}

func filterQuery(name string, vec []float32, filter map[string]string) ([]*qdrant.ScoredPoint, error) {
	searchResult, err := qdrantClient.Query(context.Background(), &qdrant.QueryPoints{
		CollectionName: name,
		Query:          qdrant.NewQuery(vec...),
		Filter: &qdrant.Filter{
			Must: []*qdrant.Condition{
				qdrant.NewMatch("city", filter["city"]),
			},
		},
		WithPayload: qdrant.NewWithPayload(true),
	})
	if err != nil {
		panic("failed to query: " + err.Error())
	}
	return searchResult, nil
}

func query(name string, vec []float32, limit uint64) ([]*qdrant.ScoredPoint, error) {
	max := new(uint64)
	*max = limit
	searchResult, err := qdrantClient.Query(context.Background(), &qdrant.QueryPoints{
		CollectionName: name,
		Query:          qdrant.NewQuery(vec...),
		Limit:          max,
	})
	if err != nil {
		panic("failed to query: " + err.Error())
	}
	return searchResult, nil
}

func embedText(ctx context.Context, text string) ([]float32, error) {
	embedding, err := openaiClient.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Input: []string{text},
		Model: openai.AdaEmbeddingV2,
	})
	if err != nil {
		return nil, err
	}
	// Convert []float64 to []float32
	vec := embedding.Data[0].Embedding
	result := make([]float32, len(vec))
	for i, v := range vec {
		result[i] = float32(v)
	}
	return result, nil
}

func Search(ctx context.Context, input map[string]any) (string, error) {
	queryArg, _ := input["query"].(string)
	limitStr, _ := input["limit"].(string)
	limit, _ := strconv.Atoi(limitStr)
	if limit == 0 {
		limit = 5
	}

	vec, err := embedText(ctx, queryArg)
	if err != nil {
		return "", err
	}

	resp, err := query(os.Getenv("QDRANT_COLLECTION"), vec, uint64(limit))
	if err != nil {
		return "", err
	}

	results := []string{}
	for _, point := range resp {
		if payload := point.Payload; payload != nil {
			if val, ok := payload["text"]; ok {
				results = append(results, val.GetStringValue())
			}
		}
	}
	return strings.Join(results, "\n---\n"), nil
}

func AnswerWithSources(ctx context.Context, input map[string]any) (string, error) {
	input["limit"] = "5"
	sources, err := Search(ctx, input)
	if err != nil {
		return "", err
	}
	question := input["question"].(string)
	prompt := fmt.Sprintf("Use the following sources to answer this question:\n\n%s\n\nQuestion: %s", sources, question)
	return agents.CallOpenAI(ctx, prompt)
}

func ExtractFacts(ctx context.Context, input map[string]any) (string, error) {
	input["limit"] = "5"
	sources, err := Search(ctx, input)
	if err != nil {
		return "", err
	}
	prompt := fmt.Sprintf("Extract the most important facts from the following:\n\n%s", sources)
	return agents.CallOpenAI(ctx, prompt)
}
