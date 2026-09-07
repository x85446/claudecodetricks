//! retry-dead-letter — Operator skill (§9, IT-S9).
use std::path::PathBuf;

use agent_comms_core::paths::slugs_dead_letter_dir;
use agent_comms_git::{git_mv, RealGit};
use anyhow::{anyhow, Context, Result};
use clap::Parser;

#[derive(Parser, Debug)]
#[command(name = "retry-dead-letter")]
struct Args {
    slug: String,
    #[arg(long)]
    dry_run: bool,
    #[arg(long)]
    repo: Option<PathBuf>,
}

fn main() -> Result<()> {
    let args = Args::parse();
    let dry = args.dry_run || std::env::var("DRY_RUN").map(|v| v == "1").unwrap_or(false);
    let repo = match args.repo {
        Some(p) => p,
        None => operator_common::find_repo_root()?,
    };
    let src_dir = slugs_dead_letter_dir(&repo);
    let target_dir = repo.join("agents/chopper2/to-chopper");

    let src = find_slug(&src_dir, &args.slug)?;
    let mut v: serde_json::Value = serde_json::from_str(&std::fs::read_to_string(&src)?)?;
    v["bounce_count"] = serde_json::json!(0);
    let now = jiff::Timestamp::now();
    let mut events = v
        .get("events")
        .and_then(|x| x.as_array())
        .cloned()
        .unwrap_or_default();
    events.push(serde_json::json!({
        "ts": now.to_string(),
        "type": "retried",
        "by": "operator",
        "actor": "operator",
    }));
    v["events"] = serde_json::Value::Array(events);

    let target = target_dir.join(src.file_name().unwrap());
    if dry {
        println!(
            "would: write {} ; git mv {} {}",
            target.display(),
            src.display(),
            target.display()
        );
        return Ok(());
    }
    std::fs::create_dir_all(&target_dir)?;
    let json = serde_json::to_string_pretty(&v)?;
    std::fs::write(&src, json)?;
    let runner = RealGit;
    git_mv(&runner, &repo, &src, &target)?;
    operator_common::commit_and_push(
        &runner,
        &repo,
        &format!("operator: retry dead-letter slug {}", args.slug),
    )?;
    Ok(())
}

fn find_slug(dir: &std::path::Path, slug: &str) -> Result<std::path::PathBuf> {
    if !dir.exists() {
        return Err(anyhow!("dead-letter dir missing"));
    }
    for entry in walkdir::WalkDir::new(dir).into_iter().flatten() {
        if !entry.file_type().is_file() {
            continue;
        }
        let p = entry.path();
        let raw = std::fs::read_to_string(p).with_context(|| format!("read {p:?}"))?;
        if let Ok(v) = serde_json::from_str::<serde_json::Value>(&raw) {
            if v.get("slug").and_then(|s| s.as_str()) == Some(slug) {
                return Ok(p.to_path_buf());
            }
        }
        if p.file_stem().and_then(|s| s.to_str()) == Some(slug) {
            return Ok(p.to_path_buf());
        }
    }
    Err(anyhow!("slug {slug} not in dead-letter — no-op"))
}

#[allow(dead_code)]
mod operator_common {
    include!("../../_common.rs");
}
