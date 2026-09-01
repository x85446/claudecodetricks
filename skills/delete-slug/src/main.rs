//! delete-slug — Operator skill (§9, IT-S9).
use std::path::PathBuf;

use agent_comms_git::{GitRunner, RealGit};
use anyhow::Result;
use clap::Parser;

#[derive(Parser, Debug)]
#[command(name = "delete-slug")]
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

    let mut found: Option<PathBuf> = None;
    for path in operator_common::iter_slug_files(&repo) {
        let raw = match std::fs::read_to_string(&path) {
            Ok(s) => s,
            Err(_) => continue,
        };
        if let Ok(v) = serde_json::from_str::<serde_json::Value>(&raw) {
            if v.get("slug").and_then(|s| s.as_str()) == Some(args.slug.as_str()) {
                found = Some(path);
                break;
            }
        }
        if path.file_stem().and_then(|s| s.to_str()) == Some(args.slug.as_str()) {
            found = Some(path);
            break;
        }
    }
    let path = match found {
        Some(p) => p,
        None => {
            eprintln!("delete-slug: slug not found — no-op");
            return Ok(());
        }
    };
    if dry {
        println!("would: git rm {}", path.display());
        return Ok(());
    }
    let runner = RealGit;
    let rel = path.strip_prefix(&repo).unwrap_or(&path);
    runner.run(&["rm", "-f", rel.to_str().unwrap()], &repo)?;
    operator_common::commit_and_push(
        &runner,
        &repo,
        &format!("operator: delete slug {}", args.slug),
    )?;
    Ok(())
}

#[allow(dead_code)]
mod operator_common {
    include!("../../_common.rs");
}
