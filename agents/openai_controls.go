package agents

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/robbyriverside/agencia/config"
	"golang.org/x/time/rate"

	"github.com/sashabaranov/go-openai"
)

type openAIControlSnapshot struct {
	rateLimit   int
	limiter     *rate.Limiter
	concurrency int
	semaphore   chan struct{}
}

var (
	controlMu sync.Mutex
	controls  openAIControlSnapshot
)

func getOpenAIControls(cfg config.OpenAIConfig) (chan struct{}, *rate.Limiter) {
	controlMu.Lock()
	defer controlMu.Unlock()

	if controls.rateLimit != cfg.RateLimitPerMinute {
		if cfg.RateLimitPerMinute > 0 {
			controls.limiter = rate.NewLimiter(
				rate.Every(time.Minute/time.Duration(cfg.RateLimitPerMinute)),
				cfg.RateLimitPerMinute,
			)
		} else {
			controls.limiter = nil
		}
		controls.rateLimit = cfg.RateLimitPerMinute
	}

	if controls.concurrency != cfg.MaxConcurrentRequests {
		if cfg.MaxConcurrentRequests > 0 {
			controls.semaphore = make(chan struct{}, cfg.MaxConcurrentRequests)
		} else {
			controls.semaphore = nil
		}
		controls.concurrency = cfg.MaxConcurrentRequests
	}

	return controls.semaphore, controls.limiter
}

func AcquireOpenAISlot(ctx context.Context, cfg *config.Config) (func(), error) {
	openAI := cfg.OpenAI
	semaphore, limiter := getOpenAIControls(openAI)

	acquired := false
	if semaphore != nil {
		select {
		case semaphore <- struct{}{}:
			acquired = true
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if limiter != nil {
		if err := limiter.Wait(ctx); err != nil {
			if acquired {
				<-semaphore
			}
			return nil, err
		}
	}

	return func() {
		if acquired && semaphore != nil {
			select {
			case <-semaphore:
			default:
			}
		}
	}, nil
}

func BuildChatCompletionRequest(cfg config.OpenAIConfig, messages []openai.ChatCompletionMessage, tools []openai.Tool) openai.ChatCompletionRequest {
	temperature := float32(0.2)
	if cfg.Temperature != nil {
		temperature = *cfg.Temperature
	}

	req := openai.ChatCompletionRequest{
		Model:       cfg.Model,
		Temperature: temperature,
		Messages:    messages,
		Tools:       tools,
	}
	if cfg.MaxTokens > 0 {
		req.MaxTokens = cfg.MaxTokens
	}
	return req
}

func CallChatCompletionWithRetry(ctx context.Context, client *openai.Client, req openai.ChatCompletionRequest, cfg config.OpenAIConfig) (openai.ChatCompletionResponse, error) {
	var resp openai.ChatCompletionResponse
	var err error
	backoff := cfg.RetryInitialBackoff.Duration()
	if backoff <= 0 {
		backoff = time.Second
	}
	maxBackoff := cfg.RetryMaxBackoff.Duration()
	maxAttempts := cfg.RetryAttempts + 1
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		resp, err = client.CreateChatCompletion(ctx, req)
		if err == nil {
			return resp, nil
		}
		if !isRetryable(err) || attempt == maxAttempts-1 {
			break
		}

		sleep := backoff
		if maxBackoff > 0 && sleep > maxBackoff {
			sleep = maxBackoff
		}
		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			timer.Stop()
			return resp, ctx.Err()
		case <-timer.C:
			timer.Stop()
		}
		backoff *= 2
		if maxBackoff > 0 && backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	return resp, err
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		if apiErr.HTTPStatusCode == http.StatusTooManyRequests {
			return true
		}
		if apiErr.HTTPStatusCode >= 500 && apiErr.HTTPStatusCode < 600 {
			return true
		}
		if code, ok := apiErr.Code.(string); ok {
			if code == "rate_limit_exceeded" || code == "server_error" {
				return true
			}
		}
		return false
	}

	var reqErr *openai.RequestError
	if errors.As(err, &reqErr) {
		if reqErr.HTTPStatusCode == http.StatusTooManyRequests {
			return true
		}
		if reqErr.HTTPStatusCode >= 500 && reqErr.HTTPStatusCode < 600 {
			return true
		}
	}

	return false
}
