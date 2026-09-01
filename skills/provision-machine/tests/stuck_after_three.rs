//! AC123 — three consecutive failures → status: stuck, retries stop.

use provision_machine::fakes::*;
use provision_machine::*;
use std::path::PathBuf;

fn inp() -> ProvisionInputs<'static> {
    ProvisionInputs {
        host_id: "agent-host-bad",
        image: "ubuntu/24.04/cloud",
        cpu: 2,
        ram_gb: 4,
        disk_gb: 20,
        role: "repo-agent",
        machine_sh_local: PathBuf::from("/tmp/machine.sh"),
    }
}

#[test]
fn three_failures_marks_stuck_and_stops() {
    let tmp = tempfile::tempdir().unwrap();
    let state_path = tmp.path().join("agent-host-bad.json");
    let notifier = CapturingNotifier::default();
    let clock = FixedClock("2026-04-28T00:00:00Z".into());

    for attempt in 1..=3 {
        let ops = FakeIncusOps::default();
        ops.fail_at(0); // every call fails
        let res = provision(&inp(), &ops, &notifier, &clock, &state_path, false);
        assert!(res.is_err(), "attempt {attempt} should error");
    }

    let st = provision_machine::read_state(&state_path).unwrap();
    assert_eq!(st.status, "stuck");
    assert_eq!(st.consecutive_failures, 3);

    // Once stuck, a fourth invocation must NOT re-attempt.
    let ops = FakeIncusOps::default();
    let st = provision(&inp(), &ops, &notifier, &clock, &state_path, false).unwrap();
    assert_eq!(st.status, "stuck");
    assert!(ops.calls.lock().unwrap().is_empty(), "no calls after stuck");

    // Notifies: at least 3 (failed/failed/stuck). Stuck event must appear.
    let events = notifier.events.lock().unwrap();
    assert!(events.iter().any(|(k, _)| k == "host_stuck_provisioning"));
}
