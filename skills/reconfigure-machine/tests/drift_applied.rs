use reconfigure_machine::fakes::*;
use reconfigure_machine::*;

#[test]
fn no_drift_is_no_op() {
    let setter = CapturingSetter::default();
    let applied = apply("agent-host-1", &Drift::default(), &setter, false).unwrap();
    assert!(applied.is_empty());
    assert!(setter.calls.lock().unwrap().is_empty());
}

#[test]
fn drift_calls_set_for_every_field() {
    let setter = CapturingSetter::default();
    let drift = Drift {
        image: Some("ubuntu/24.04/cloud".into()),
        cpu: Some(4),
        ram_gb: Some(8),
        disk_gb: Some(40),
    };
    let applied = apply("agent-host-2", &drift, &setter, false).unwrap();
    assert_eq!(applied.len(), 4);
    assert_eq!(setter.calls.lock().unwrap().len(), 4);
}

#[test]
fn dry_run_records_intent_no_calls() {
    let setter = CapturingSetter::default();
    let drift = Drift {
        cpu: Some(8),
        ..Default::default()
    };
    let applied = apply("agent-host-3", &drift, &setter, true).unwrap();
    assert_eq!(applied, vec!["dry: limits.cpu=8"]);
    assert!(setter.calls.lock().unwrap().is_empty());
}
