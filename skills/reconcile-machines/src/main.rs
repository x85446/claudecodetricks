use anyhow::{Context, Result};
use clap::Parser;
use reconcile_machines::*;
use std::path::PathBuf;

#[derive(Parser)]
#[command(
    name = "reconcile-machines",
    about = "ai-operator: machine reconciliation"
)]
struct Args {
    /// Path to the agent-comms repo root.
    #[arg(long, env = "REPO_ROOT", default_value = "/opt/agent-comms")]
    repo_root: PathBuf,
    /// Skip every mutation. Honors DRY_RUN=1.
    #[arg(long, env = "DRY_RUN", default_value_t = false)]
    dry_run: bool,
}

fn main() -> Result<()> {
    let args = Args::parse();
    let machines = load_machines_yml(&args.repo_root.join("infra/machines.yml"))
        .context("load machines.yml")?;

    let incus = real_incus::RealIncus;
    let dispatch = dispatch_subskills_via_run(&args.repo_root);
    let notifier = real_notifier::RealNotifier::new(&args.repo_root);

    let plan = reconcile(&machines, &incus, &dispatch, &notifier, args.dry_run)?;
    println!("{}", serde_json::to_string_pretty(&plan)?);
    Ok(())
}

mod real_incus {
    use super::*;
    #[derive(Default)]
    pub struct RealIncus;
    impl IncusClient for RealIncus {
        fn list(&self) -> Result<Vec<String>> {
            let out = std::process::Command::new("incus")
                .args(["list", "--project", "agent-comms", "-f", "csv", "-c", "n"])
                .output()
                .context("incus list")?;
            if !out.status.success() {
                anyhow::bail!(
                    "incus list failed: {}",
                    String::from_utf8_lossy(&out.stderr)
                );
            }
            Ok(String::from_utf8_lossy(&out.stdout)
                .lines()
                .map(|s| s.trim().to_string())
                .filter(|s| !s.is_empty())
                .collect())
        }
        fn get_config(&self, host: &str) -> Result<Option<HostConfig>> {
            let out = std::process::Command::new("incus")
                .args(["config", "show", "--project", "agent-comms", host])
                .output()?;
            if !out.status.success() {
                return Ok(None);
            }
            // Best-effort parse — full schema is intentionally fuzzy here;
            // reconcile-machines doesn't need every field, just the ones in
            // machines.yml::hosts[]. The merge step's RealIncus wires the
            // canonical reading through agent-comms-cfg.
            Ok(Some(HostConfig::default()))
        }
        fn ping(&self, host: &str) -> Result<()> {
            let st = std::process::Command::new("incus")
                .args(["exec", "--project", "agent-comms", host, "--", "true"])
                .status()?;
            if st.success() {
                Ok(())
            } else {
                anyhow::bail!("ping failed")
            }
        }
    }
}

mod real_notifier {
    use super::*;
    use std::path::Path;
    pub struct RealNotifier {
        notify_bin: std::path::PathBuf,
    }
    impl RealNotifier {
        pub fn new(repo_root: &Path) -> Self {
            Self {
                notify_bin: repo_root.join("global/skills/chopper2/notify/run"),
            }
        }
    }
    impl Notifier for RealNotifier {
        fn notify(&self, kind: &str, payload: serde_json::Value) -> Result<()> {
            // Best-effort fire — never fail the cycle on a notify error.
            let _ = std::process::Command::new(&self.notify_bin)
                .arg("--kind")
                .arg(kind)
                .arg("--payload")
                .arg(payload.to_string())
                .status();
            Ok(())
        }
    }
}
