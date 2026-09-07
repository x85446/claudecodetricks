//! Smoke test: read-only skill exposes --help.

#[test]
fn cli_help_succeeds() {
    let out = assert_cmd::Command::cargo_bin("show")
        .expect("binary exists")
        .arg("--help")
        .ok();
    assert!(out.is_ok(), "--help should succeed: {:?}", out);
}
