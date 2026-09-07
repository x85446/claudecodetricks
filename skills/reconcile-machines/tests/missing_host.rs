//! S17 base — a declared host with no running container → provision invoked.

use reconcile_machines::fakes::*;
use reconcile_machines::*;

#[test]
fn missing_host_dispatches_provision() {
    let machines = MachinesYml {
        schema_version: 1,
        hosts: vec![DeclaredHost {
            id: "agent-host-3".into(),
            agents: vec!["coder@df-payment".into(), "tester@df-payment".into()],
            purpose: None,
            image: None,
            cpu: None,
            ram_gb: None,
            disk_gb: None,
        }],
    };
    let incus = FakeIncus::with_running::<[&str; 0], &str>([]);
    let dispatch = CapturingDispatch::default();
    let notifier = CapturingNotifier::default();

    let plan = reconcile(&machines, &incus, &dispatch, &notifier, false).unwrap();

    assert_eq!(plan.provision, vec!["agent-host-3"]);
    assert_eq!(*dispatch.provisioned.lock().unwrap(), vec!["agent-host-3"]);
    assert!(notifier.events.lock().unwrap().is_empty());
}
