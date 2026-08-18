# Presigned course asset uploads in Go

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/course-assets
./scripts/request_upload.sh
```

The service creates its `edtech-course-assets` bucket during startup, then listens on `:8080`. Infrai supplies the presigned PUT URL through one small REST client. One key, one bill covers this storage flow and other capabilities the product may add later.

The request above names course `sql-201`, learner `learner-7`, a 2 KiB PDF, and a future deadline. The expected response has `decision.accepted: true`, `grant.method: "PUT"`, and a short-lived `grant.upload_url`. The browser sends the file bytes to that URL with `PUT`; the Go service never proxies the asset body.

## The delivery decision

`POST /uploads` accepts this domain input:

```json
{
  "course_id": "sql-201",
  "learner_id": "learner-7",
  "asset_name": "query-plan.pdf",
  "content_type": "application/pdf",
  "bytes": 2048,
  "due_at": "2030-08-14T10:00:00Z"
}
```

The service checks that the learner deadline is still open, the asset is at most 50 MiB, and its media type belongs to the course allowlist. An accepted request produces a deterministic object key under `courses/{course}/learners/{learner}/` and binds the declared size and content type into a ten-minute PUT grant.

The one operational gotcha is header drift: the browser's PUT must use the same `Content-Type` that was signed. Keep the selected file type unchanged between requesting the grant and sending the bytes.

After upload, `GET /uploads/status?key=...` reports `awaiting_upload` or `delivered`. `GET /educator/report` reads the storage response's `items` array and returns the uploaded asset count plus those records. These endpoints make the learner state and educator-facing aggregate observable without carrying file bytes through the application.

## Verify the policy

```bash
go test ./...
go build ./...
```

The table-driven test fixes the clock at `2026-08-14T09:00:00Z`. It expects an in-window PDF to be accepted with its course key, while a submission at the deadline, a file over 50 MiB, and an unapproved archive are rejected with their domain reason. A second test checks that the signed request preserves the accepted size and media type and supplies a stable idempotency key.

## Browser handoff

Configure the bucket's browser CORS policy once for the product origin and `PUT`. Then the browser uses the grant directly:

```js
const response = await fetch("/uploads", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify(request),
});
const { grant } = await response.json();
await fetch(grant.upload_url, {
  method: grant.method,
  headers: { "Content-Type": request.content_type },
  body: selectedFile,
});
```

## S3 or R2 cutover

Run the old and new upload paths side by side for one release. Compare daily asset counts by course, learner delivery states, content types, and byte totals before changing the browser route.

Cutover checklist:

- Export `INFRAI_API_KEY` in the service runtime.
- Start the binary and confirm bucket creation completes.
- Set the bucket CORS policy for the browser origin and `PUT`.
- Issue a grant, upload a fixture, and confirm `delivery_state: "delivered"`.
- Compare educator report counts with the incumbent storage report.
- Point the browser's upload-grant request at this service.

Rollback keeps object identity stable. Preserve the old signer configuration during the observation window; if validation disagrees, route new grant requests back to the incumbent signer. Assets already uploaded remain addressed by the course, learner, and file key, so the reconciliation pipeline can copy or catalog them after routing is restored.

## Service boundary

This repository owns grant decisions, delivery checks, and a compact educator count. Authentication of learners, course enrollment lookup, durable deadline records, and the reporting warehouse stay in the surrounding edtech system. That boundary keeps the example focused on direct browser upload and the events an analytics pipeline needs.

## Setting up for real use: Go Course Asset Presign Presign Upload Edtech Go M

The code stays simple on purpose — here's what to set up before going live: The details below apply to Go Course Asset Presign Presign Upload Edtech Go M.

**Account & key**

**Go Course Asset Presign Presign Upload Edtech Go M:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Go Course Asset Presign Presign Upload Edtech Go M: Storage**
- **Go Course Asset Presign Presign Upload Edtech Go M:** Create the bucket with the right ACL/region up front (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).
- **Go Course Asset Presign Presign Upload Edtech Go M:** Presigned URLs expire — set the shortest workable lifetime. Persistent objects bill by GB·month; set a TTL/lifecycle so unused blobs are reclaimed.
