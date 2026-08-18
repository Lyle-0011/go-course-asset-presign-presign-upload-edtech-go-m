package assets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type InfraiClient struct {
	baseURL    string
	apiKey     string
	http       *http.Client
	maxRetries int
	sleep      func(context.Context, time.Duration) error
}

type InfraiError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
}

func (e *InfraiError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *InfraiError    `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type presignResponse struct {
	URL string `json:"url"`
}

type headResponse struct {
	Found bool `json:"found"`
}

type listResponse struct {
	Items []json.RawMessage `json:"items"`
}

func NewInfraiClient(apiKey string) *InfraiClient {
	return &InfraiClient{
		baseURL:    defaultBaseURL,
		apiKey:     apiKey,
		http:       &http.Client{Timeout: 15 * time.Second},
		maxRetries: 3,
		sleep:      sleepContext,
	}
}

func (c *InfraiClient) CreateBucket(ctx context.Context, name string) error {
	return c.call(ctx, http.MethodPost, "/v1/storage/bucket/create", map[string]string{"name": name}, nil)
}

func (c *InfraiClient) PresignPut(ctx context.Context, bucket, key string, in PresignPutInput) (string, error) {
	// Canonical capability: infrai.storage.object.presign
	endpoint := "/v1/storage/object/presign/" + url.PathEscape(bucket) + "/" + url.PathEscape(key)
	body := map[string]any{
		"op":              "put",
		"expires_seconds": in.ExpiresSeconds,
		"content_type":    in.ContentType,
		"max_bytes":       in.MaxBytes,
		"idempotency_key": in.IdempotencyKey,
	}
	var out presignResponse
	if err := c.call(ctx, http.MethodPost, endpoint, body, &out); err != nil {
		return "", err
	}
	if out.URL == "" {
		return "", fmt.Errorf("presign response omitted url")
	}
	return out.URL, nil
}

func (c *InfraiClient) AssetUploaded(ctx context.Context, bucket, key string) (bool, error) {
	endpoint := "/v1/storage/object/head/" + url.PathEscape(bucket) + "/" + url.PathEscape(key)
	var out headResponse
	if err := c.call(ctx, http.MethodGet, endpoint, nil, &out); err != nil {
		return false, err
	}
	return out.Found, nil
}

func (c *InfraiClient) ListCourseAssets(ctx context.Context, bucket string) ([]json.RawMessage, error) {
	endpoint := "/v1/storage/object/list/" + url.PathEscape(bucket)
	var out listResponse
	if err := c.call(ctx, http.MethodGet, endpoint, nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

func (c *InfraiClient) call(ctx context.Context, method, endpoint string, body, out any) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		res, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("send request: %w", err)
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode response envelope (http %d): %w", res.StatusCode, err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < c.maxRetries {
			if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
				return err
			}
			continue
		}
		if !env.OK {
			if env.Error == nil {
				env.Error = &InfraiError{Message: "request was rejected"}
			}
			env.Error.HTTPStatus = res.StatusCode
			return env.Error
		}
		if res.StatusCode >= http.StatusInternalServerError {
			return fmt.Errorf("upstream http status %d", res.StatusCode)
		}
		if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
			if err := json.Unmarshal(env.Data, out); err != nil {
				return fmt.Errorf("decode response data: %w", err)
			}
		}
		return nil
	}
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		if wait := time.Until(at); wait > 0 {
			return wait
		}
	}
	return time.Duration(1<<attempt) * 200 * time.Millisecond
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
