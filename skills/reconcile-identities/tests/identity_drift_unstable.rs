//! AC126 — same stray key reappears within 24h → identity_drift_unstable notify.

use reconcile_identities::fakes::*;
use reconcile_identities::*;

#[test]
fn reappearing_stray_within_24h_fires_unstable_notify() {
    let tmp = tempfile::tempdir().unwrap();
    let ledger = tmp.path().join("identities.json");

    let agents = vec![AgentSpec {
        name: "ai-operator".into(),
        host: "chopper2-host".into(),
        target_repo: None,
    }];

    let gl = FakeGitlab::default();
    let fs = FakeHostFs::default();
    let notifier = CapturingNotifier::default();

    // Cycle 1: stray seeded → revoked + audited.
    let t0: jiff::Timestamp = "2026-04-28T00:00:00Z".parse().unwrap();
    let clock = FixedClock(t0);
    gl.seed(
        DESIGN_REPO,
        DeployKey {
            id: 1,
            title: "ghost".into(),
            fingerprint: "SHA256:ghost".into(),
            key: "ssh-ed25519 ghost".into(),
            created_at: "2026-04-27T23:00:00Z".into(),
        },
    );
    let r1 = reconcile(&agents, &gl, &fs, &clock, &notifier, &ledger, false).unwrap();
    assert_eq!(r1.revoked_strays.len(), 1);
    assert!(r1.unstable_strays.is_empty());

    // Cycle 2 (same day): same title reappears → flag unstable.
    let t1: jiff::Timestamp = "2026-04-28T10:00:00Z".parse().unwrap();
    let clock2 = FixedClock(t1);
    gl.seed(
        DESIGN_REPO,
        DeployKey {
            id: 2,
            title: "ghost".into(),
            fingerprint: "SHA256:ghost-v2".into(),
            key: "ssh-ed25519 ghost".into(),
            created_at: "2026-04-28T09:00:00Z".into(),
        },
    );
    let r2 = reconcile(&agents, &gl, &fs, &clock2, &notifier, &ledger, false).unwrap();
    assert_eq!(r2.unstable_strays, vec!["ghost"]);

    let evs = notifier.events.lock().unwrap();
    assert!(evs.iter().any(|(k, _)| k == "identity_drift_unstable"));
}
