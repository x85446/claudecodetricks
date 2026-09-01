//! resume — Operator skill (§9, IT-S30).
use std::path::PathBuf;

use agent_comms_git::{GitRunner, RealGit};
use anyhow::Result;
use clap::Parser;

#[derive(Parser, Debug)]
#[command(name = "resume")]
struct Args {
    #[arg(long)]
    dry_run: bool,
    #[arg(long)]
    repo: Option<PathBuf>,
}

fn main() -> Result<()> {
    let args = Args::parse();
    set_paused(args.repo, args.dry_run, false)
}

fn set_paused(repo: Option<PathBuf>, dry_run: bool, value: bool) -> Result<()> {
    let dry = dry_run || std::env::var("DRY_RUN").map(|v| v == "1").unwrap_or(false);
    let repo = match repo {
        Some(p) => p,
        None => operator_common::find_repo_root()?,
    };
    let cfg_path = repo.join("config.yml");
    let raw = std::fs::read_to_string(&cfg_path)?;
    let mut cfg: serde_yaml::Value = serde_yaml::from_str(&raw)?;
    let current = cfg.get("paused").and_then(|v| v.as_bool()).unwrap_or(false);
    if current == value {
        eprintln!("pause/resume: paused already {value} — no-op");
        return Ok(());
    }
    if let serde_yaml::Value::Mapping(m) = &mut cfg {
        m.insert(
            serde_yaml::Value::String("paused".into()),
            serde_yaml::Value::Bool(value),
        );
    }
    if dry {
        println!("would: set config.yml::paused = {value}");
        return Ok(());
    }
    let new_yaml = serde_yaml::to_string(&cfg)?;
    std::fs::write(&cfg_path, new_yaml)?;
    let runner = RealGit;
    runner.run(&["add", "config.yml"], &repo)?;
    operator_common::commit_and_push(
        &runner,
        &repo,
        &format!(
            "operator: {} cluster",
            if value { "pause" } else { "resume" }
        ),
    )?;
    Ok(())
}

#[allow(dead_code)]
mod operator_common {
    include!("../../_common.rs");
}
