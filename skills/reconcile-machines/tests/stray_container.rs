//! S18 base — observed container not in machines.yml → decommission invoked.

use reconcile_machines::fakes::*;
use reconcile_machines::*;

#[test]
fn stray_container_dispatches_decommission() {
    let machines = MachinesYml {
        schema_version: 1,
        hosts: vec![],
    };
    let incus = FakeIncus::with_running(["agent-host-zombie"]);
    let dispatch = CapturingDispatch::default();
    let notifier = CapturingNotifier::default();

    let plan = reconcile(&machines, &incus, &dispatch, &notifier, false).unwrap();

    assert_eq!(plan.decommission, vec!["agent-host-zombie"]);
    assert_eq!(
        *dispatch.decommissioned.lock().unwrap(),
        vec!["agent-host-zombie"]
    );
}

#[test]
fn dry_run_emits_plan_without_dispatching() {
    let machines = MachinesYml {
        schema_version: 1,
        hosts: vec![],
    };
    let incus = FakeIncus::with_running(["zombie-1"]);
    let dispatch = CapturingDispatch::default();
    let notifier = CapturingNotifier::default();

    let plan = reconcile(&machines, &incus, &dispatch, &notifier, true).unwrap();

    assert_eq!(plan.decommission, vec!["zombie-1"]);
    assert!(dispatch.decommissioned.lock().unwrap().is_empty());
}
