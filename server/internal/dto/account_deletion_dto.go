package dto

// AccountDeletionStatusRequested is the response `status` constant returned
// on every successful DELETE /v1/users/me (Story 1.6). Kept as a named
// constant so the handler, docs, and any future client integration test
// reference the same literal — a drift between handler string and client
// parser would silently break the UX on re-delete / first-delete parity.
const AccountDeletionStatusRequested = "deletion_requested"

// AccountDeletionNoteMVP is the MVP-era note text explaining the 30 days
// manual cleanup window (NFR-COMP-5). A named constant makes the text
// grep-able and lets the unit test assert the long value is not truncated.
const AccountDeletionNoteMVP = "30 days manual cleanup per MVP policy"

// AccountDeletionResponse is the 202 body for DELETE /v1/users/me.
//
// Fields:
//   - Status: always AccountDeletionStatusRequested — both first-time and
//     idempotent repeat calls land here (the server returns 202 on the
//     first-write-wins path; a second call on an already-deleted user
//     returns the original timestamp unchanged, never re-stamps).
//   - RequestedAt: UTC RFC3339 string — NOT a time.Time — so JSON decoders
//     across iOS / watchOS cannot produce timezone-parsing discrepancies.
//     Handler layer converts via `.UTC().Format(time.RFC3339)` before
//     marshalling (Story 1.6 Semantic-correctness #4).
//   - Note: always AccountDeletionNoteMVP — client displays as-is to the
//     user ("your account will be deleted within 30 days").
//
// There is deliberately NO request DTO — DELETE has no body and the
// handler explicitly does not call c.ShouldBindJSON (AC #2 / §21.8 #7).
type AccountDeletionResponse struct {
	Status      string `json:"status"`
	RequestedAt string `json:"requested_at"`
	Note        string `json:"note"`
}
