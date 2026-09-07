use anyhow::{Context, Result};
use clap::Parser;
use reconfigure_machine::*;

#[derive(Parser)]
#[command(
    name = "reconfigure-machine",
    about = "ai-operator: apply declared incus config drift"
)]
struct Args {
    host: String,
    /// JSON-encoded Drift body (image / cpu / ram_gb / disk_gb).
    drift: String,
    #[arg(long, env = "DRY_RUN", default_value_t = false)]
    dry_run: bool,
}

fn main() -> Result<()> {
    let args = Args::parse();
    let drift: Drift = serde_json::from_str(&args.drift).context("parse drift json")?;
    let setter = ShellSetter;
    let applied = apply(&args.host, &drift, &setter, args.dry_run)?;
    for line in applied {
        println!("{line}");
    }
    Ok(())
}

struct ShellSetter;
impl ConfigSetter for ShellSetter {
    fn set(&self, host: &str, key: &str, value: &str) -> Result<()> {
        let st = std::process::Command::new("incus")
            .args([
                "config",
                "set",
                "--project",
                "agent-comms",
                host,
                key,
                value,
            ])
            .status()?;
        if !st.success() {
            anyhow::bail!("incus config set {host} {key} failed");
        }
        Ok(())
    }
}
