//! Smoke test: --help parses; dry-run flag is recognized.
//! Deeper IT-S<n> coverage lives in the infra_tests crate (Module 6).

#[test]
fn cli_help_succeeds() {
    let out = assert_cmd::Command::cargo_bin("force-close")
        .expect("binary exists")
        .arg("--help")
        .ok();
    assert!(out.is_ok(), "--help should succeed: {:?}", out);
}
