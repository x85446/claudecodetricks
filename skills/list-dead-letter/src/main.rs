//! list-dead-letter — Operator skill (§9, IT-S9). Read-only.
use std::path::PathBuf;

use agent_comms_core::paths::slugs_dead_letter_dir;
use anyhow::Result;
use clap::Parser;

#[derive(Parser, Debug)]
#[command(name = "list-dead-letter")]
struct Args {
    #[arg(long)]
    repo: Option<PathBuf>,
}

fn main() -> Result<()> {
    let args = Args::parse();
    let repo = match args.repo {
        Some(p) => p,
        None => operator_common::find_repo_root()?,
    };
    let dir = slugs_dead_letter_dir(&repo);
    if !dir.exists() {
        eprintln!("(dead-letter dir is empty or absent)");
        return Ok(());
    }
    let mut count = 0;
    for entry in walkdir::WalkDir::new(&dir).into_iter().flatten() {
        if !entry.file_type().is_file() {
            continue;
        }
        if entry.path().extension().and_then(|s| s.to_str()) != Some("json") {
            continue;
        }
        let raw = std::fs::read_to_string(entry.path())?;
        let v: serde_json::Value = match serde_json::from_str(&raw) {
            Ok(v) => v,
            Err(_) => continue,
        };
        let slug = v.get("slug").and_then(|x| x.as_str()).unwrap_or("?");
        let filed_by = v.get("filed_by").and_then(|x| x.as_str()).unwrap_or("?");
        let bounce_count = v.get("bounce_count").and_then(|x| x.as_u64()).unwrap_or(0);
        let last_reason = v
            .get("feedback")
            .and_then(|f| f.get("reason"))
            .and_then(|s| s.as_str())
            .unwrap_or("-");
        println!("{slug}\t{filed_by}\t{bounce_count}\t{last_reason}");
        count += 1;
    }
    eprintln!("{count} slug(s) in dead-letter.");
    Ok(())
}

#[allow(dead_code)]
mod operator_common {
    include!("../../_common.rs");
}
