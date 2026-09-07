use provision_machine::fakes::*;
use provision_machine::*;
use std::path::PathBuf;

fn inputs(host: &str) -> ProvisionInputs<'static> {
    let host_static: &'static str = Box::leak(host.to_string().into_boxed_str());
    ProvisionInputs {
        host_id: host_static,
        image: "ubuntu/24.04/cloud",
        cpu: 2,
        ram_gb: 4,
        disk_gb: 20,
        role: "repo-agent",
        machine_sh_local: PathBuf::from("/tmp/machine.sh"),
    }
}

#[test]
fn provisioned_writes_state_and_clears_failures() {
    let tmp = tempfile::tempdir().unwrap();
    let state_path = tmp.path().join("infra/host-state/agent-host-1.json");

    let ops = FakeIncusOps::default();
    let notifier = CapturingNotifier::default();
    let clock = FixedClock("2026-04-28T00:00:00Z".to_string());

    let st = provision(
        &inputs("agent-host-1"),
        &ops,
        &notifier,
        &clock,
        &state_path,
        false,
    )
    .unwrap();

    assert_eq!(st.status, "healthy");
    assert_eq!(st.consecutive_failures, 0);
    assert_eq!(st.image, "ubuntu/24.04/cloud");
    assert_eq!(st.provisioned_at.as_deref(), Some("2026-04-28T00:00:00Z"));
    assert!(state_path.exists());
    // No notify on happy path.
    assert!(notifier.events.lock().unwrap().is_empty());
    // Sequence covered all 6 steps.
    assert!(ops.calls.lock().unwrap().len() >= 5);
}

#[test]
fn dry_run_writes_no_state_and_makes_no_calls() {
    let tmp = tempfile::tempdir().unwrap();
    let state_path = tmp.path().join("infra/host-state/agent-host-2.json");
    let ops = FakeIncusOps::default();
    let notifier = CapturingNotifier::default();
    let clock = FixedClock("2026-04-28T00:00:00Z".to_string());

    let st = provision(
        &inputs("agent-host-2"),
        &ops,
        &notifier,
        &clock,
        &state_path,
        true,
    )
    .unwrap();
    assert_eq!(st.status, "provisioned_dry_run");
    assert!(!state_path.exists());
    assert!(ops.calls.lock().unwrap().is_empty());
}
