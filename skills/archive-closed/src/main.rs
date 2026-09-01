//! `archive-closed` — git-rm bugs in `bugs/closed/` older than the TTL.

use std::path::PathBuf;

use agent_comms_core::{cfg_load, paths};
use agent_comms_git::{GitRunner, RealGit};
use anyhow::{Context, Result};
use clap::Parser;
use jiff::Timestamp;
use serde::Serialize;
use serde_json::Value;
use walkdir::WalkDir;

#[derive(Parser)]
#[command(name = "archive-closed")]
struct Args {
    #[arg(long)]
    repo_root: PathBuf,
    #[arg(long, env = "DRY_RUN")]
    dry_run: bool,
}

#[derive(Debug, Serialize)]
struct ArchiveResult {
    pruned: Vec<String>,
    inspected: usize,
}

fn closed_at(bug: &Value) -> Option<Timestamp> {
    let events = bug.get("events").and_then(Value::as_array)?;
    for ev in events.iter().rev() {
        let action = ev.get("action").and_then(Value::as_str).unwrap_or("");
        let ts = ev.get("ts").and_then(Value::as_str)?;
        if action.contains("closed") || action == "transition_to_closed" {
            if let Ok(t) = ts.parse::<Timestamp>() {
                return Some(t);
            }
        }
    }
    None
}

fn run(args: &Args) -> Result<ArchiveResult> {
    let cfg = cfg_load::load_yaml(&args.repo_root.join("config.yml")).context("load config.yml")?;
    let prune_days = cfg
        .get("ttl")
        .and_then(|v| v.get("defaults"))
        .and_then(|v| v.get("prune_closed_days"))
        .and_then(Value::as_i64)
        .unwrap_or(30);
    let now = Timestamp::now();
    let cutoff_secs = prune_days * 86_400;

    let dir = paths::bugs_closed_dir(&args.repo_root);
    let mut pruned = Vec::new();
    let mut inspected = 0usize;
    let runner = RealGit;

    if !dir.exists() {
        return Ok(ArchiveResult { pruned, inspected });
    }

    for entry in WalkDir::new(&dir)
        .max_depth(1)
        .into_iter()
        .filter_map(|e| e.ok())
    {
        let p = entry.path();
        if !p.is_file() {
            continue;
        }
        let stem = match p.file_stem().and_then(|s| s.to_str()) {
            Some(s) if s.starts_with("BUG-") => s.to_string(),
            _ => continue,
        };
        inspected += 1;
        let bytes = match std::fs::read(p) {
            Ok(b) => b,
            Err(_) => continue,
        };
        let bug: Value = match serde_json::from_slice(&bytes) {
            Ok(v) => v,
            Err(_) => continue,
        };
        if let Some(t) = closed_at(&bug) {
            if (now.as_second() - t.as_second()) > cutoff_secs {
                if !args.dry_run {
                    runner
                        .run(&["rm", "--", p.to_str().unwrap_or("")], &args.repo_root)
                        .context("git rm")?;
                }
                pruned.push(stem);
            }
        }
    }

    Ok(ArchiveResult { pruned, inspected })
}

fn main() -> Result<()> {
    let args = Args::parse();
    let r = run(&args)?;
    println!("{}", serde_json::to_string(&r)?);
    Ok(())
}
