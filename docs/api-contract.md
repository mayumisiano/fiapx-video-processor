# API Contract — FIAP X (Video Processing System)

> Contract for `identity-api` and `video-api` (see `docs/technical-architecture.md` and `docs/adr/0007`). Derived from the use cases (`docs/use-cases.md`) and the core domain model (`docs/core-domain.md`). This is the source of truth for both backend implementation and frontend integration (replacing the mocked `lib/api-client.ts` in `fiapx-frontend`).
>
> Two services, two base URLs — no gateway yet: `/auth/*` is served by `identity-api` (`http://localhost:8081`), everything else (`/videos/*`) by `video-api` (`http://localhost:8080`). Both use the `/api/v1` prefix below.

## Conventions

- **Base URL**: `/api/v1` on each service (see above — `identity-api` vs. `video-api`)
- **Content-Type**: `application/json`, except for upload (`multipart/form-data`).
- **Authentication**: `Authorization: Bearer <jwt>` header, required on every route except `POST /auth/register` and `POST /auth/login`.
- **IDs**: UUID v4 strings.
- **Timestamps**: ISO 8601 UTC (e.g., `2026-09-03T14:20:00Z`).
- **Status enum** (`ProcessingRequest.status`): `PENDING` | `PROCESSING` | `COMPLETED` | `FAILED`.
- **Failure reason enum** (`ProcessingRequest.failureReason`): `INVALID_FORMAT` | `SIZE_EXCEEDED` | `CORRUPTED_FILE` | `INTERNAL_ERROR`.

### Error response shape

Every non-2xx response follows this shape:

```json
{
  "error": {
    "code": "INVALID_FORMAT",
    "message": "Unsupported video format. Accepted formats: mp4, mov, mkv, webm."
  }
}
```

`code` is a machine-readable string (stable across releases, safe to branch on in the frontend); `message` is a human-readable string suitable for direct display.

---

## POST /auth/register

Creates a new user account and returns a session token (see UC01).

**Auth**: none.

**Request body**:
```json
{
  "name": "Jane Doe",
  "email": "jane@example.com",
  "password": "at-least-8-characters"
}
```

**Success — `201 Created`**:
```json
{
  "user": { "id": "3f2a...", "name": "Jane Doe", "email": "jane@example.com" },
  "token": "eyJhbGciOi..."
}
```

**Errors**:
| Status | `code` | When |
|---|---|---|
| 400 | `VALIDATION_ERROR` | Invalid email format or password below minimum length |
| 409 | `EMAIL_ALREADY_REGISTERED` | Email already exists |

---

## POST /auth/login

Authenticates an existing user (see UC02).

**Auth**: none.

**Request body**:
```json
{ "email": "jane@example.com", "password": "at-least-8-characters" }
```

**Success — `200 OK`**:
```json
{
  "user": { "id": "3f2a...", "name": "Jane Doe", "email": "jane@example.com" },
  "token": "eyJhbGciOi..."
}
```

**Errors**:
| Status | `code` | When |
|---|---|---|
| 401 | `INVALID_CREDENTIALS` | Email not found or password doesn't match (same code/message for both, to avoid user enumeration) |
| 429 | `TOO_MANY_ATTEMPTS` | Rate limit exceeded on this endpoint |

---

## POST /videos

Submits one or more videos for processing (see UC03).

**Auth**: required.

**Request**: `multipart/form-data`, field name `videos` repeated per file (supports multiple files in a single request).

**Success — `201 Created`**:

Returns one entry per submitted file, whether accepted or rejected — so the frontend can show a per-file outcome without extra round-trips.

```json
{
  "results": [
    {
      "fileName": "lecture.mp4",
      "accepted": true,
      "request": {
        "id": "b7e1...",
        "fileName": "lecture.mp4",
        "status": "PENDING",
        "attempts": 1,
        "createdAt": "2026-09-03T14:20:00Z"
      }
    },
    {
      "fileName": "clip.wmv",
      "accepted": false,
      "error": { "code": "INVALID_FORMAT", "message": "Unsupported video format. Accepted formats: mp4, mov, mkv, webm." }
    }
  ]
}
```

**Notes**:
- HTTP status is `201` as long as the request itself was well-formed, even if individual files were rejected — rejection is business-level, communicated per item in the `results` array (see UC03 design note: one file's failure must not block the others).
- Server-side validation order per file, per `docs/domain-modeling.md` §7.2/7.6: format → size (≤ 500 MB) → duration (≤ 30 min).

### Design decision: multipart via `api`, not a presigned upload URL

Unlike `GET /videos/{id}/download` (which uses a presigned URL so the binary never flows through `api`), uploads go through `api` as a regular `multipart/form-data` request. This is intentional, not an oversight:

1. **Synchronous, pre-acceptance validation.** UC03 requires validating format, size, and duration *before* deciding whether a video is accepted (`VideoRejected` vs. `VideoAcceptedForProcessing`), returning a per-file result in the same request. This requires the `api` to actually see the bytes — checking magic bytes (not just trusting the extension, per `docs/technical-architecture.md` §8) and inspecting real duration (e.g., via `ffprobe`) before the object is written to storage. With a presigned upload, the client writes directly to MinIO and the `api` never sees the bytes at accept-time; validation would have to happen *after* the object already landed in storage, breaking the "reject before accept" premise and turning it into an extra asynchronous step (e.g., a storage webhook notifying `api`, which then downloads/inspects, then decides).
2. **No orphaned objects.** A presigned-upload flow is: `api` issues a URL → client uploads directly to storage → client (or storage) notifies `api` that the upload finished. If that last step fails (dropped connection, closed tab), you end up with a file in storage with no corresponding business record, requiring a cleanup job. Routing the upload through `api` keeps record creation and file receipt as a single logical step.
3. **Fewer moving parts for the hackathon timeline.** Presigned upload requires CORS configuration on MinIO (for direct browser `PUT` requests), an additional "confirm upload" endpoint, and more error scenarios to implement and test — consistent complexity we chose to avoid elsewhere (see `docs/technical-architecture.md` §6).

**Known trade-off, accepted consciously**: with multipart-via-`api`, upload bandwidth for every video (up to 500 MB, per `docs/domain-modeling.md` §7.2) flows through the `api` process, which becomes a bottleneck under many concurrent large uploads — the opposite of how large-scale upload products (YouTube, Google Drive, etc.) work, which upload directly to storage via a signed URL. For the MVP's size limits and timeline, this cost is acceptable. If this system needed to support much larger files or far higher upload concurrency, **migrating `POST /videos` to a presigned-upload flow** (issue URL → direct upload to MinIO → confirmation endpoint or storage event triggers validation and enqueueing) would be the natural next evolution — logged here as a future improvement, not implemented now.

**Errors (request-level, not per-file)**:
| Status | `code` | When |
|---|---|---|
| 400 | `NO_FILES_PROVIDED` | No file included in the request |
| 401 | `UNAUTHORIZED` | Missing/invalid/expired token |

---

## GET /videos

Lists the authenticated user's processing requests (see UC04).

**Auth**: required.

**Query params**: none required for the MVP (no pagination — acceptable given the 500 MB / 30 min / no-concurrency-limit scope; see `docs/domain-modeling.md` §7.2).

**Success — `200 OK`**:
```json
{
  "requests": [
    {
      "id": "b7e1...",
      "fileName": "lecture.mp4",
      "status": "COMPLETED",
      "attempts": 1,
      "frameCount": 1800,
      "createdAt": "2026-09-03T14:20:00Z",
      "updatedAt": "2026-09-03T14:23:10Z"
    },
    {
      "id": "a91c...",
      "fileName": "interview.mov",
      "status": "FAILED",
      "failureReason": "CORRUPTED_FILE",
      "attempts": 2,
      "createdAt": "2026-09-03T13:00:00Z",
      "updatedAt": "2026-09-03T13:05:00Z"
    }
  ]
}
```

Ordered from most recent to oldest (`createdAt` descending).

**Errors**:
| Status | `code` | When |
|---|---|---|
| 401 | `UNAUTHORIZED` | Missing/invalid/expired token |

---

## GET /videos/{id}/download

Generates a short-lived presigned URL for the result package (see UC05).

**Auth**: required.

**Success — `200 OK`**:
```json
{ "downloadUrl": "https://storage.fiapx.local/results/b7e1....zip?X-Amz-Signature=...", "expiresIn": 300 }
```

**Errors**:
| Status | `code` | When |
|---|---|---|
| 404 | `REQUEST_NOT_FOUND` | Request doesn't exist, or doesn't belong to the authenticated user (same code for both — see UC05 2a) |
| 409 | `RESULT_NOT_READY` | Request exists but `status != COMPLETED` |
| 410 | `RESULT_EXPIRED` | Result already passed the 1-month retention window (`docs/domain-modeling.md` §7.3) |

---

## POST /videos/{id}/retry

Requests reprocessing of a failed request, without a new upload (see UC06).

**Auth**: required.

**Request body**: none.

**Success — `202 Accepted`**:
```json
{
  "request": {
    "id": "a91c...",
    "fileName": "interview.mov",
    "status": "PENDING",
    "attempts": 2,
    "createdAt": "2026-09-03T13:00:00Z",
    "updatedAt": "2026-09-03T13:10:00Z"
  }
}
```

**Errors**:
| Status | `code` | When |
|---|---|---|
| 404 | `REQUEST_NOT_FOUND` | Request doesn't exist, or doesn't belong to the authenticated user |
| 409 | `NOT_FAILED` | Request isn't currently `FAILED` |
| 409 | `RETRIES_EXHAUSTED` | `attempts` already reached 3 (see UC06 3a) |
| 410 | `ORIGINAL_VIDEO_UNAVAILABLE` | Original video no longer available in storage (rare, see UC06 3b) |

---

## Summary table

| Method | Path | Auth | Use case |
|---|---|---|---|
| POST | `/auth/register` | No | UC01 |
| POST | `/auth/login` | No | UC02 |
| POST | `/videos` | Yes | UC03 |
| GET | `/videos` | Yes | UC04 |
| GET | `/videos/{id}/download` | Yes | UC05 |
| POST | `/videos/{id}/retry` | Yes | UC06 |

Logout (UC08) has no corresponding endpoint — it's purely a frontend action (discarding the stored JWT), consistent with stateless authentication.

## Frontend integration notes

The current `fiapx-frontend` prototype (`src/lib/api-client.ts`) already matches this contract closely (job shape, status enum values, multi-file upload), with three gaps to close when wiring the real API:

1. **Retry action**: the mocked client has no `retry` call yet — needs `videoApi.retryJob(id)` calling `POST /videos/{id}/retry`, plus a "Retry" button conditionally rendered when `status === "FAILED"` and `attempts < 3` (mirroring UC06).
2. **Structured failure reason**: the mock uses a free-text `errorMessage`; the real API returns `failureReason` as an enum code. The frontend should map each code to a friendly, translated message (client-side dictionary), not just display the raw code.
3. **Accepted formats list**: update from the original 7-format list to the 4 formats decided in `docs/domain-modeling.md` §7.6 (`.mp4`, `.mov`, `.mkv`, `.webm`).
