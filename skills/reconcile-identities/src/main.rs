use anyhow::Result;
use clap::Parser;
use reconcile_identities::*;
use std::path::PathBuf;

#[derive(Parser)]
#[command(
    name = "reconcile-identities",
    about = "ai-operator: per-agent deploy-key reconciliation"
)]
struct Args {
    #[arg(long, env = "REPO_ROOT", default_value = "/opt/agent-comms")]
    repo_root: PathBuf,
    #[arg(long, env = "DRY_RUN", default_value_t = false)]
    dry_run: bool,
}

fn main() -> Result<()> {
    let args = Args::parse();
    let machines = read_machines(&args.repo_root.join("infra/machines.yml"))?;
    let desired = compute_desired_agents(&machines);

    let ledger_path = default_ledger_path(&args.repo_root);

    // Real impls require feature gating; this binary is a stub that prints
    // the desired set when invoked without a configured GitLab/host env.
    // The actual provisioning runs from the integration test harness or a
    // future `live` feature build.
    if std::env::var("RECONCILE_IDENTITIES_LIVE").is_err() {
        println!(
            "{}",
            serde_json::to_string_pretty(&serde_json::json!({
                "desired_agents": desired
                    .iter()
                    .map(|a| serde_json::json!({
                        "name": a.name,
                        "host": a.host,
                        "target_repo": a.target_repo,
                    })).collect::<Vec<_>>(),
                "ledger_path": ledger_path,
                "dry_run": args.dry_run,
            }))?
        );
        return Ok(());
    }

    // Live mode (only when explicitly opted in via env var) wires real
    // backends. The real backends live behind a `live` feature in a future
    // module; here we keep the binary buildable in the workspace.
    eprintln!("RECONCILE_IDENTITIES_LIVE=1 set but `live` impls not compiled");
    std::process::exit(2);
}
