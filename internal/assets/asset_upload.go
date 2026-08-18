package assets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"
)

const maxAssetBytes int64 = 50 << 20

var allowedContentTypes = map[string]bool{
	"application/pdf": true,
	"audio/mpeg":      true,
	"image/jpeg":      true,
	"image/png":       true,
	"video/mp4":       true,
}

type UploadRequest struct {
	CourseID    string    `json:"course_id"`
	LearnerID   string    `json:"learner_id"`
	AssetName   string    `json:"asset_name"`
	ContentType string    `json:"content_type"`
	Bytes       int64     `json:"bytes"`
	DueAt       time.Time `json:"due_at"`
}

type UploadDecision struct {
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason"`
	Key      string `json:"key,omitempty"`
}

type UploadGrant struct {
	UploadURL string    `json:"upload_url"`
	Method    string    `json:"method"`
	Key       string    `json:"key"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Signer interface {
	PresignPut(context.Context, string, string, PresignPutInput) (string, error)
}

type PresignPutInput struct {
	ExpiresSeconds int
	ContentType    string
	MaxBytes       int64
	IdempotencyKey string
}

type UploadService struct {
	bucket string
	signer Signer
	now    func() time.Time
}

func NewUploadService(bucket string, signer Signer, now func() time.Time) *UploadService {
	return &UploadService{bucket: bucket, signer: signer, now: now}
}

func DecideUpload(req UploadRequest, now time.Time) UploadDecision {
	if strings.TrimSpace(req.CourseID) == "" || strings.TrimSpace(req.LearnerID) == "" || strings.TrimSpace(req.AssetName) == "" {
		return UploadDecision{Reason: "course_id, learner_id, and asset_name are required"}
	}
	if !req.DueAt.After(now) {
		return UploadDecision{Reason: "learner deadline has passed"}
	}
	if req.Bytes <= 0 || req.Bytes > maxAssetBytes {
		return UploadDecision{Reason: "asset size must be between 1 byte and 50 MiB"}
	}
	if !allowedContentTypes[req.ContentType] {
		return UploadDecision{Reason: "content type is not allowed for course assets"}
	}

	name := path.Base(strings.ReplaceAll(req.AssetName, "\\", "/"))
	if name == "." || name == "/" || name == "" {
		return UploadDecision{Reason: "asset_name must identify a file"}
	}
	key := fmt.Sprintf("courses/%s/learners/%s/%s", cleanSegment(req.CourseID), cleanSegment(req.LearnerID), cleanSegment(name))
	return UploadDecision{Accepted: true, Reason: "upload window is open", Key: key}
}

func (s *UploadService) Authorize(ctx context.Context, req UploadRequest) (UploadDecision, UploadGrant, error) {
	decision := DecideUpload(req, s.now())
	if !decision.Accepted {
		return decision, UploadGrant{}, nil
	}

	const expiry = 600
	idempotencyKey := stableRequestID(req, decision.Key)
	uploadURL, err := s.signer.PresignPut(ctx, s.bucket, decision.Key, PresignPutInput{
		ExpiresSeconds: expiry,
		ContentType:    req.ContentType,
		MaxBytes:       req.Bytes,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return UploadDecision{}, UploadGrant{}, err
	}
	return decision, UploadGrant{
		UploadURL: uploadURL,
		Method:    "PUT",
		Key:       decision.Key,
		ExpiresAt: s.now().Add(expiry * time.Second),
	}, nil
}

func cleanSegment(value string) string {
	return url.PathEscape(strings.TrimSpace(value))
}

func stableRequestID(req UploadRequest, key string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%s", key, req.ContentType, req.Bytes, req.DueAt.UTC().Format(time.RFC3339))))
	return "course-asset-" + hex.EncodeToString(sum[:12])
}

var ErrMissingObject = errors.New("course asset has not been uploaded")
