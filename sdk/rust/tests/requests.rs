//! The SDK's request rows are the daemon's contract.
//!
//! The request bodies in `sdk/fixtures/requests.json` are rendered by the
//! daemon's own REST contract (`kernel/contract/restapiapi`'s `RowBody`), so
//! this test checks that the SDK's request rows serialize to the very bytes
//! the contract declares — not to a payload this test wrote itself. A change in
//! the contract's request vocabulary moves the fixture and fails here.
//!
//! Regenerate the fixture with:
//!
//! ```text
//! AGEZT_UPDATE_GOLDEN=1 go test ./kernel/restapi/ -run TestUpdateTheGoldenFixtures
//! ```

use std::path::PathBuf;

use agezt::{MailAck, MailDraft, RunRequest, Value};

/// Where the recorded fixtures live: `<repo>/sdk/fixtures`.
fn fixtures_dir() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("..")
        .join("..")
        .join("sdk")
        .join("fixtures")
}

/// Read one fixture file (the contract's own bytes).
fn read(name: &str) -> Value {
    let path = fixtures_dir().join(name);
    let text =
        std::fs::read_to_string(&path).unwrap_or_else(|e| panic!("read {}: {}", path.display(), e));
    Value::parse(&text).unwrap_or_else(|e| panic!("parse {}: {}", path.display(), e))
}

/// The contract's request body for one row.
fn row(name: &str) -> Value {
    let fixture = read("requests.json");
    let rows = fixture.get("rows").expect("the fixture has no rows");
    rows.get(name).expect("the fixture has no such row").clone()
}

/// A run request serializes to the contract's body.
#[test]
fn a_run_request_serializes_to_the_contract_body() {
    let body = row("run_request");
    assert_eq!(RunRequest::from_value(&body).to_value(), body);
}

/// A message to leave serializes to the contract's body.
#[test]
fn a_message_to_leave_serializes_to_the_contract_body() {
    let body = row("mail_draft");
    assert_eq!(MailDraft::from_value(&body).to_value(), body);
}

/// An ack serializes to the contract's body.
#[test]
fn an_ack_serializes_to_the_contract_body() {
    let body = row("mail_ack");
    assert_eq!(MailAck::from_value(&body).to_value(), body);
}

/// An unset optional field is not sent.
///
/// The daemon reads an absent optional field and an empty one as the same
/// thing, but a request that says `"help": false` is not the request that says
/// nothing about help.
#[test]
fn an_unset_optional_field_is_not_sent() {
    let draft = MailDraft {
        text: "hello".to_string(),
        ..MailDraft::default()
    };
    assert_eq!(draft.to_value().to_json(), "{\"text\":\"hello\"}");
}
