//! Compute the desired-agent set from machines.yml + trunk-managed agents.

use reconcile_identities::*;

#[test]
fn desired_set_includes_trunk_agents() {
    let yml = MachinesYml {
        hosts: vec![
            MachineHost {
                id: "agent-host-1".into(),
                agents: vec!["df-chat-coder".into(), "df-chat-tester".into()],
            },
            MachineHost {
                id: "chopper2-host".into(),
                agents: vec![],
            },
        ],
    };
    let desired = compute_desired_agents(&yml);
    let names: Vec<String> = desired.iter().map(|a| a.name.clone()).collect();
    assert!(names.contains(&"chopper2".to_string()));
    assert!(names.contains(&"ai-operator".to_string()));
    assert!(names.contains(&"operator".to_string()));
    assert!(names.contains(&"df-chat-coder".to_string()));
    assert!(names.contains(&"df-chat-tester".to_string()));

    let coder = desired.iter().find(|a| a.name == "df-chat-coder").unwrap();
    assert_eq!(coder.target_repo.as_deref(), Some("df-chat"));
    assert_eq!(coder.host, "agent-host-1");

    let trunk = desired.iter().find(|a| a.name == "ai-operator").unwrap();
    assert!(trunk.target_repo.is_none());
    assert_eq!(trunk.host, "chopper2-host");
}
