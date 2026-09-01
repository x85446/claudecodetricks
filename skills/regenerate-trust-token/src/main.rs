use anyhow::Result;
use clap::Parser;
use regenerate_trust_token::*;
use std::path::PathBuf;

#[derive(Parser)]
#[command(name = "regenerate-trust-token")]
struct Args {
    host: String,
    #[arg(long, env = "REPO_ROOT", default_value = "/opt/agent-comms")]
    repo_root: PathBuf,
    #[arg(long, env = "DRY_RUN", default_value_t = false)]
    dry_run: bool,
}

fn main() -> Result<()> {
    let args = Args::parse();
    let state_path = args
        .repo_root
        .join("infra/host-state")
        .join(format!("{}.json", args.host));
    let outcome = regenerate(
        &args.host,
        &ShellTrust,
        &RealClock,
        &state_path,
        args.dry_run,
    )?;
    println!("{:?}", outcome);
    Ok(())
}

struct ShellTrust;
impl IncusTrust for ShellTrust {
    fn remote_works(&self, host: &str) -> Result<bool> {
        let st = std::process::Command::new("incus")
            .args([
                "exec",
                "--project",
                "agent-comms",
                host,
                "--",
                "incus",
                "remote",
                "list",
            ])
            .status()?;
        Ok(st.success())
    }
    fn issue_token(&self, host: &str) -> Result<String> {
        let out = std::process::Command::new("incus")
            .args([
                "config",
                "trust",
                "add",
                "--restricted",
                "--projects",
                "agent-comms",
                host,
            ])
            .output()?;
        if !out.status.success() {
            anyhow::bail!("trust add failed: {}", String::from_utf8_lossy(&out.stderr));
        }
        Ok(String::from_utf8_lossy(&out.stdout)
            .lines()
            .find(|l| l.starts_with("eyJ"))
            .unwrap_or_default()
            .to_string())
    }
    fn install_in_container(&self, host: &str, token: &str) -> Result<()> {
        let st = std::process::Command::new("incus")
            .args([
                "exec",
                "--project",
                "agent-comms",
                host,
                "--",
                "incus",
                "remote",
                "add",
                "host",
                &format!("--token={token}"),
            ])
            .status()?;
        if !st.success() {
            anyhow::bail!("install failed");
        }
        Ok(())
    }
}
