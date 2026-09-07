# Use Cases — FIAP X (Video Processing System)

> Step-by-step breakdown, per actor, of main and exception flows. Derived directly from the Event Storming (`docs/event-storming.md`) and the core domain model (`docs/core-domain.md`). Serves as the direct basis for acceptance criteria and test cases.

## Actors

- **Customer**: authenticated end user who submits videos and tracks their results.
- **System (Worker)**: internal/automatic actor that runs background processing — included here because part of the flows (UC05, UC07) are initiated by it, not by direct Customer interaction.

---

## UC01 — Register

**Primary actor**: Visitor (not yet registered user).

**Preconditions**: none.

**Main flow**:
1. Visitor accesses the registration screen.
2. Enters name, email, and password.
3. System validates email format and minimum password requirements.
4. System checks that the email isn't already registered.
5. System creates the user (password stored as a hash), emits the `UserRegistered` event.
6. System automatically authenticates the newly created user and redirects to the Dashboard.

**Exception flows**:
- **3a. Invalid email format or password doesn't meet the minimum**: system shows a validation error, stays on the registration screen, no data is persisted.
- **4a. Email already registered**: system shows "email already in use," stays on the registration screen.

**Postconditions**: user created and authenticated (success case).

---

## UC02 — Sign in (Login)

**Primary actor**: Customer (already registered).

**Preconditions**: user has an active registration.

**Main flow**:
1. Customer enters email and password on the login screen.
2. System validates the credentials.
3. System emits the `UserAuthenticated` event, generates a session token.
4. Customer is redirected to the Dashboard.

**Exception flows**:
- **2a. Invalid credentials**: system shows a generic message "invalid email or password" (never specify which of the two is incorrect, for security reasons). Customer stays on the login screen.

**Postconditions**: valid session token issued and stored by the client (frontend).

---

## UC03 — Submit video(s) for processing

**Primary actor**: Authenticated Customer.

**Preconditions**: valid session (token not expired).

**Main flow**:
1. Customer accesses the Dashboard and selects one or more video files.
2. For each selected file, the system validates, in order:
   a. Supported format (`.mp4`, `.mov`, `.mkv`, `.webm` — see `docs/domain-modeling.md` 7.6);
   b. Size ≤ 500 MB (see 7.2);
   c. Duration ≤ 30 minutes (see 7.2).
3. For each file that passes all validations: the system stores the video in object storage, creates a `ProcessingRequest` with `status = Pending`, emits `VideoAcceptedForProcessing`, publishes a message to the processing queue.
4. System responds immediately to the Customer (without waiting for processing), listing the created requests.
5. Customer sees the new requests in the list, with status `Pending`.

**Exception flows**:
- **2a. Unsupported format**: the file is rejected before any upload to storage; `VideoRejected` event (reason `INVALID_FORMAT`); Customer sees a specific error message for that file, without preventing the submission of the other valid files in the same batch.
- **2b. Size or duration exceeded**: same handling, `VideoRejected` event (reason `SIZE_EXCEEDED`).
- **No file selected**: system doesn't allow triggering "Process videos"; immediate feedback in the frontend, without a round-trip to the backend.

**Postconditions**: one `ProcessingRequest` created per accepted file, each in `Pending` and enqueued.

**Design note**: since submission is batched (multiple files), each file is evaluated and processed independently — the failure of one must not prevent the acceptance of the others.

---

## UC04 — Check request status

**Primary actor**: Authenticated Customer.

**Preconditions**: valid session.

**Main flow**:
1. Customer accesses the Dashboard.
2. System lists all `ProcessingRequest`s belonging to the authenticated user, ordered from most recent to oldest, showing: file name, current status, and available action (see UC05/UC06 depending on status).
3. Customer can trigger "Refresh" at any time, or the list auto-refreshes at a short interval (polling).

**Exception flows**:
- **No existing requests**: system shows an empty state ("no videos submitted yet").
- **Session expired during the query**: system redirects to login.

**Postconditions**: none (pure query, no side effects).

---

## UC05 — Download result (ZIP)

**Primary actor**: Authenticated Customer.

**Preconditions**: a `ProcessingRequest` belonging to the user exists with `status = Completed`.

**Main flow**:
1. Customer clicks "Download ZIP" on the corresponding row of the requests list.
2. System validates that the request belongs to the authenticated user and is `Completed`.
3. System generates a short-lived signed URL (presigned URL) for the storage object.
4. Customer is redirected to / downloads the file directly from storage (it doesn't flow through the application backend).
5. System emits the `FileDownloaded` event (for audit/observability purposes — doesn't block or restrict future downloads).

**Exception flows**:
- **2a. Request doesn't belong to the authenticated user**: system returns an authorization error (404, not 403, to avoid leaking the resource's existence to third parties).
- **2b. Request exists but isn't `Completed`**: the download button isn't even shown in this case (UC04); if triggered anyway (e.g., a direct API call), system returns an "file not yet available" error.
- **Expired result (more than 1 month since completion — see `docs/domain-modeling.md` 7.3)**: object already removed by storage; system returns "file is no longer available" clearly, instead of a generic error.

**Postconditions**: no state change on the request (idempotent — can be downloaded multiple times while available).

---

## UC06 — Retry a failed processing request

**Primary actor**: Authenticated Customer.

**Preconditions**: a `ProcessingRequest` belonging to the user exists with `status = Failed` and `attempts < 3` (see `docs/domain-modeling.md` 7.4).

**Main flow**:
1. Customer sees a request with status `Failed` and the error reason displayed.
2. Customer clicks "Retry."
3. System validates that `attempts < 3` and that the original video is still available in storage.
4. System increments `attempts`, transitions the request back to `Pending`, republishes it to the processing queue (`ReprocessingRequested` event).
5. Flow proceeds as in UC03 from step 4 onward (background processing).

**Exception flows**:
- **3a. `attempts` has already hit the limit (3)**: the "Retry" button isn't shown; the request is in a terminal state (`RetriesExhausted`), Customer is guided to submit the video again from scratch (a new UC03) if desired.
- **3b. Original video is no longer available** (rare case, e.g., storage infrastructure failure): system informs that automatic reprocessing isn't possible and suggests a new submission.

**Postconditions**: request goes back into the processing cycle (same id, attempt counter incremented).

---

## UC07 — Be notified about the processing outcome

**Primary actor**: System (Worker), with the Customer as passive recipient.

**Preconditions**: a `ProcessingRequest` has reached a terminal state for that attempt (`Completed`, `Failed`, or `RetriesExhausted`).

**Main flow**:
1. Worker completes processing successfully (`ResultFileAvailable`) or records a failure (`ProcessingFailed` or `RetriesExhausted`).
2. Notification context consumes the corresponding event.
3. System sends an email to the Customer with the outcome: download link/instructions (success) or a structured error reason in friendly language (failure — see code mapping in `docs/domain-modeling.md` 7.5).
4. `UserNotified` event is recorded.

**Exception flows**:
- **3a. Email delivery failure** (provider unavailable, etc.): must not revert or impact the request's `status` (hotspot already logged in `docs/event-storming.md`). The system may retain the notification for a retry, asynchronously and decoupled from the processing pipeline.

**Postconditions**: user informed by email (best-effort — the system's strong guarantee is about the request's *status*, reflected in the UI via UC04, not about the email's delivery itself).

---

## UC08 — Sign out (Logout)

**Primary actor**: Authenticated Customer.

**Main flow**:
1. Customer clicks "Sign out."
2. System invalidates the token locally (frontend discards the stored token).
3. Customer is redirected to the login screen.

**Exception flows**: none relevant — simple, frontend-local operation, given that authentication is stateless via JWT.

---

## Traceability: use cases × business decisions

| Use case | Business decisions applied (`docs/domain-modeling.md` §7) |
|---|---|
| UC03 | 7.2 (quotas), 7.5 (error reasons), 7.6 (formats) |
| UC05 | 7.3 (retention) |
| UC06 | 7.3 (original video retention), 7.4 (reprocessing) |
| UC07 | 7.5 (structured error reasons) |

This table should be used as the minimum checklist of acceptance test cases per use case.
