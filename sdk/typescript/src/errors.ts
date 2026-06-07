/** Base class for all errors thrown by this package. */
export class AgeztError extends Error {}

/**
 * Thrown when the daemon returns a non-2xx response.
 *
 * - `status` — the HTTP status code.
 * - `type` — the machine-readable error type from the body (`error.type`), or "".
 * - `detail` — the human-readable message from the body, or the raw body.
 */
export class APIError extends AgeztError {
  readonly status: number;
  readonly type: string;
  readonly detail: string;

  constructor(status: number, type = "", detail = "") {
    super(`agezt: HTTP ${status}: ${detail || type || "request failed"}`);
    this.name = "APIError";
    this.status = status;
    this.type = type;
    this.detail = detail;
  }
}
