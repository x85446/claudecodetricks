//! `rollup-parent` — close a parent bug when all children are verified/closed.

use std::path::PathBuf;

use agent_comms_core::{cfg_load, paths};
use agent_comms_git::RealGit;
use anyhow::{Context, Result};
use clap::Parser;
use serde::Serialize;
use serde_json::Value;

#[derive(Parser)]
#[command(name = "rollup-parent")]
struct Args {
    #[arg(long)]
    parent_id: String,
    #[arg(long)]
    repo_root: PathBuf,
    #[arg(long, env = "DRY_RUN")]
    dry_run: bool,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "kebab-case")]
enum RollupAction {
    Closed,
    Pending(usize),
    WarnCap,
    NotFound,
}

#[derive(Debug, Serialize)]
struct RollupResult {
    action: RollupAction,
}

fn child_state(repo_root: &std::path::Path, child_id: &str) -> Option<String> {
    let candidates = [
        paths::bugs_dir(repo_root).join(format!("{child_id}.json")),
        paths::bugs_blocked_dir(repo_root).join(format!("{child_id}.json")),
        paths::bugs_closed_dir(repo_root).join(format!("{child_id}.json")),
    ];
    for path in candidates {
        if path.exists() {
            let bytes = std::fs::read(&path).ok()?;
            let v: Value = serde_json::from_slice(&bytes).ok()?;
            return v
                .get("GHLSTATE")
                .or_else(|| v.get("state"))
                .and_then(Value::as_str)
                .map(|s| s.to_string());
        }
    }
    None
}

fn run(args: &Args) -> Result<RollupResult> {
    let parent_path = paths::bugs_dir(&args.repo_root).join(format!("{}.json", args.parent_id));
    if !parent_path.exists() {
        return Ok(RollupResult {
            action: RollupAction::NotFound,
        });
    }
    let parent: Value =
        serde_json::from_slice(&std::fs::read(&parent_path).context("read parent")?)
            .context("parse parent")?;

    let children: Vec<String> = parent
        .get("children")
        .and_then(Value::as_array)
        .map(|arr| {
            arr.iter()
                .filter_map(|v| v.as_str().map(|s| s.to_string()))
                .collect()
        })
        .unwrap_or_default();

    let cfg = cfg_load::load_yaml(&args.repo_root.join("config.yml")).ok();
    let cap = cfg
        .as_ref()
        .and_then(|c| {
            c.pointer("/limits/defaults/bug_max_children_per_parent")
                .and_then(Value::as_u64)
        })
        .unwrap_or(6) as usize;
    if children.len() == cap {
        return Ok(RollupResult {
            action: RollupAction::WarnCap,
        });
    }

    let mut pending = 0usize;
    for c in &children {
        match child_state(&args.repo_root, c).as_deref() {
            Some("verified") | Some("closed") => continue,
            _ => pending += 1,
        }
    }
    if pending > 0 {
        return Ok(RollupResult {
            action: RollupAction::Pending(pending),
        });
    }

    if !args.dry_run {
        // Move parent to bugs/closed/.
        let dst_dir = paths::bugs_closed_dir(&args.repo_root);
        std::fs::create_dir_all(&dst_dir).ok();
        let dst = dst_dir.join(format!("{}.json", args.parent_id));
        let runner = RealGit;
        agent_comms_git::git_mv(&runner, &args.repo_root, &parent_path, &dst)?;
    }
    Ok(RollupResult {
        action: RollupAction::Closed,
    })
}

fn main() -> Result<()> {
    let args = Args::parse();
    let r = run(&args)?;
    println!("{}", serde_json::to_string(&r)?);
    Ok(())
}
