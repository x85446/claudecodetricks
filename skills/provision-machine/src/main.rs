use anyhow::{Context, Result};
use clap::Parser;
use provision_machine::*;
use std::path::PathBuf;
use std::process::Command;

#[derive(Parser)]
#[command(
    name = "provision-machine",
    about = "ai-operator: bootstrap an Incus container"
)]
struct Args {
    /// Host id matching infra/machines.yml::hosts[].id
    host: String,
    #[arg(long, env = "REPO_ROOT", default_value = "/opt/agent-comms")]
    repo_root: PathBuf,
    #[arg(long, env = "DRY_RUN", default_value_t = false)]
    dry_run: bool,
}

fn main() -> Result<()> {
    let args = Args::parse();

    // Load minimal facts about the host from machines.yml.
    let machines = std::fs::read_to_string(args.repo_root.join("infra/machines.yml"))
        .context("read machines.yml")?;
    let yaml: serde_yaml::Value = serde_yaml::from_str(&machines).context("parse machines.yml")?;
    let host = yaml["hosts"]
        .as_sequence()
        .and_then(|hs| hs.iter().find(|h| h["id"].as_str() == Some(&args.host)))
        .context("host not in machines.yml")?;

    let role = host["purpose"].as_str().unwrap_or("repo-agent");
    let image = host["image"].as_str().unwrap_or("ubuntu/24.04/cloud");
    let cpu = host["cpu"].as_u64().unwrap_or(2) as u32;
    let ram_gb = host["ram_gb"].as_u64().unwrap_or(4) as u32;
    let disk_gb = host["disk_gb"].as_u64().unwrap_or(20) as u32;

    let machine_sh = args
        .repo_root
        .join("global/scripts/machine")
        .join(format!("{role}.sh"));

    let inputs = ProvisionInputs {
        host_id: &args.host,
        image,
        cpu,
        ram_gb,
        disk_gb,
        role,
        machine_sh_local: machine_sh.clone(),
    };

    let ops = ShellIncusOps;
    let notifier = ShellNotifier {
        bin: args.repo_root.join("global/skills/chopper2/notify/run"),
    };

    let state_path = args
        .repo_root
        .join("infra/host-state")
        .join(format!("{}.json", args.host));

    let st = provision(
        &inputs,
        &ops,
        &notifier,
        &RealClock,
        &state_path,
        args.dry_run,
    )?;
    println!("{}", serde_json::to_string_pretty(&st)?);
    Ok(())
}

struct ShellIncusOps;
impl IncusOps for ShellIncusOps {
    fn launch(&self, host: &str, image: &str) -> Result<()> {
        run(
            "incus",
            &[
                "launch",
                &format!("host:{image}"),
                host,
                "--project",
                "agent-comms",
            ],
        )
    }
    fn issue_trust_token(&self, client: &str) -> Result<String> {
        let out = Command::new("incus")
            .args([
                "config",
                "trust",
                "add",
                "--restricted",
                "--projects",
                "agent-comms",
                client,
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
    fn push_file(&self, host: &str, src: &std::path::Path, dst: &std::path::Path) -> Result<()> {
        let dest = format!("{host}{}", dst.display());
        run(
            "incus",
            &[
                "file",
                "push",
                src.to_str().unwrap_or(""),
                &dest,
                "--project",
                "agent-comms",
            ],
        )
    }
    fn exec(&self, host: &str, args: &[&str]) -> Result<()> {
        let mut all = vec!["exec", "--project", "agent-comms", host, "--"];
        all.extend(args.iter().copied());
        run("incus", &all)
    }
}

fn run(cmd: &str, args: &[&str]) -> Result<()> {
    let st = Command::new(cmd)
        .args(args)
        .status()
        .with_context(|| format!("spawn {cmd}"))?;
    if !st.success() {
        anyhow::bail!("{} {} failed: {}", cmd, args.join(" "), st);
    }
    Ok(())
}

struct ShellNotifier {
    bin: PathBuf,
}
impl Notifier for ShellNotifier {
    fn notify(&self, kind: &str, payload: serde_json::Value) -> Result<()> {
        let _ = Command::new(&self.bin)
            .arg("--kind")
            .arg(kind)
            .arg("--payload")
            .arg(payload.to_string())
            .status();
        Ok(())
    }
}
