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

/**
 * Thrown when access to a config value is denied.
 *
 * This is raised when:
 * - The config key has rating=secret (always denied)
 * - HITL approval was denied or timed out
 * - The agent lacks the config.access capability
 *
 * - `key` — the config key that was denied.
 * - `code` — machine-readable error code (e.g., 'ACCESS_DENIED', 'KEY_NOT_FOUND').
 * - `detail` — human-readable explanation.
 * - `status` — HTTP status code (403, 404, 429, etc.).
 */
export class ConfigAccessError extends AgeztError {
  readonly key: string;
  readonly code: string;
  readonly status: number;

  constructor(key: string, code: string, detail: string, status = 403) {
    super(`config: ${code}: ${detail}`);
    this.name = "ConfigAccessError";
    this.key = key;
    this.code = code;
    this.status = status;
  }
}
