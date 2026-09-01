//! S20 / AC108 — missing private key on host triggers regeneration + GitLab registration.

use reconcile_identities::fakes::*;
use reconcile_identities::*;

fn ts() -> jiff::Timestamp {
    "2026-04-28T03:00:00Z".parse().unwrap()
}

#[test]
fn missing_key_regenerates_and_registers() {
    let tmp = tempfile::tempdir().unwrap();
    let ledger = tmp.path().join("identities.json");

    let agents = vec![AgentSpec {
        name: "df-chat-coder".into(),
        host: "agent-host-1".into(),
        target_repo: Some("df-chat".into()),
    }];

    let gl = FakeGitlab::default();
    let fs = FakeHostFs::default(); // empty → key missing
    let clock = FixedClock(ts());
    let notifier = CapturingNotifier::default();

    let report = reconcile(&agents, &gl, &fs, &clock, &notifier, &ledger, false).unwrap();

    assert_eq!(report.generated_keys, vec!["df-chat-coder"]);
    assert_eq!(report.registered_keys.len(), 2); // design + target repo
    assert!(notifier.events.lock().unwrap().is_empty());

    // Re-running should be a no-op.
    let fs2 = FakeHostFs::default();
    fs2.already_has(
        "agent-host-1",
        "df-chat-coder",
        Keypair {
            private_pem: "x".into(),
            public_openssh: "ssh-ed25519 AAAA-fake df-chat-coder@agent-host-1".into(),
            fingerprint: "SHA256:fake".into(),
        },
    );
    let report2 = reconcile(&agents, &gl, &fs2, &clock, &notifier, &ledger, false).unwrap();
    assert!(report2.generated_keys.is_empty());
    assert!(report2.registered_keys.is_empty());
}
