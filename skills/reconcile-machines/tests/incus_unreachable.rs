//! AC125 — `incus list` failing emits notify(incus_unreachable) and does
//! NOT cascade per-host failures.

use reconcile_machines::fakes::*;
use reconcile_machines::*;

#[test]
fn incus_unreachable_notifies_once_and_returns_empty_plan() {
    let machines = MachinesYml {
        schema_version: 1,
        hosts: vec![DeclaredHost {
            id: "agent-host-1".into(),
            agents: vec![],
            purpose: None,
            image: None,
            cpu: None,
            ram_gb: None,
            disk_gb: None,
        }],
    };
    let incus = FakeIncus::default();
    incus.fail_list();
    let dispatch = CapturingDispatch::default();
    let notifier = CapturingNotifier::default();

    let plan = reconcile(&machines, &incus, &dispatch, &notifier, false).unwrap();

    assert!(plan.provision.is_empty());
    assert!(plan.health.is_empty());
    let events = notifier.events.lock().unwrap();
    assert_eq!(events.len(), 1);
    assert_eq!(events[0].0, "incus_unreachable");
    // Important: nothing dispatched even though host was missing.
    assert!(dispatch.provisioned.lock().unwrap().is_empty());
}
