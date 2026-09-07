use decommission_machine::fakes::*;
use decommission_machine::*;

#[test]
fn notify_first_then_delete_then_state_removal() {
    let tmp = tempfile::tempdir().unwrap();
    // Pre-create host-state file so we can confirm it gets removed.
    std::fs::create_dir_all(tmp.path().join("infra/host-state")).unwrap();
    let state_file = tmp.path().join("infra/host-state/agent-host-zombie.json");
    std::fs::write(&state_file, "{}").unwrap();

    let ops = FakeIncusOps::default();
    let notifier = CapturingNotifier::default();

    let outcome = decommission("agent-host-zombie", &ops, &notifier, tmp.path(), false).unwrap();
    assert_eq!(outcome, Outcome::Decommissioned);

    let events = notifier.events.lock().unwrap();
    assert_eq!(events.len(), 1);
    assert_eq!(events[0].0, "host_decommissioned");
    assert_eq!(*ops.deleted.lock().unwrap(), vec!["agent-host-zombie"]);
    assert!(!state_file.exists());

    let q: DecommissionAudit = serde_json::from_str(
        &std::fs::read_to_string(tmp.path().join("infra/.identity_revocation_queue.json")).unwrap(),
    )
    .unwrap();
    assert_eq!(q.queued_for_identity_revocation, vec!["agent-host-zombie"]);
    assert_eq!(q.deleted_hosts, vec!["agent-host-zombie"]);
}

#[test]
fn chopper2_host_is_refused() {
    let tmp = tempfile::tempdir().unwrap();
    let ops = FakeIncusOps::default();
    let notifier = CapturingNotifier::default();
    let outcome = decommission("chopper2-host", &ops, &notifier, tmp.path(), false).unwrap();
    assert_eq!(outcome, Outcome::Refused);
    assert!(notifier.events.lock().unwrap().is_empty());
    assert!(ops.deleted.lock().unwrap().is_empty());
}

#[test]
fn dry_run_notifies_but_does_not_delete() {
    let tmp = tempfile::tempdir().unwrap();
    let ops = FakeIncusOps::default();
    let notifier = CapturingNotifier::default();
    let outcome = decommission("agent-host-1", &ops, &notifier, tmp.path(), true).unwrap();
    assert_eq!(outcome, Outcome::DryRun);
    assert_eq!(notifier.events.lock().unwrap().len(), 1);
    assert!(ops.deleted.lock().unwrap().is_empty());
}
