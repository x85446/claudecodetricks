use anyhow::Result;
use clap::Parser;
use decommission_machine::*;
use std::path::PathBuf;

#[derive(Parser)]
#[command(name = "decommission-machine")]
struct Args {
    host: String,
    #[arg(long, env = "REPO_ROOT", default_value = "/opt/agent-comms")]
    repo_root: PathBuf,
    #[arg(long, env = "DRY_RUN", default_value_t = false)]
    dry_run: bool,
}

fn main() -> Result<()> {
    let args = Args::parse();
    let ops = ShellOps;
    let notifier = ShellNotifier {
        bin: args.repo_root.join("global/skills/chopper2/notify/run"),
    };
    let outcome = decommission(&args.host, &ops, &notifier, &args.repo_root, args.dry_run)?;
    println!("{:?}", outcome);
    Ok(())
}

struct ShellOps;
impl IncusOps for ShellOps {
    fn delete(&self, host: &str) -> Result<()> {
        let st = std::process::Command::new("incus")
            .args(["delete", "--force", "--project", "agent-comms", host])
            .status()?;
        if !st.success() {
            anyhow::bail!("incus delete failed");
        }
        Ok(())
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
