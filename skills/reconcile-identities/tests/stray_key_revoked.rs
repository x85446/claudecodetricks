//! S19 / AC94 / AC95 — a deploy key whose title doesn't match any desired agent
//! must be revoked; an `audit[]` entry recorded.

use reconcile_identities::fakes::*;
use reconcile_identities::*;

fn ts() -> jiff::Timestamp {
    "2026-04-28T03:00:00Z".parse().unwrap()
}

#[test]
fn stray_key_revoked_and_audited() {
    let tmp = tempfile::tempdir().unwrap();
    let ledger = tmp.path().join("identities.json");

    let agents = vec![AgentSpec {
        name: "df-auth-coder".into(),
        host: "agent-host-1".into(),
        target_repo: Some("df-auth".into()),
    }];

    let gl = FakeGitlab::default();
    gl.seed(
        DESIGN_REPO,
        DeployKey {
            id: 999,
            title: "stranger".into(),
            fingerprint: "SHA256:stranger".into(),
            key: "ssh-ed25519 stranger".into(),
            created_at: "2026-04-01T00:00:00Z".into(),
        },
    );
    let fs = FakeHostFs::default();
    let clock = FixedClock(ts());
    let notifier = CapturingNotifier::default();

    let report = reconcile(&agents, &gl, &fs, &clock, &notifier, &ledger, false).unwrap();

    assert_eq!(report.revoked_strays.len(), 1);
    assert_eq!(report.revoked_strays[0].1, 999);
    // Stray gone from project.
    assert!(gl
        .project_keys(DESIGN_REPO)
        .iter()
        .all(|k| k.title != "stranger"));

    // Audit entry recorded.
    let saved = load_ledger(&ledger).unwrap();
    assert!(saved
        .audit
        .iter()
        .any(|e| e.event == "revoked_stray_key" && e.agent == "stranger"));
}
