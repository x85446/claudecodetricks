//! decommission-machine — tear down an undeclared Incus container (S18 / AC18).

use anyhow::Result;
use serde::{Deserialize, Serialize};
use std::path::Path;

pub trait IncusOps: Send + Sync {
    fn delete(&self, host: &str) -> Result<()>;
}

pub trait Notifier: Send + Sync {
    fn notify(&self, kind: &str, payload: serde_json::Value) -> Result<()>;
}

#[derive(Debug, Default, PartialEq, Serialize, Deserialize)]
pub struct DecommissionAudit {
    pub queued_for_identity_revocation: Vec<String>,
    pub deleted_hosts: Vec<String>,
}

#[derive(Debug, PartialEq)]
pub enum Outcome {
    Refused, // chopper2-host
    Decommissioned,
    DryRun,
    AlreadyGone, // host-state file missing AND incus delete returned NotFound
}

pub fn decommission(
    host: &str,
    ops: &dyn IncusOps,
    notify: &dyn Notifier,
    repo_root: &Path,
    dry_run: bool,
) -> Result<Outcome> {
    if host == "chopper2-host" {
        return Ok(Outcome::Refused);
    }
    // Order: notify FIRST so observers see it before the host is gone.
    notify.notify("host_decommissioned", serde_json::json!({"host": host}))?;

    if dry_run {
        return Ok(Outcome::DryRun);
    }

    // Queue this host for reconcile-identities to revoke its agents' keys.
    let queue_path = repo_root.join("infra/.identity_revocation_queue.json");
    let mut queue = read_queue(&queue_path).unwrap_or_default();
    if !queue
        .queued_for_identity_revocation
        .iter()
        .any(|h| h == host)
    {
        queue.queued_for_identity_revocation.push(host.into());
    }
    write_queue(&queue_path, &queue)?;

    let already_gone = match ops.delete(host) {
        Ok(()) => false,
        Err(e) => {
            let s = format!("{e}").to_lowercase();
            // Treat "not found" as success — idempotence.
            if s.contains("not found") || s.contains("does not exist") {
                true
            } else {
                return Err(e);
            }
        }
    };

    let state_path = repo_root
        .join("infra/host-state")
        .join(format!("{host}.json"));
    if state_path.exists() {
        std::fs::remove_file(&state_path)?;
    }

    queue.deleted_hosts.push(host.into());
    write_queue(&queue_path, &queue)?;
    Ok(if already_gone {
        Outcome::AlreadyGone
    } else {
        Outcome::Decommissioned
    })
}

fn read_queue(path: &Path) -> Result<DecommissionAudit> {
    let s = std::fs::read_to_string(path)?;
    Ok(serde_json::from_str(&s)?)
}
fn write_queue(path: &Path, q: &DecommissionAudit) -> Result<()> {
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent)?;
    }
    let tmp = path.with_extension("json.tmp");
    std::fs::write(&tmp, serde_json::to_vec_pretty(q)?)?;
    std::fs::rename(&tmp, path)?;
    Ok(())
}

#[doc(hidden)]
pub mod fakes {
    use super::*;
    use std::sync::Mutex;

    #[derive(Default)]
    pub struct FakeIncusOps {
        pub deleted: Mutex<Vec<String>>,
        pub fail_with: Mutex<Option<String>>,
    }
    impl IncusOps for FakeIncusOps {
        fn delete(&self, host: &str) -> Result<()> {
            if let Some(msg) = self.fail_with.lock().unwrap().clone() {
                anyhow::bail!(msg);
            }
            self.deleted.lock().unwrap().push(host.into());
            Ok(())
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
}
