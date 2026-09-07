//! health-check-machines — per-host reachability + status snapshot.

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::path::{Path, PathBuf};

pub trait IncusPing: Send + Sync {
    /// `incus exec <host> -- hostname -s` → returned name (or Err if unreachable).
    fn hostname_of(&self, host: &str) -> Result<String>;
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

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct HealthRecord {
    pub host: String,
    pub status: String, // "healthy" | "unhealthy"
    pub last_health_check: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub error: Option<String>,
}

#[derive(Debug, Clone)]
pub struct Host {
    pub id: String,
}

#[derive(Debug, Default)]
pub struct Outcome {
    pub records: Vec<HealthRecord>,
    pub notifies: Vec<String>,
}

pub fn check_all(
    hosts: &[Host],
    pinger: &dyn IncusPing,
    notify: &dyn Notifier,
    clock: &dyn Clock,
    health_dir: &Path,
    host_state_dir: &Path,
) -> Result<Outcome> {
    let mut out = Outcome::default();
    for host in hosts {
        if host.id == "chopper2-host" {
            continue; // bootstrap surface; never reconciled
        }
        let now = clock.now_rfc3339();
        let rec = match pinger.hostname_of(&host.id) {
            Ok(observed) if observed == host.id => HealthRecord {
                host: host.id.clone(),
                status: "healthy".into(),
                last_health_check: now.clone(),
                error: None,
            },
            Ok(observed) => HealthRecord {
                host: host.id.clone(),
                status: "unhealthy".into(),
                last_health_check: now.clone(),
                error: Some(format!("hostname mismatch: got {observed}")),
            },
            Err(e) => {
                notify.notify(
                    "incus_unreachable",
                    serde_json::json!({"host": host.id, "error": e.to_string()}),
                )?;
                out.notifies.push(host.id.clone());
                HealthRecord {
                    host: host.id.clone(),
                    status: "unhealthy".into(),
                    last_health_check: now.clone(),
                    error: Some(e.to_string()),
                }
            }
        };
        write_json(&health_dir.join(format!("{}.json", host.id)), &rec)?;
        update_host_state(host_state_dir, &host.id, &rec)?;
        out.records.push(rec);
    }
    Ok(out)
}

fn update_host_state(dir: &Path, host: &str, rec: &HealthRecord) -> Result<()> {
    let path = dir.join(format!("{host}.json"));
    let mut existing: serde_json::Value = if path.exists() {
        serde_json::from_str(&std::fs::read_to_string(&path)?)
            .unwrap_or_else(|_| serde_json::json!({}))
    } else {
        serde_json::json!({})
    };
    existing["last_health_check"] = serde_json::Value::String(rec.last_health_check.clone());
    existing["status"] = serde_json::Value::String(rec.status.clone());
    write_json(&path, &existing)
}

fn write_json<T: Serialize>(path: &Path, value: &T) -> Result<()> {
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent)?;
    }
    let tmp = path.with_extension("json.tmp");
    std::fs::write(&tmp, serde_json::to_vec_pretty(value)?)?;
    std::fs::rename(&tmp, path)?;
    Ok(())
}

pub fn load_hosts(machines_yml: &Path) -> Result<Vec<Host>> {
    let s = std::fs::read_to_string(machines_yml)
        .with_context(|| format!("read {}", machines_yml.display()))?;
    let v: serde_yaml::Value = serde_yaml::from_str(&s)?;
    let mut out = Vec::new();
    if let Some(seq) = v["hosts"].as_sequence() {
        for h in seq {
            if let Some(id) = h["id"].as_str() {
                out.push(Host { id: id.into() });
            }
        }
    }
    Ok(out)
}

pub fn default_health_dir(repo_root: &Path) -> PathBuf {
    repo_root.join("infra/health")
}
pub fn default_host_state_dir(repo_root: &Path) -> PathBuf {
    repo_root.join("infra/host-state")
}

#[doc(hidden)]
pub mod fakes {
    use super::*;
    use std::collections::HashMap;
    use std::sync::Mutex;

    #[derive(Default)]
    pub struct FakePing {
        pub responses: Mutex<HashMap<String, std::result::Result<String, String>>>,
    }
    impl FakePing {
        pub fn ok(&self, host: &str, observed: &str) {
            self.responses
                .lock()
                .unwrap()
                .insert(host.into(), Ok(observed.into()));
        }
        pub fn fail(&self, host: &str, msg: &str) {
            self.responses
                .lock()
                .unwrap()
                .insert(host.into(), Err(msg.into()));
        }
    }
    impl IncusPing for FakePing {
        fn hostname_of(&self, host: &str) -> Result<String> {
            match self.responses.lock().unwrap().get(host).cloned() {
                Some(Ok(s)) => Ok(s),
                Some(Err(e)) => anyhow::bail!(e),
                None => anyhow::bail!("not configured: {host}"),
            }
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
