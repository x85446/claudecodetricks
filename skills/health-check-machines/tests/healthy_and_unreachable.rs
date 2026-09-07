use health_check_machines::fakes::*;
use health_check_machines::*;

fn host(id: &str) -> Host {
    Host { id: id.into() }
}

#[test]
fn healthy_host_writes_health_and_state() {
    let tmp = tempfile::tempdir().unwrap();
    let pinger = FakePing::default();
    pinger.ok("agent-host-1", "agent-host-1");
    let notifier = CapturingNotifier::default();
    let clock = FixedClock("2026-04-28T01:00:00Z".into());

    let outcome = check_all(
        &[host("agent-host-1")],
        &pinger,
        &notifier,
        &clock,
        tmp.path(),
        tmp.path(),
    )
    .unwrap();

    assert_eq!(outcome.records.len(), 1);
    assert_eq!(outcome.records[0].status, "healthy");
    assert!(notifier.events.lock().unwrap().is_empty());
    let written = std::fs::read_to_string(tmp.path().join("agent-host-1.json")).unwrap();
    assert!(written.contains("healthy"));
}

#[test]
fn unreachable_host_notifies_and_marks_unhealthy_only_for_self() {
    let tmp_h = tempfile::tempdir().unwrap();
    let tmp_s = tempfile::tempdir().unwrap();
    let pinger = FakePing::default();
    pinger.ok("agent-host-1", "agent-host-1");
    pinger.fail("agent-host-2", "container offline");
    let notifier = CapturingNotifier::default();
    let clock = FixedClock("2026-04-28T01:00:00Z".into());

    let outcome = check_all(
        &[host("agent-host-1"), host("agent-host-2")],
        &pinger,
        &notifier,
        &clock,
        tmp_h.path(),
        tmp_s.path(),
    )
    .unwrap();

    assert_eq!(outcome.records.len(), 2);
    assert_eq!(outcome.records[0].status, "healthy");
    assert_eq!(outcome.records[1].status, "unhealthy");
    let events = notifier.events.lock().unwrap();
    assert_eq!(events.len(), 1);
    assert_eq!(events[0].0, "incus_unreachable");
    // Healthy peer untouched and present.
    assert!(tmp_h.path().join("agent-host-1.json").exists());
    assert!(tmp_h.path().join("agent-host-2.json").exists());
}

#[test]
fn chopper2_host_is_skipped() {
    let tmp = tempfile::tempdir().unwrap();
    let pinger = FakePing::default();
    let notifier = CapturingNotifier::default();
    let clock = FixedClock("2026-04-28T01:00:00Z".into());

    let outcome = check_all(
        &[host("chopper2-host")],
        &pinger,
        &notifier,
        &clock,
        tmp.path(),
        tmp.path(),
    )
    .unwrap();

    assert!(outcome.records.is_empty());
}
