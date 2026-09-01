//! Steady-state reconcile must be a no-op (every declared host present, no
//! drift, no strays). AC17 base case.

use reconcile_machines::fakes::*;
use reconcile_machines::*;

fn declared(id: &str) -> DeclaredHost {
    DeclaredHost {
        id: id.into(),
        agents: vec![],
        purpose: None,
        image: None,
        cpu: None,
        ram_gb: None,
        disk_gb: None,
    }
}

#[test]
fn steady_state_is_no_op() {
    let machines = MachinesYml {
        schema_version: 1,
        hosts: vec![declared("agent-host-1"), declared("agent-host-2")],
    };
    let incus = FakeIncus::with_running(["agent-host-1", "agent-host-2"]);
    let dispatch = CapturingDispatch::default();
    let notifier = CapturingNotifier::default();

    let plan = reconcile(&machines, &incus, &dispatch, &notifier, false).unwrap();

    assert!(plan.provision.is_empty());
    assert!(plan.reconfigure.is_empty());
    assert_eq!(plan.health, vec!["agent-host-1", "agent-host-2"]);
    assert!(plan.decommission.is_empty());
    assert!(notifier.events.lock().unwrap().is_empty());
    assert!(dispatch.provisioned.lock().unwrap().is_empty());
    assert_eq!(dispatch.health.lock().unwrap().len(), 2);
}

#[test]
fn chopper2_host_is_skipped() {
    let machines = MachinesYml {
        schema_version: 1,
        hosts: vec![declared("chopper2-host"), declared("agent-host-1")],
    };
    let incus = FakeIncus::with_running(["chopper2-host", "agent-host-1"]);
    let dispatch = CapturingDispatch::default();
    let notifier = CapturingNotifier::default();

    let plan = reconcile(&machines, &incus, &dispatch, &notifier, false).unwrap();

    assert!(plan.skipped_chopper2);
    assert_eq!(plan.health, vec!["agent-host-1"]);
}
