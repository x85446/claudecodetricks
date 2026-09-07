//! regenerate-trust-token — re-issue an Incus trust token (AC124).

use anyhow::Result;
use serde::{Deserialize, Serialize};
use std::path::Path;

pub trait IncusTrust: Send + Sync {
    /// Returns Ok(true) when the host's existing trust still works.
    fn remote_works(&self, host: &str) -> Result<bool>;
    /// Issue a new scoped trust token; return the JWT.
    fn issue_token(&self, host: &str) -> Result<String>;
    /// Push the token into the container and re-run `incus remote add host`.
    fn install_in_container(&self, host: &str, token: &str) -> Result<()>;
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
pub struct HostStatePartial {
    #[serde(default)]
    pub trust_last_rotated_at: Option<String>,
    #[serde(default, flatten)]
    pub other: serde_json::Map<String, serde_json::Value>,
}

#[derive(Debug, PartialEq)]
pub enum Outcome {
    AlreadyHealthy,
    Rotated,
}

pub fn regenerate(
    host: &str,
    incus: &dyn IncusTrust,
    clock: &dyn Clock,
    state_path: &Path,
    dry_run: bool,
) -> Result<Outcome> {
    if incus.remote_works(host)? {
        return Ok(Outcome::AlreadyHealthy);
    }
    if dry_run {
        return Ok(Outcome::Rotated);
    }

    let token = incus.issue_token(host)?;
    incus.install_in_container(host, &token)?;

    let mut state = load_state(state_path).unwrap_or_default();
    state.trust_last_rotated_at = Some(clock.now_rfc3339());
    save_state(state_path, &state)?;
    Ok(Outcome::Rotated)
}

fn load_state(path: &Path) -> Result<HostStatePartial> {
    let s = std::fs::read_to_string(path)?;
    Ok(serde_json::from_str(&s)?)
}
fn save_state(path: &Path, st: &HostStatePartial) -> Result<()> {
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent)?;
    }
    let tmp = path.with_extension("json.tmp");
    std::fs::write(&tmp, serde_json::to_vec_pretty(st)?)?;
    std::fs::rename(&tmp, path)?;
    Ok(())
}

#[doc(hidden)]
pub mod fakes {
    use super::*;
    use std::sync::Mutex;

    pub struct FakeIncusTrust {
        pub remote_ok: Mutex<bool>,
        pub issued: Mutex<Vec<String>>,
        pub installed: Mutex<Vec<(String, String)>>,
    }
    impl FakeIncusTrust {
        pub fn healthy() -> Self {
            Self {
                remote_ok: Mutex::new(true),
                issued: Mutex::new(vec![]),
                installed: Mutex::new(vec![]),
            }
        }
        pub fn broken() -> Self {
            Self {
                remote_ok: Mutex::new(false),
                issued: Mutex::new(vec![]),
                installed: Mutex::new(vec![]),
            }
        }
    }
    impl IncusTrust for FakeIncusTrust {
        fn remote_works(&self, _host: &str) -> Result<bool> {
            Ok(*self.remote_ok.lock().unwrap())
        }
        fn issue_token(&self, host: &str) -> Result<String> {
            self.issued.lock().unwrap().push(host.into());
            Ok(format!("eyJ.{host}"))
        }
        fn install_in_container(&self, host: &str, token: &str) -> Result<()> {
            self.installed
                .lock()
                .unwrap()
                .push((host.into(), token.into()));
            // Once installed, the remote works again.
            *self.remote_ok.lock().unwrap() = true;
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
