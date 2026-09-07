//! reconcile-identities — per-agent deploy-key reconciliation (§22, S19/S20/AC126).

use anyhow::{Context, Result};
use indexmap::IndexMap;
use serde::{Deserialize, Serialize};
use std::collections::{HashMap, HashSet};
use std::path::{Path, PathBuf};

pub const REAPPEARANCE_WINDOW_HOURS: i64 = 24;

// ---------------------------------------------------------------------------
// Trait seams
// ---------------------------------------------------------------------------

/// Subset of GitLab API used by reconcile-identities. Real impl uses
/// reqwest behind feature `live`; tests use FakeGitlab.
pub trait GitlabClient: Send + Sync {
    /// Project keyed by full path (e.g. `gravhl/gravhl-code-factory`).
    fn list_deploy_keys(&self, project: &str) -> Result<Vec<DeployKey>>;
    fn create_deploy_key(
        &self,
        project: &str,
        title: &str,
        public_key: &str,
        can_push: bool,
    ) -> Result<DeployKey>;
    fn revoke_deploy_key(&self, project: &str, key_id: u64) -> Result<()>;
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct DeployKey {
    pub id: u64,
    pub title: String,
    pub fingerprint: String,
    pub key: String,
    pub created_at: String,
}

/// On-host operations — generate keypair, write private key, edit cron.sh.
pub trait HostFs: Send + Sync {
    fn private_key_exists(&self, host: &str, agent: &str) -> Result<bool>;
    fn generate_keypair(&self, host: &str, agent: &str) -> Result<Keypair>;
    fn install_private_key(&self, host: &str, agent: &str, private_pem: &str) -> Result<()>;
    fn update_cron_ssh_command(&self, host: &str, agent: &str) -> Result<()>;
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Keypair {
    pub private_pem: String,
    pub public_openssh: String,
    pub fingerprint: String,
}

pub trait Clock: Send + Sync {
    fn now(&self) -> jiff::Timestamp;
}
pub struct RealClock;
impl Clock for RealClock {
    fn now(&self) -> jiff::Timestamp {
        jiff::Timestamp::now()
    }
}

pub trait Notifier: Send + Sync {
    fn notify(&self, kind: &str, payload: serde_json::Value) -> Result<()>;
}

// ---------------------------------------------------------------------------
// Identities ledger (`infra/identities.json`)
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct IdentitiesLedger {
    pub schema_version: u32,
    #[serde(default)]
    pub agents: IndexMap<String, AgentIdentity>,
    #[serde(default)]
    pub audit: Vec<AuditEntry>,
}
impl Default for IdentitiesLedger {
    fn default() -> Self {
        Self {
            schema_version: 1,
            agents: IndexMap::new(),
            audit: Vec::new(),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct AgentIdentity {
    pub fingerprint: String,
    pub registration: Vec<Registration>,
    pub last_rotated_at: Option<String>,
    pub status: String, // active | identity_pending | revoked
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct Registration {
    pub project: String,
    pub deploy_key_id: u64,
    pub registered_at: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct AuditEntry {
    pub ts: String,
    pub agent: String,
    pub event: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub from_host: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub project: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub key_id: Option<u64>,
}

pub fn load_ledger(path: &Path) -> Result<IdentitiesLedger> {
    if !path.exists() {
        return Ok(IdentitiesLedger::default());
    }
    let s = std::fs::read_to_string(path).with_context(|| format!("read {}", path.display()))?;
    let v: IdentitiesLedger = serde_json::from_str(&s)?;
    Ok(v)
}
pub fn save_ledger(path: &Path, ledger: &IdentitiesLedger) -> Result<()> {
    if let Some(p) = path.parent() {
        std::fs::create_dir_all(p)?;
    }
    let tmp = path.with_extension("json.tmp");
    std::fs::write(&tmp, serde_json::to_vec_pretty(ledger)?)?;
    std::fs::rename(&tmp, path)?;
    Ok(())
}

// ---------------------------------------------------------------------------
// Inputs and reconciliation core
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, PartialEq)]
pub struct AgentSpec {
    pub name: String,
    pub host: String,
    /// Set when the agent is a leaf (df-<repo>-<role>); name of the target repo.
    pub target_repo: Option<String>,
}

#[derive(Debug, Default)]
pub struct ReconcileReport {
    pub generated_keys: Vec<String>,
    pub registered_keys: Vec<(String, String)>, // (agent, project)
    pub revoked_strays: Vec<(String, u64)>,     // (project, key_id)
    pub unstable_strays: Vec<String>,           // titles flagged AC126
    pub identity_pending: Vec<String>,
}

pub const DESIGN_REPO: &str = "gravhl/gravhl-code-factory";

pub fn reconcile(
    desired: &[AgentSpec],
    gl: &dyn GitlabClient,
    fs: &dyn HostFs,
    clock: &dyn Clock,
    notify: &dyn Notifier,
    ledger_path: &Path,
    dry_run: bool,
) -> Result<ReconcileReport> {
    let mut ledger = load_ledger(ledger_path)?;
    let mut report = ReconcileReport::default();

    let desired_titles: HashSet<String> = desired.iter().map(|a| a.name.clone()).collect();

    // Pass 1: every desired agent must have a key + GitLab registration.
    for agent in desired {
        match reconcile_agent(agent, gl, fs, clock, &mut ledger, dry_run) {
            Ok((generated, registered)) => {
                if generated {
                    report.generated_keys.push(agent.name.clone());
                }
                for proj in registered {
                    report.registered_keys.push((agent.name.clone(), proj));
                }
            }
            Err(e) => {
                let entry = ledger
                    .agents
                    .entry(agent.name.clone())
                    .or_insert(AgentIdentity {
                        fingerprint: String::new(),
                        registration: vec![],
                        last_rotated_at: None,
                        status: "identity_pending".into(),
                    });
                entry.status = "identity_pending".into();
                report.identity_pending.push(agent.name.clone());
                notify.notify(
                    "identity_provision_failed",
                    serde_json::json!({"agent": agent.name, "error": e.to_string()}),
                )?;
            }
        }
    }

    // Pass 2: revoke strays on the design repo.
    if let Ok(keys) = gl.list_deploy_keys(DESIGN_REPO) {
        for k in keys {
            if !desired_titles.contains(&k.title) {
                let already_seen = recently_revoked(&ledger, &k.title, clock.now());
                if !dry_run {
                    let _ = gl.revoke_deploy_key(DESIGN_REPO, k.id);
                    push_audit(
                        &mut ledger,
                        AuditEntry {
                            ts: clock.now().to_string(),
                            agent: k.title.clone(),
                            event: "revoked_stray_key".into(),
                            from_host: None,
                            project: Some(DESIGN_REPO.into()),
                            key_id: Some(k.id),
                        },
                    );
                }
                report.revoked_strays.push((DESIGN_REPO.into(), k.id));
                if already_seen {
                    report.unstable_strays.push(k.title.clone());
                    notify.notify(
                        "identity_drift_unstable",
                        serde_json::json!({"title": k.title, "key_id": k.id}),
                    )?;
                    push_audit(
                        &mut ledger,
                        AuditEntry {
                            ts: clock.now().to_string(),
                            agent: k.title.clone(),
                            event: "identity_drift_unstable".into(),
                            from_host: None,
                            project: Some(DESIGN_REPO.into()),
                            key_id: Some(k.id),
                        },
                    );
                }
            }
        }
    }

    if !dry_run {
        save_ledger(ledger_path, &ledger)?;
    }
    Ok(report)
}

fn reconcile_agent(
    agent: &AgentSpec,
    gl: &dyn GitlabClient,
    fs: &dyn HostFs,
    clock: &dyn Clock,
    ledger: &mut IdentitiesLedger,
    dry_run: bool,
) -> Result<(bool, Vec<String>)> {
    let mut generated = false;
    let mut registered_projects = Vec::new();

    let needs_key = !fs.private_key_exists(&agent.host, &agent.name)?;
    let mut current_keypair: Option<Keypair> = None;

    if needs_key {
        if dry_run {
            return Ok((true, registered_projects));
        }
        let kp = fs.generate_keypair(&agent.host, &agent.name)?;
        fs.install_private_key(&agent.host, &agent.name, &kp.private_pem)?;
        fs.update_cron_ssh_command(&agent.host, &agent.name)?;
        push_audit(
            ledger,
            AuditEntry {
                ts: clock.now().to_string(),
                agent: agent.name.clone(),
                event: "regenerated".into(),
                from_host: Some(agent.host.clone()),
                project: None,
                key_id: None,
            },
        );
        generated = true;
        current_keypair = Some(kp);
    }

    // Ensure design-repo registration exists with title=<agent>.
    let design_keys = gl.list_deploy_keys(DESIGN_REPO).unwrap_or_default();
    if !design_keys.iter().any(|k| k.title == agent.name) {
        if dry_run {
            registered_projects.push(DESIGN_REPO.into());
        } else {
            let pubkey = match &current_keypair {
                Some(kp) => kp.public_openssh.clone(),
                None => {
                    let kp = fs.generate_keypair(&agent.host, &agent.name)?;
                    fs.install_private_key(&agent.host, &agent.name, &kp.private_pem)?;
                    fs.update_cron_ssh_command(&agent.host, &agent.name)?;
                    let pk = kp.public_openssh.clone();
                    current_keypair = Some(kp);
                    generated = true;
                    push_audit(
                        ledger,
                        AuditEntry {
                            ts: clock.now().to_string(),
                            agent: agent.name.clone(),
                            event: "regenerated".into(),
                            from_host: Some(agent.host.clone()),
                            project: None,
                            key_id: None,
                        },
                    );
                    pk
                }
            };
            let key = gl.create_deploy_key(DESIGN_REPO, &agent.name, &pubkey, true)?;
            push_audit(
                ledger,
                AuditEntry {
                    ts: clock.now().to_string(),
                    agent: agent.name.clone(),
                    event: format!("registered_on_{DESIGN_REPO}"),
                    from_host: Some(agent.host.clone()),
                    project: Some(DESIGN_REPO.into()),
                    key_id: Some(key.id),
                },
            );
            update_agent_entry(ledger, agent, &key, &current_keypair, clock);
            registered_projects.push(DESIGN_REPO.into());
        }
    }

    // Leaf agents also register on the target repo.
    if let Some(repo) = &agent.target_repo {
        let project = format!("gravhl/{repo}");
        let target_keys = gl.list_deploy_keys(&project).unwrap_or_default();
        if !target_keys.iter().any(|k| k.title == agent.name) {
            if dry_run {
                registered_projects.push(project);
            } else {
                let pubkey = match &current_keypair {
                    Some(kp) => kp.public_openssh.clone(),
                    None => {
                        // Existing key on host but no keypair in memory — best-effort.
                        // Real impl reads pubkey from the host. For now skip if unavailable.
                        return Err(anyhow::anyhow!(
                            "leaf {} key on host but pubkey not in scope; will retry next cycle",
                            agent.name
                        ));
                    }
                };
                let key = gl.create_deploy_key(&project, &agent.name, &pubkey, true)?;
                push_audit(
                    ledger,
                    AuditEntry {
                        ts: clock.now().to_string(),
                        agent: agent.name.clone(),
                        event: format!("registered_on_{project}"),
                        from_host: Some(agent.host.clone()),
                        project: Some(project.clone()),
                        key_id: Some(key.id),
                    },
                );
                update_agent_entry(ledger, agent, &key, &current_keypair, clock);
                registered_projects.push(project);
            }
        }
    }

    Ok((generated, registered_projects))
}

fn update_agent_entry(
    ledger: &mut IdentitiesLedger,
    agent: &AgentSpec,
    key: &DeployKey,
    current: &Option<Keypair>,
    clock: &dyn Clock,
) {
    let entry = ledger
        .agents
        .entry(agent.name.clone())
        .or_insert_with(|| AgentIdentity {
            fingerprint: current
                .as_ref()
                .map(|k| k.fingerprint.clone())
                .unwrap_or_default(),
            registration: vec![],
            last_rotated_at: None,
            status: "active".into(),
        });
    if let Some(kp) = current {
        entry.fingerprint = kp.fingerprint.clone();
        entry.last_rotated_at = Some(clock.now().to_string());
    }
    if !entry.registration.iter().any(|r| r.deploy_key_id == key.id) {
        entry.registration.push(Registration {
            project: key.fingerprint.clone(), // placeholder — overwritten below
            deploy_key_id: key.id,
            registered_at: clock.now().to_string(),
        });
        // Caller knows the project; rewrite the most recent push.
        if let Some(last) = entry.registration.last_mut() {
            last.project = if let Some(p) = ledger.audit.last() {
                p.project.clone().unwrap_or_default()
            } else {
                String::new()
            };
        }
    }
    entry.status = "active".into();
}

fn push_audit(ledger: &mut IdentitiesLedger, entry: AuditEntry) {
    ledger.audit.push(entry);
}

fn recently_revoked(ledger: &IdentitiesLedger, title: &str, now: jiff::Timestamp) -> bool {
    let cutoff_secs = now.as_second() - REAPPEARANCE_WINDOW_HOURS * 3600;
    ledger.audit.iter().rev().any(|e| {
        if e.event != "revoked_stray_key" || e.agent != title {
            return false;
        }
        match e.ts.parse::<jiff::Timestamp>() {
            Ok(t) => t.as_second() >= cutoff_secs,
            Err(_) => false,
        }
    })
}

// ---------------------------------------------------------------------------
// Desired-agent computation from machines.yml
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Deserialize)]
pub struct MachinesYml {
    pub hosts: Vec<MachineHost>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct MachineHost {
    pub id: String,
    #[serde(default)]
    pub agents: Vec<String>,
}

pub fn compute_desired_agents(machines: &MachinesYml) -> Vec<AgentSpec> {
    let mut byname: HashMap<String, AgentSpec> = HashMap::new();
    for h in &machines.hosts {
        for a in &h.agents {
            let normalized = a.replace('@', "-");
            let target_repo = parse_leaf_repo(&normalized);
            byname.entry(normalized.clone()).or_insert(AgentSpec {
                name: normalized,
                host: h.id.clone(),
                target_repo,
            });
        }
    }
    // Trunk-managed agents: chopper2, ai-operator, operator → on chopper2-host.
    for name in ["chopper2", "ai-operator", "operator"] {
        byname.entry(name.into()).or_insert(AgentSpec {
            name: name.into(),
            host: "chopper2-host".into(),
            target_repo: None,
        });
    }
    let mut out: Vec<AgentSpec> = byname.into_values().collect();
    out.sort_by(|a, b| a.name.cmp(&b.name));
    out
}

fn parse_leaf_repo(name: &str) -> Option<String> {
    // df-<repo>-<role>  →  df-<repo>
    let stripped = name
        .strip_suffix("-coder")
        .or_else(|| name.strip_suffix("-tester"))?;
    if stripped.starts_with("df-") {
        Some(stripped.to_string())
    } else {
        None
    }
}

pub fn read_machines(path: &Path) -> Result<MachinesYml> {
    let s = std::fs::read_to_string(path)?;
    Ok(serde_yaml::from_str(&s)?)
}

pub fn default_ledger_path(repo_root: &Path) -> PathBuf {
    repo_root.join("infra/identities.json")
}

// ---------------------------------------------------------------------------
// Test fakes
// ---------------------------------------------------------------------------

#[doc(hidden)]
pub mod fakes {
    use super::*;
    use std::sync::Mutex;

    #[derive(Default)]
    pub struct FakeGitlab {
        pub keys: Mutex<HashMap<String, Vec<DeployKey>>>,
        pub next_id: Mutex<u64>,
    }
    impl FakeGitlab {
        pub fn seed(&self, project: &str, key: DeployKey) {
            self.keys
                .lock()
                .unwrap()
                .entry(project.into())
                .or_default()
                .push(key);
        }
        pub fn project_keys(&self, project: &str) -> Vec<DeployKey> {
            self.keys
                .lock()
                .unwrap()
                .get(project)
                .cloned()
                .unwrap_or_default()
        }
    }
    impl GitlabClient for FakeGitlab {
        fn list_deploy_keys(&self, project: &str) -> Result<Vec<DeployKey>> {
            Ok(self.project_keys(project))
        }
        fn create_deploy_key(
            &self,
            project: &str,
            title: &str,
            public_key: &str,
            _can_push: bool,
        ) -> Result<DeployKey> {
            let mut next = self.next_id.lock().unwrap();
            *next += 1;
            let key = DeployKey {
                id: *next,
                title: title.into(),
                fingerprint: format!("SHA256:fake-{title}"),
                key: public_key.into(),
                created_at: "2026-04-28T00:00:00Z".into(),
            };
            self.keys
                .lock()
                .unwrap()
                .entry(project.into())
                .or_default()
                .push(key.clone());
            Ok(key)
        }
        fn revoke_deploy_key(&self, project: &str, key_id: u64) -> Result<()> {
            if let Some(v) = self.keys.lock().unwrap().get_mut(project) {
                v.retain(|k| k.id != key_id);
            }
            Ok(())
        }
    }

    #[derive(Default)]
    pub struct FakeHostFs {
        pub keys: Mutex<HashMap<(String, String), Keypair>>,
        pub cron_updates: Mutex<Vec<(String, String)>>,
    }
    impl FakeHostFs {
        pub fn already_has(&self, host: &str, agent: &str, kp: Keypair) {
            self.keys
                .lock()
                .unwrap()
                .insert((host.into(), agent.into()), kp);
        }
    }
    impl HostFs for FakeHostFs {
        fn private_key_exists(&self, host: &str, agent: &str) -> Result<bool> {
            Ok(self
                .keys
                .lock()
                .unwrap()
                .contains_key(&(host.into(), agent.into())))
        }
        fn generate_keypair(&self, host: &str, agent: &str) -> Result<Keypair> {
            let kp = Keypair {
                private_pem: format!("-----BEGIN-----\nfake-{agent}\n"),
                public_openssh: format!("ssh-ed25519 AAAA-fake {agent}@{host}"),
                fingerprint: format!("SHA256:fake-{agent}"),
            };
            self.keys
                .lock()
                .unwrap()
                .insert((host.into(), agent.into()), kp.clone());
            Ok(kp)
        }
        fn install_private_key(&self, _host: &str, _agent: &str, _pem: &str) -> Result<()> {
            Ok(())
        }
        fn update_cron_ssh_command(&self, host: &str, agent: &str) -> Result<()> {
            self.cron_updates
                .lock()
                .unwrap()
                .push((host.into(), agent.into()));
            Ok(())
        }
    }

    pub struct FixedClock(pub jiff::Timestamp);
    impl Clock for FixedClock {
        fn now(&self) -> jiff::Timestamp {
            self.0
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

// ---------------------------------------------------------------------------
// Real-world helpers
// ---------------------------------------------------------------------------

pub fn fingerprint_of_openssh(pubkey: &str) -> String {
    use sha2::{Digest, Sha256};
    let body = pubkey.split_whitespace().nth(1).unwrap_or(pubkey);
    let bytes = match base64_decode(body) {
        Ok(b) => b,
        Err(_) => pubkey.as_bytes().to_vec(),
    };
    let digest = Sha256::digest(&bytes);
    format!("SHA256:{}", base64_encode_nopad(&digest))
}

fn base64_decode(s: &str) -> Result<Vec<u8>> {
    use std::convert::TryFrom;
    let s = s.trim();
    let pad_count = (4 - s.len() % 4) % 4;
    let padded = format!("{}{}", s, "=".repeat(pad_count));
    let mut out = Vec::with_capacity(padded.len() / 4 * 3);
    let charset = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    let mut idx = [0u8; 4];
    for chunk in padded.as_bytes().chunks(4) {
        for (i, c) in chunk.iter().enumerate() {
            idx[i] = match c {
                b'=' => 0,
                _ => u8::try_from(charset.iter().position(|&x| x == *c).unwrap_or(0))?,
            };
        }
        let triple = ((idx[0] as u32) << 18)
            | ((idx[1] as u32) << 12)
            | ((idx[2] as u32) << 6)
            | (idx[3] as u32);
        out.push((triple >> 16) as u8);
        if chunk[2] != b'=' {
            out.push((triple >> 8) as u8);
        }
        if chunk[3] != b'=' {
            out.push(triple as u8);
        }
    }
    Ok(out)
}

fn base64_encode_nopad(bytes: &[u8]) -> String {
    let charset = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    let mut out = String::with_capacity(bytes.len() * 4 / 3 + 4);
    for chunk in bytes.chunks(3) {
        let b0 = chunk[0];
        let b1 = if chunk.len() > 1 { chunk[1] } else { 0 };
        let b2 = if chunk.len() > 2 { chunk[2] } else { 0 };
        let triple = ((b0 as u32) << 16) | ((b1 as u32) << 8) | (b2 as u32);
        out.push(charset[((triple >> 18) & 0x3F) as usize] as char);
        out.push(charset[((triple >> 12) & 0x3F) as usize] as char);
        if chunk.len() > 1 {
            out.push(charset[((triple >> 6) & 0x3F) as usize] as char);
        }
        if chunk.len() > 2 {
            out.push(charset[(triple & 0x3F) as usize] as char);
        }
    }
    out
}
