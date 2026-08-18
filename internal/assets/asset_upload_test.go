package assets

import (
	"context"
	"testing"
	"time"
)

func TestDecideUpload(t *testing.T) {
	now := time.Date(2026, time.August, 14, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		request     UploadRequest
		accepted    bool
		reason      string
		expectedKey string
	}{
		{
			name:     "course handout before deadline",
			request:  UploadRequest{CourseID: "sql-201", LearnerID: "learner-7", AssetName: "query-plan.pdf", ContentType: "application/pdf", Bytes: 2048, DueAt: now.Add(time.Hour)},
			accepted: true, reason: "upload window is open", expectedKey: "courses/sql-201/learners/learner-7/query-plan.pdf",
		},
		{
			name:     "late learner submission",
			request:  UploadRequest{CourseID: "sql-201", LearnerID: "learner-7", AssetName: "query-plan.pdf", ContentType: "application/pdf", Bytes: 2048, DueAt: now},
			accepted: false, reason: "learner deadline has passed",
		},
		{
			name:     "oversized recording",
			request:  UploadRequest{CourseID: "sql-201", LearnerID: "learner-7", AssetName: "demo.mp4", ContentType: "video/mp4", Bytes: maxAssetBytes + 1, DueAt: now.Add(time.Hour)},
			accepted: false, reason: "asset size must be between 1 byte and 50 MiB",
		},
		{
			name:     "unapproved archive",
			request:  UploadRequest{CourseID: "sql-201", LearnerID: "learner-7", AssetName: "work.zip", ContentType: "application/zip", Bytes: 2048, DueAt: now.Add(time.Hour)},
			accepted: false, reason: "content type is not allowed for course assets",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecideUpload(tt.request, now)
			if got.Accepted != tt.accepted || got.Reason != tt.reason || got.Key != tt.expectedKey {
				t.Fatalf("DecideUpload() = %+v", got)
			}
		})
	}
}

type recordingSigner struct {
	input PresignPutInput
}

func (s *recordingSigner) PresignPut(_ context.Context, _, _ string, input PresignPutInput) (string, error) {
	s.input = input
	return "https://upload.example/signed", nil
}

func TestAuthorizeBindsUploadConstraints(t *testing.T) {
	now := time.Date(2026, time.August, 14, 9, 0, 0, 0, time.UTC)
	signer := &recordingSigner{}
	service := NewUploadService("course-assets", signer, func() time.Time { return now })
	req := UploadRequest{CourseID: "sql-201", LearnerID: "learner-7", AssetName: "lesson.png", ContentType: "image/png", Bytes: 4096, DueAt: now.Add(time.Hour)}

	decision, grant, err := service.Authorize(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Accepted || grant.Method != "PUT" || signer.input.ContentType != req.ContentType || signer.input.MaxBytes != req.Bytes {
		t.Fatalf("grant did not preserve decision constraints: decision=%+v grant=%+v input=%+v", decision, grant, signer.input)
	}
	if signer.input.IdempotencyKey == "" {
		t.Fatal("expected a stable idempotency key")
	}
}
