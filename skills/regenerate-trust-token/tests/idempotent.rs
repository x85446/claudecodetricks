//! AC124 — regen is idempotent: a healthy host is a no-op; a broken host is rotated;
//! a second invocation after rotation is a no-op.

use regenerate_trust_token::fakes::*;
use regenerate_trust_token::*;

#[test]
fn healthy_host_is_no_op() {
    let tmp = tempfile::tempdir().unwrap();
    let path = tmp.path().join("agent-host-1.json");
    let trust = FakeIncusTrust::healthy();
    let clock = FixedClock("2026-04-28T02:00:00Z".into());

    let out = regenerate("agent-host-1", &trust, &clock, &path, false).unwrap();
    assert_eq!(out, Outcome::AlreadyHealthy);
    assert!(trust.issued.lock().unwrap().is_empty());
    assert!(!path.exists());
}

#[test]
fn broken_host_rotates_then_subsequent_call_is_no_op() {
    let tmp = tempfile::tempdir().unwrap();
    let path = tmp.path().join("agent-host-2.json");
    let trust = FakeIncusTrust::broken();
    let clock = FixedClock("2026-04-28T02:00:00Z".into());

    let out = regenerate("agent-host-2", &trust, &clock, &path, false).unwrap();
    assert_eq!(out, Outcome::Rotated);
    assert_eq!(trust.issued.lock().unwrap().len(), 1);
    assert!(path.exists());

    // Second invocation: trust now works → AlreadyHealthy.
    let out2 = regenerate("agent-host-2", &trust, &clock, &path, false).unwrap();
    assert_eq!(out2, Outcome::AlreadyHealthy);
    assert_eq!(
        trust.issued.lock().unwrap().len(),
        1,
        "no extra issue calls"
    );
}

#[test]
fn dry_run_does_not_install_or_write() {
    let tmp = tempfile::tempdir().unwrap();
    let path = tmp.path().join("agent-host-3.json");
    let trust = FakeIncusTrust::broken();
    let clock = FixedClock("2026-04-28T02:00:00Z".into());

    let out = regenerate("agent-host-3", &trust, &clock, &path, true).unwrap();
    assert_eq!(out, Outcome::Rotated);
    assert!(trust.installed.lock().unwrap().is_empty());
    assert!(!path.exists());
}
