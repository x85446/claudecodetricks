use anyhow::Result;
use clap::Parser;
use health_check_machines::*;
use std::path::PathBuf;

#[derive(Parser)]
#[command(name = "health-check-machines")]
struct Args {
    #[arg(long, env = "REPO_ROOT", default_value = "/opt/agent-comms")]
    repo_root: PathBuf,
}

fn main() -> Result<()> {
    let args = Args::parse();
    let hosts = load_hosts(&args.repo_root.join("infra/machines.yml"))?;
    let pinger = ShellPing;
    let notifier = ShellNotifier {
        bin: args.repo_root.join("global/skills/chopper2/notify/run"),
    };
    let outcome = check_all(
        &hosts,
        &pinger,
        &notifier,
        &RealClock,
        &default_health_dir(&args.repo_root),
        &default_host_state_dir(&args.repo_root),
    )?;
    println!("{}", serde_json::to_string_pretty(&outcome.records)?);
    Ok(())
}

struct ShellPing;
impl IncusPing for ShellPing {
    fn hostname_of(&self, host: &str) -> Result<String> {
        let out = std::process::Command::new("incus")
            .args([
                "exec",
                "--project",
                "agent-comms",
                host,
                "--",
                "hostname",
                "-s",
            ])
            .output()?;
        if !out.status.success() {
            anyhow::bail!("exec failed: {}", String::from_utf8_lossy(&out.stderr));
        }
        Ok(String::from_utf8_lossy(&out.stdout).trim().to_string())
    }
}

struct ShellNotifier {
    bin: PathBuf,
}
impl Notifier for ShellNotifier {
    fn notify(&self, kind: &str, payload: serde_json::Value) -> Result<()> {
        let _ = std::process::Command::new(&self.bin)
            .arg("--kind")
            .arg(kind)
            .arg("--payload")
            .arg(payload.to_string())
            .status();
        Ok(())
    }
}
