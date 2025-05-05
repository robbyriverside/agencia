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

var (
	openaiClient      *openai.Client
	openaiInitError   error
	openaiInitialized bool
)

func getOpenAIClient() (*openai.Client, error) {
	if openaiInitialized {
		return openaiClient, openaiInitError
	}
	openaiInitialized = true

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		openaiInitError = fmt.Errorf("OPENAI_API_KEY environment variable is not set")
		return nil, openaiInitError
	}

	client := openai.NewClient(apiKey)
	openaiClient = client
	return client, nil
}

var Agents = map[string]*agents.Agent{
	"search": {
		Description: "Search the knowledge base for relevant passages.",
		Inputs: map[string]*agents.Argument{
			"query": {
				Description: "What are you looking for?",
				Type:        "string",
				Required:    true,
			},
			"limit": {
				Description: "Max number of results to return.",
				Type:        "string",
				Required:    true,
			},
		},
		Function: Search,
	},

	"answer_with_sources": {
		Description: "Return an answer with cited source excerpts.",
		Inputs: map[string]*agents.Argument{
			"question": {
				Description: "What do you want to know?",
				Type:        "string",
				Required:    true,
			},
		},
		Function: AnswerWithSources,
	},

	"extract_facts": {
		Description: "Return bullet-pointed facts from the most relevant documents.",
		Inputs: map[string]*agents.Argument{
			"query": {
				Description: "What information are you trying to gather?",
				Type:        "string",
				Required:    true,
			},
		},
		Function: ExtractFacts,
	},

	"show_inputs": {
		Description: "Return the inputs to the agent.",
		Function: func(ctx context.Context, input map[string]any) (string, error) {
			return fmt.Sprintf("Inputs: %v", input), nil
		},
	},
}

var (
	qdrantClient      *qdrant.Client
	qdrantInitError   error
	qdrantInitialized bool
)

func getQdrantClient(ctx context.Context) (*qdrant.Client, error) {
	if qdrantInitialized {
		return qdrantClient, qdrantInitError
	}
	qdrantInitialized = true

	client, err := qdrant.NewClient(&qdrant.Config{
		Host: "localhost",
		Port: 6334,
	})
	if err != nil {
		qdrantInitError = fmt.Errorf("failed to create Qdrant client: %w", err)
		return nil, qdrantInitError
	}

	_, err = client.ListCollections(ctx)
	if err != nil {
		qdrantInitError = fmt.Errorf("cannot connect to Qdrant server at localhost:6334: %w", err)
		return nil, qdrantInitError
	}

	qdrantClient = client
	return client, nil
}

func createCollection(name string) {
	client, err := getQdrantClient(context.Background())
	if err != nil {
		fmt.Println("[RAG] Error:", err)
		return
	}
	client.CreateCollection(context.Background(), &qdrant.CreateCollection{
		CollectionName: name,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     4,
			Distance: qdrant.Distance_Cosine,
		}),
	})
}

func upsertVectors(name string, vec []float32) (*qdrant.UpdateResult, error) {
	client, err := getQdrantClient(context.Background())
	if err != nil {
		return nil, err
	}

	operationInfo, err := client.Upsert(context.Background(), &qdrant.UpsertPoints{
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
	client, err := getQdrantClient(context.Background())
	if err != nil {
		return nil, err
	}

	result, err := client.CreateFieldIndex(context.Background(), &qdrant.CreateFieldIndexCollection{
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
	client, err := getQdrantClient(context.Background())
	if err != nil {
		return nil, err
	}

	results, err := client.CreateFieldIndex(context.Background(), &qdrant.CreateFieldIndexCollection{
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
	client, err := getQdrantClient(context.Background())
	if err != nil {
		panic("failed to get Qdrant client: " + err.Error())
	}

	searchResult, err := client.Query(context.Background(), &qdrant.QueryPoints{
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
	client, err := getQdrantClient(context.Background())
	if err != nil {
		panic("failed to get Qdrant client: " + err.Error())
	}

	max := new(uint64)
	*max = limit
	searchResult, err := client.Query(context.Background(), &qdrant.QueryPoints{
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
	client, err := getOpenAIClient()
	if err != nil {
		return nil, err
	}
	embedding, err := client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
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
