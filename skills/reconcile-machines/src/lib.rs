//! reconcile-machines — driver for machine-fleet reconciliation.
//!
//! See SKILL.md and design_ai_operator.md §3.1.

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::collections::HashSet;
use std::path::{Path, PathBuf};

/// Incus container abstraction. Real impl shells out to `incus`; tests use
/// `FakeIncus`. Methods scope to project=agent-comms automatically.
pub trait IncusClient: Send + Sync {
    /// List running container names in the agent-comms project.
    fn list(&self) -> Result<Vec<String>>;
    /// Return the current resource configuration of a container, or `None`
    /// if it does not exist.
    fn get_config(&self, host: &str) -> Result<Option<HostConfig>>;
    /// `incus exec <host> -- true`. `Ok(())` => reachable.
    fn ping(&self, host: &str) -> Result<()>;
}

/// Notification dispatch (delegates to chopper2's notify skill in real impl).
pub trait Notifier: Send + Sync {
    fn notify(&self, kind: &str, payload: serde_json::Value) -> Result<()>;
}

/// Side-effect dispatch — reconcile-machines invokes other skills. In real
/// runtime each sub-skill is a separate binary; the trait lets tests record
/// which sub-skills were called with what arguments.
pub trait SubSkillDispatch: Send + Sync {
    fn provision(&self, host: &str) -> Result<()>;
    fn reconfigure(&self, host: &str, drift: &Drift) -> Result<()>;
    fn health_check(&self, host: &str) -> Result<()>;
    fn decommission(&self, host: &str) -> Result<()>;
}

/// Declared machine.yml host entry.
#[derive(Debug, Clone, Deserialize, Serialize, PartialEq)]
pub struct DeclaredHost {
    pub id: String,
    #[serde(default)]
    pub agents: Vec<String>,
    #[serde(default)]
    pub purpose: Option<String>,
    #[serde(default)]
    pub image: Option<String>,
    #[serde(default)]
    pub cpu: Option<u32>,
    #[serde(default)]
    pub ram_gb: Option<u32>,
    #[serde(default)]
    pub disk_gb: Option<u32>,
}

/// Configuration observed from `incus config`.
#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]
pub struct HostConfig {
    pub image: String,
    pub cpu: u32,
    pub ram_gb: u32,
    pub disk_gb: u32,
}

/// Per-field drift between declared vs observed.
#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]
pub struct Drift {
    pub image: Option<String>,
    pub cpu: Option<u32>,
    pub ram_gb: Option<u32>,
    pub disk_gb: Option<u32>,
}
impl Drift {
    pub fn is_empty(&self) -> bool {
        self.image.is_none()
            && self.cpu.is_none()
            && self.ram_gb.is_none()
            && self.disk_gb.is_none()
    }
}

/// What `reconcile_once` decided about each host. Returned for tests.
#[derive(Debug, Default, PartialEq, Serialize)]
pub struct Plan {
    pub provision: Vec<String>,
    pub reconfigure: Vec<(String, Drift)>,
    pub health: Vec<String>,
    pub decommission: Vec<String>,
    pub skipped_chopper2: bool,
}

/// `infra/machines.yml` root.
#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct MachinesYml {
    pub schema_version: u32,
    pub hosts: Vec<DeclaredHost>,
}

pub fn load_machines_yml(path: &Path) -> Result<MachinesYml> {
    let data = std::fs::read_to_string(path).with_context(|| format!("read {}", path.display()))?;
    let v: MachinesYml = serde_yaml::from_str(&data).with_context(|| "parse machines.yml")?;
    Ok(v)
}

const CHOPPER2_HOST: &str = "chopper2-host";

/// Build the reconciliation plan for a single cycle, then dispatch.
/// Returns the plan even on partial failure so callers can log.
pub fn reconcile(
    machines: &MachinesYml,
    incus: &dyn IncusClient,
    dispatch: &dyn SubSkillDispatch,
    notify: &dyn Notifier,
    dry_run: bool,
) -> Result<Plan> {
    let observed = match incus.list() {
        Ok(v) => v,
        Err(e) => {
            // AC125: incus_unreachable does not cascade.
            notify.notify(
                "incus_unreachable",
                serde_json::json!({"error": e.to_string()}),
            )?;
            return Ok(Plan::default());
        }
    };

    let observed_set: HashSet<String> = observed.iter().cloned().collect();
    let declared_set: HashSet<String> = machines.hosts.iter().map(|h| h.id.clone()).collect();
    let mut plan = Plan::default();

    // Pass 1 — declared hosts.
    for host in &machines.hosts {
        if host.id == CHOPPER2_HOST {
            plan.skipped_chopper2 = true;
            continue;
        }
        if !observed_set.contains(&host.id) {
            plan.provision.push(host.id.clone());
            if !dry_run {
                let _ = dispatch.provision(&host.id);
            }
            continue;
        }
        let drift = compute_drift(host, incus.get_config(&host.id)?.unwrap_or_default());
        if !drift.is_empty() {
            plan.reconfigure.push((host.id.clone(), drift.clone()));
            if !dry_run {
                let _ = dispatch.reconfigure(&host.id, &drift);
            }
            continue;
        }
        plan.health.push(host.id.clone());
        if !dry_run {
            let _ = dispatch.health_check(&host.id);
        }
    }

    // Pass 2 — undeclared containers (skip chopper2-host even if Incus knows it).
    for name in &observed {
        if name == CHOPPER2_HOST {
            continue;
        }
        if !declared_set.contains(name) {
            plan.decommission.push(name.clone());
            if !dry_run {
                let _ = dispatch.decommission(name);
            }
        }
    }
    Ok(plan)
}

fn compute_drift(declared: &DeclaredHost, observed: HostConfig) -> Drift {
    let mut d = Drift::default();
    if let Some(image) = &declared.image {
        if image != &observed.image && !observed.image.is_empty() {
            d.image = Some(image.clone());
        }
    }
    if let Some(cpu) = declared.cpu {
        if cpu != observed.cpu && observed.cpu != 0 {
            d.cpu = Some(cpu);
        }
    }
    if let Some(ram) = declared.ram_gb {
        if ram != observed.ram_gb && observed.ram_gb != 0 {
            d.ram_gb = Some(ram);
        }
    }
    if let Some(disk) = declared.disk_gb {
        if disk != observed.disk_gb && observed.disk_gb != 0 {
            d.disk_gb = Some(disk);
        }
    }
    d
}

// ---------------------------------------------------------------------------
// Test-support fakes (also exposed under `test-support` feature for the
// integration test crate, per design_index §8 mocks table).
// ---------------------------------------------------------------------------

#[doc(hidden)]
pub mod fakes {
    use super::*;
    use indexmap::IndexMap;
    use std::sync::Mutex;

    #[derive(Default)]
    pub struct FakeIncus {
        pub running: Mutex<Vec<String>>,
        pub configs: Mutex<IndexMap<String, HostConfig>>,
        pub fail_list: Mutex<bool>,
    }
    impl FakeIncus {
        pub fn with_running<I, S>(running: I) -> Self
        where
            I: IntoIterator<Item = S>,
            S: Into<String>,
        {
            Self {
                running: Mutex::new(running.into_iter().map(Into::into).collect()),
                configs: Mutex::new(IndexMap::new()),
                fail_list: Mutex::new(false),
            }
        }
        pub fn add(&self, host: &str, cfg: HostConfig) {
            self.running.lock().unwrap().push(host.to_string());
            self.configs.lock().unwrap().insert(host.to_string(), cfg);
        }
        pub fn fail_list(&self) {
            *self.fail_list.lock().unwrap() = true;
        }
    }
    impl IncusClient for FakeIncus {
        fn list(&self) -> Result<Vec<String>> {
            if *self.fail_list.lock().unwrap() {
                anyhow::bail!("incus daemon unreachable");
            }
            Ok(self.running.lock().unwrap().clone())
        }
        fn get_config(&self, host: &str) -> Result<Option<HostConfig>> {
            Ok(self.configs.lock().unwrap().get(host).cloned())
        }
        fn ping(&self, host: &str) -> Result<()> {
            if self.running.lock().unwrap().iter().any(|h| h == host) {
                Ok(())
            } else {
                anyhow::bail!("not running")
            }
        }
    }

    #[derive(Default)]
    pub struct CapturingDispatch {
        pub provisioned: Mutex<Vec<String>>,
        pub reconfigured: Mutex<Vec<(String, Drift)>>,
        pub health: Mutex<Vec<String>>,
        pub decommissioned: Mutex<Vec<String>>,
    }
    impl SubSkillDispatch for CapturingDispatch {
        fn provision(&self, host: &str) -> Result<()> {
            self.provisioned.lock().unwrap().push(host.into());
            Ok(())
        }
        fn reconfigure(&self, host: &str, drift: &Drift) -> Result<()> {
            self.reconfigured
                .lock()
                .unwrap()
                .push((host.into(), drift.clone()));
            Ok(())
        }
        fn health_check(&self, host: &str) -> Result<()> {
            self.health.lock().unwrap().push(host.into());
            Ok(())
        }
        fn decommission(&self, host: &str) -> Result<()> {
            self.decommissioned.lock().unwrap().push(host.into());
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

/// Real-impl helpers (kept here so the binary entrypoint is thin).
pub fn dispatch_subskills_via_run(repo_root: &Path) -> impl SubSkillDispatch + 'static {
    RealDispatch {
        repo_root: repo_root.to_path_buf(),
    }
}

pub struct RealDispatch {
    pub repo_root: PathBuf,
}
impl SubSkillDispatch for RealDispatch {
    fn provision(&self, host: &str) -> Result<()> {
        invoke_run(&self.repo_root, "provision-machine", &[host])
    }
    fn reconfigure(&self, host: &str, drift: &Drift) -> Result<()> {
        let json = serde_json::to_string(drift)?;
        invoke_run(&self.repo_root, "reconfigure-machine", &[host, &json])
    }
    fn health_check(&self, host: &str) -> Result<()> {
        invoke_run(&self.repo_root, "health-check-machines", &[host])
    }
    fn decommission(&self, host: &str) -> Result<()> {
        invoke_run(&self.repo_root, "decommission-machine", &[host])
    }
}

fn invoke_run(repo_root: &Path, skill: &str, args: &[&str]) -> Result<()> {
    let bin = repo_root
        .join("global/skills/ai-operator")
        .join(skill)
        .join("run");
    let status = std::process::Command::new(&bin)
        .args(args)
        .status()
        .with_context(|| format!("invoke {}", bin.display()))?;
    if !status.success() {
        anyhow::bail!("{} exited {}", skill, status);
    }
    Ok(())
}
