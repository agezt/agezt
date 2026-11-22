//! The Rust SDK parses what the DAEMON answered.
//!
//! The fixtures in `sdk/fixtures/` are recorded from the daemon's own REST
//! handlers (`kernel/restapi`'s golden test), so these tests check the SDK's
//! parsing against the server's bytes — not against a payload this test wrote
//! itself. A change in the daemon's wire shape moves the fixture and fails
//! here.
//!
//! Regenerate the fixtures with:
//!
//! ```text
//! AGEZT_UPDATE_GOLDEN=1 go test ./kernel/restapi/ -run TestUpdateTheGoldenFixtures
//! ```

use std::collections::BTreeSet;
use std::path::PathBuf;

use agezt::{
    paths, Health, MailAcked, MailboxInbox, MailboxMessages, MailboxReplies, MailboxSent,
    MailboxTopics, Models, RunArc, RunResult, Value,
};

/// Where the recorded fixtures live: `<repo>/sdk/fixtures`.
fn fixtures_dir() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("..")
        .join("..")
        .join("sdk")
        .join("fixtures")
}

/// Read one fixture file (the server's own bytes).
fn read(name: &str) -> Value {
    let path = fixtures_dir().join(name);
    let text =
        std::fs::read_to_string(&path).unwrap_or_else(|e| panic!("read {}: {}", path.display(), e));
    Value::parse(&text).unwrap_or_else(|e| panic!("parse {}: {}", path.display(), e))
}

/// The SDK's generated paths and the server's templates are one set.
#[test]
fn the_sdk_paths_and_the_server_templates_are_one_set() {
    let manifest = read("manifest.json");
    let mut server = BTreeSet::new();
    if let Some(Value::Array(items)) = manifest.get("fixtures") {
        for item in items {
            if let Some(p) = item.get("path").and_then(Value::as_str) {
                server.insert(p.to_string());
            }
        }
    }
    let sdk: BTreeSet<String> = paths::ALL.iter().map(|p| p.to_string()).collect();
    assert_eq!(
        sdk, server,
        "the SDK's generated paths and the server's fixtures disagree"
    );
}

/// The daemon's health and model rows.
#[test]
fn the_health_and_model_rows_parse() {
    let health = Health::from_value(&read("health.json"));
    assert_eq!(health.status, "ok");
    assert_eq!(health.version, "9.9.9");
    assert_eq!(health.default_model, "model-a");
    assert_eq!(health.model_count, 2);

    let models = Models::from_value(&read("models.json"));
    assert_eq!(models.default, "model-a");
    assert_eq!(
        models.models,
        vec!["model-a".to_string(), "model-b".to_string()]
    );
}

/// The daemon's run rows.
#[test]
fn the_run_rows_parse() {
    let run = RunResult::from_value(&read("run.json"));
    assert_eq!(run.correlation_id, "corr-golden");
    assert_eq!(run.model, "model-a");
    assert_eq!(run.status, "completed");
    assert_eq!(run.answer, "golden answer");

    let arc = RunArc::from_value(&read("run_arc.json"));
    assert_eq!(arc.correlation_id, "corr-golden");
    assert_eq!(arc.count, 1);
    assert_eq!(arc.events.len(), 1);
    let kind = arc.events[0].get("kind").and_then(Value::as_str);
    assert_eq!(kind, Some("llm.token"));
}

/// The daemon's mailbox rows.
#[test]
fn the_mailbox_rows_parse() {
    let sent = MailboxSent::from_value(&read("mailbox_send.json"));
    assert_eq!(sent.message.id, "m-3");
    assert_eq!(sent.message.topic, "dm");
    assert_eq!(sent.message.text, "golden message");
    assert_eq!(sent.message.from, "agent-a");
    assert_eq!(sent.message.to, "agent-b");
    assert_eq!(sent.message.ts_unix_ms, 1700000000002);
    assert!(!sent.message.help);

    let listed = MailboxMessages::from_value(&read("mailbox_messages.json"));
    assert_eq!(listed.count, 1);
    assert_eq!(listed.messages.len(), 1);
    assert_eq!(listed.messages[0].id, "m-1");
    assert_eq!(listed.messages[0].topic, "dm");
    assert_eq!(listed.messages[0].ts_unix_ms, 1700000000000);

    let acked = MailAcked::from_value(&read("mailbox_ack.json"));
    assert!(acked.acked);
    assert_eq!(acked.id, "m-1");
    assert_eq!(acked.by, "agent-b");

    let replies = MailboxReplies::from_value(&read("mailbox_replies.json"));
    assert_eq!(replies.id, "m-1");
    assert_eq!(replies.count, 0);
    assert!(replies.replies.is_empty());

    let inbox = MailboxInbox::from_value(&read("mailbox_inbox.json"));
    assert_eq!(inbox.name, "agent-b");
    assert_eq!(inbox.count, 2);
    assert_eq!(inbox.waiting.len(), 2);
    assert_eq!(inbox.waiting[0].id, "m-2");
    assert_eq!(inbox.waiting[1].id, "m-1");
    assert_eq!(inbox.waiting[0].to, "*");

    let topics = MailboxTopics::from_value(&read("mailbox_topics.json"));
    assert_eq!(topics.topics.get("dm"), Some(&1));
    assert_eq!(topics.topics.get("status"), Some(&1));
}
