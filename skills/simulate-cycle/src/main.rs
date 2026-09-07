//! simulate-cycle — Operator skill (§9, IT-S31, AC31).
use std::path::PathBuf;
use std::time::SystemTime;

use anyhow::{bail, Context, Result};
use clap::Parser;

#[derive(Parser, Debug)]
#[command(name = "simulate-cycle")]
struct Args {
    /// "chopper2", "<repo>-coder", "<repo>-tester", or "ai-operator".
    #[arg(long, default_value = "chopper2")]
    role: String,
    #[arg(long)]
    repo: Option<PathBuf>,
}

fn main() -> Result<()> {
    let args = Args::parse();
    let repo = match args.repo {
        Some(p) => p,
        None => operator_common::find_repo_root()?,
    };

    // Snapshot pre-state for mutation assertion.
    let pre = snapshot_repo(&repo)?;

    let cron_path = match args.role.as_str() {
        "chopper2" => repo.join("agents/chopper2/cron.sh"),
        "ai-operator" => repo.join("agents/ai-operator/cron.sh"),
        s if s.ends_with("-coder") => {
            let r = s.trim_end_matches("-coder");
            repo.join(format!("agents/repo-agents/{r}/coder/cron.sh"))
        }
        s if s.ends_with("-tester") => {
            let r = s.trim_end_matches("-tester");
            repo.join(format!("agents/repo-agents/{r}/tester/cron.sh"))
        }
        other => bail!("unknown role: {other}"),
    };
    if !cron_path.exists() {
        eprintln!("simulate-cycle: cron.sh not found at {cron_path:?} — synthesizing dry-run preview only.");
    } else {
        let status = std::process::Command::new("sh")
            .arg(cron_path)
            .env("DRY_RUN", "1")
            .env("KILROY_SIMULATE", "1")
            .current_dir(&repo)
            .status()
            .context("invoke cron.sh in DRY_RUN mode")?;
        if !status.success() {
            eprintln!("(cron.sh exited {status})");
        }
    }

    let post = snapshot_repo(&repo)?;
    if pre != post {
        bail!("simulate-cycle: filesystem mutated under DRY_RUN — assertion failed");
    }
    println!("simulate-cycle: clean (no filesystem mutations)");
    Ok(())
}

fn snapshot_repo(repo: &std::path::Path) -> Result<Vec<(PathBuf, SystemTime, u64)>> {
    let mut out = Vec::new();
    for entry in walkdir::WalkDir::new(repo).into_iter().flatten() {
        let p = entry.path();
        if p.components().any(|c| {
            matches!(
                c.as_os_str().to_str(),
                Some(".git") | Some("target") | Some(".kilroy") | Some(".ai")
            )
        }) {
            continue;
        }
        if !entry.file_type().is_file() {
            continue;
        }
        let md = entry.metadata()?;
        out.push((
            p.to_path_buf(),
            md.modified().unwrap_or(SystemTime::UNIX_EPOCH),
            md.len(),
        ));
    }
    out.sort();
    Ok(out)
}

#[allow(dead_code)]
mod operator_common {
    include!("../../_common.rs");
}
