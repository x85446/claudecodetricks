//! provision-machine — Incus container bootstrap (§22, AC123).

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::path::{Path, PathBuf};

pub const STUCK_AT_FAILURES: u32 = 3;

/// Provision-time Incus operations. Tests use `FakeIncusOps`.
pub trait IncusOps: Send + Sync {
    fn launch(&self, host: &str, image: &str) -> Result<()>;
    fn issue_trust_token(&self, client: &str) -> Result<String>;
    fn push_file(&self, host: &str, src: &Path, dst: &Path) -> Result<()>;
    fn exec(&self, host: &str, args: &[&str]) -> Result<()>;
}

pub trait Notifier: Send + Sync {
    fn notify(&self, kind: &str, payload: serde_json::Value) -> Result<()>;
}

pub trait Clock: Send + Sync {
    fn now_rfc3339(&self) -> String;
}
pub struct RealClock;
impl Clock for RealClock {
    fn now_rfc3339(&self) -> String {
        jiff::Timestamp::now().to_string()
    }
}

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq)]
pub struct HostState {
    pub provisioned_at: Option<String>,
    pub image: String,
    pub cpu: u32,
    pub ram_gb: u32,
    pub disk_gb: u32,
    pub last_health_check: Option<String>,
    pub status: String,
    #[serde(default)]
    pub consecutive_failures: u32,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct ProvisionInputs<'a> {
    pub host_id: &'a str,
    pub image: &'a str,
    pub cpu: u32,
    pub ram_gb: u32,
    pub disk_gb: u32,
    pub role: &'a str,
    pub machine_sh_local: PathBuf,
}

pub fn provision(
    inputs: &ProvisionInputs<'_>,
    ops: &dyn IncusOps,
    notify: &dyn Notifier,
    clock: &dyn Clock,
    state_path: &Path,
    dry_run: bool,
) -> Result<HostState> {
    let mut state = read_state(state_path).unwrap_or_default();

    if state.status == "stuck" {
        // AC123: stop retrying once stuck.
        return Ok(state);
    }

    if dry_run {
        return Ok(HostState {
            status: "provisioned_dry_run".into(),
            image: inputs.image.into(),
            cpu: inputs.cpu,
            ram_gb: inputs.ram_gb,
            disk_gb: inputs.disk_gb,
            ..state
        });
    }

    let outcome: Result<()> = (|| {
        ops.launch(inputs.host_id, inputs.image)?;
        let token = ops.issue_trust_token(inputs.host_id)?;
        ops.exec(
            inputs.host_id,
            &["apt-get", "install", "-y", "incus-client"],
        )?;
        ops.exec(
            inputs.host_id,
            &[
                "incus",
                "remote",
                "add",
                "host",
                &format!("--token={}", token),
            ],
        )?;
        ops.push_file(
            inputs.host_id,
            &inputs.machine_sh_local,
            Path::new("/opt/machine.sh"),
        )?;
        ops.exec(inputs.host_id, &["bash", "/opt/machine.sh"])?;
        Ok(())
    })();

    match outcome {
        Ok(()) => {
            state = HostState {
                provisioned_at: Some(clock.now_rfc3339()),
                image: inputs.image.into(),
                cpu: inputs.cpu,
                ram_gb: inputs.ram_gb,
                disk_gb: inputs.disk_gb,
                last_health_check: Some(clock.now_rfc3339()),
                status: "healthy".into(),
                consecutive_failures: 0,
            };
            write_state(state_path, &state)?;
            Ok(state)
        }
        Err(e) => {
            state.consecutive_failures += 1;
            state.image = inputs.image.into();
            state.cpu = inputs.cpu;
            state.ram_gb = inputs.ram_gb;
            state.disk_gb = inputs.disk_gb;
            if state.consecutive_failures >= STUCK_AT_FAILURES {
                state.status = "stuck".into();
                notify.notify(
                    "host_stuck_provisioning",
                    serde_json::json!({"host": inputs.host_id, "error": e.to_string()}),
                )?;
            } else {
                state.status = "provision_failed".into();
                notify.notify(
                    "host_provision_failed",
                    serde_json::json!({"host": inputs.host_id, "error": e.to_string()}),
                )?;
            }
            write_state(state_path, &state)?;
            Err(e)
        }
    }
}

pub fn read_state(path: &Path) -> Result<HostState> {
    let s = std::fs::read_to_string(path).with_context(|| format!("read {}", path.display()))?;
    let v: HostState = serde_json::from_str(&s)?;
    Ok(v)
}

pub fn write_state(path: &Path, state: &HostState) -> Result<()> {
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent)?;
    }
    let tmp = path.with_extension("json.tmp");
    std::fs::write(&tmp, serde_json::to_vec_pretty(state)?)?;
    std::fs::rename(&tmp, path)?;
    Ok(())
}

#[doc(hidden)]
pub mod fakes {
    use super::*;
    use std::sync::Mutex;

    #[derive(Default)]
    pub struct FakeIncusOps {
        pub calls: Mutex<Vec<String>>,
        pub fail_after: Mutex<Option<usize>>, // injected failure point
    }
    impl FakeIncusOps {
        pub fn fail_at(&self, n: usize) {
            *self.fail_after.lock().unwrap() = Some(n);
        }
        fn record_or_fail(&self, label: String) -> Result<()> {
            let mut calls = self.calls.lock().unwrap();
            calls.push(label);
            let cnt = calls.len();
            drop(calls);
            if let Some(f) = *self.fail_after.lock().unwrap() {
                if cnt > f {
                    anyhow::bail!("injected failure at call #{}", cnt);
                }
            }
            Ok(())
        }
    }
    impl IncusOps for FakeIncusOps {
        fn launch(&self, host: &str, image: &str) -> Result<()> {
            self.record_or_fail(format!("launch {host} {image}"))
        }
        fn issue_trust_token(&self, client: &str) -> Result<String> {
            self.record_or_fail(format!("trust {client}"))?;
            Ok("eyJfaketoken".to_string())
        }
        fn push_file(&self, host: &str, src: &Path, dst: &Path) -> Result<()> {
            self.record_or_fail(format!(
                "push {host} {} -> {}",
                src.display(),
                dst.display()
            ))
        }
        fn exec(&self, host: &str, args: &[&str]) -> Result<()> {
            self.record_or_fail(format!("exec {host} {}", args.join(" ")))
        }
    }

    #[derive(Default)]
    pub struct CapturingNotifier {
        pub events: Mutex<Vec<(String, serde_json::Value)>>,
    }
    impl Notifier for CapturingNotifier {
        fn notify(&self, kind: &str, payload: serde_json::Value) -> Result<()> {
            self.events.lock().unwrap().push((kind.into(), payload));
            Ok(())
        }
    }

    pub struct FixedClock(pub String);
    impl Clock for FixedClock {
        fn now_rfc3339(&self) -> String {
            self.0.clone()
        }
    }
}
